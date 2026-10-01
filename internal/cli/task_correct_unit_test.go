package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

// Invoke the Cobra unit directly: no shipped subprocess, queue or verification
// executor contributes to this dedicated coverage run.
func TestTaskCorrectUnit(t *testing.T) {
	for _, mode := range []string{"success", "planned", "missing_flag", "bad_decision", "missing_db", "missing_task", "bad_receipt", "bad_plan", "replay"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".openexec/\n"), 0644); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"init", "-b", "retained"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "retained"}} {
				cmd := exec.Command("git", args...)
				cmd.Dir = root
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git: %s %v", out, err)
				}
			}
			path := filepath.Join(root, ".openexec/openexec.db")
			s, err := state.NewStore(path)
			if err != nil {
				t.Fatal(err)
			}
			closeStore := sync.OnceFunc(func() { s.Close() })
			t.Cleanup(closeStore)
			rel, err := release.NewSQLiteStore(s.GetDB())
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := rel.CreateStory(ctx, &release.Story{ID: "S", Title: "story", Status: release.StoryStatusInProgress}); err != nil {
				t.Fatal(err)
			}
			if err := rel.CreateTask(ctx, &release.Task{ID: "A", StoryID: "S", Title: "task", Status: release.TaskStatusFailed, AttemptCount: 3, MaxAttempts: 3, Git: &release.TaskGitInfo{Branch: "retained"}, Metadata: map[string]interface{}{"verification_failure_evidence": "legacy"}}); err != nil {
				t.Fatal(err)
			}
			if err := s.CreateRun(ctx, "A", "", "", root, "workspace-write"); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal([]gates.CheckFailure{{Gate: "verify", ExitCode: 2}})
			sum := sha256.Sum256(raw)
			receipt, _ := json.Marshal(map[string]string{gates.VerificationFailureReceiptKey: string(raw), gates.VerificationFailureDigestKey: hex.EncodeToString(sum[:])})
			if mode == "bad_receipt" {
				receipt = []byte(`{}`)
			}
			if err := s.AddRunStepFull(ctx, "legacy", "A", "", "verify", "deterministic-verification", 0, "failed", "", string(receipt)); err != nil {
				t.Fatal(err)
			}
			if mode == "planned" {
				manifest, err := knowledge.BuildScanManifest(root)
				if err != nil {
					t.Fatal(err)
				}
				_, err = s.GetDB().Exec(`INSERT INTO repositories(id,persisted_uuid) VALUES('r','r'); INSERT INTO checkouts(id,repository_id,root_path) VALUES('c','r','/candidate'); INSERT INTO worktrees(id,repository_id,checkout_id,root_path) VALUES('w','r','c','/candidate'); INSERT INTO graph_generations(id,schema_version,repository_id,checkout_id,worktree_id,worktree_state_hash,configuration_digest,extractor_version,manifest_hash,status) VALUES('g',1,'r','c','w',?,'config','extractor','manifest','current')`, manifest.WorktreeStateHash)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.CreateValidationPlanRevision(ctx, state.ValidationPlanRevision{TaskID: "A", GenerationID: "g", WorktreeStateHash: manifest.WorktreeStateHash, Status: "accepted"}); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"A", "--dir", root, "--authorize-correction", "--decision-ref", "owner:unit"}
			switch mode {
			case "missing_flag":
				args = []string{"A", "--dir", root, "--decision-ref", "owner:unit"}
			case "bad_decision":
				args[len(args)-1] = "bad decision"
			case "missing_task":
				args[0] = "missing"
			case "missing_db":
				args[2] = t.TempDir()
			case "bad_plan":
				args = append(args, "--plan", "wrong")
			}
			cmd := newTaskCorrectCommand(true)
			cmd.SetArgs(args)
			err = cmd.Execute()
			success := mode == "success" || mode == "planned" || mode == "replay"
			if (err == nil) != success {
				t.Fatalf("success=%t: %v", success, err)
			}
			if mode == "replay" {
				cmd = newTaskCorrectCommand(true)
				cmd.SetArgs(args)
				if err := cmd.Execute(); err == nil {
					t.Fatal("replay accepted")
				}
			}
			closeStore()
			s, err = state.NewStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			rel, err = release.NewSQLiteStore(s.GetDB())
			if err != nil {
				t.Fatal(err)
			}
			task, err := rel.GetTask(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			if task.AttemptCount != 3 || task.Status != release.TaskStatusFailed {
				t.Fatalf("authorization executed work: %+v", task)
			}
			if success {
				c, err := release.CorrectionForTask(task)
				if err != nil || c.DecisionRef != "owner:unit" || c.Consumed {
					t.Fatalf("decision not persisted: %+v %v", c, err)
				}
			} else if task.Metadata["task_correction"] != nil {
				t.Fatal("denial wrote authority")
			}
		})
	}
}
