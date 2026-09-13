package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

type planCompletionFunc func(context.Context, string) (string, error)

func (f planCompletionFunc) Complete(ctx context.Context, prompt string) (string, error) {
	return f(ctx, prompt)
}

func TestReviewedPlanUsesImportedIdentitiesAndRefusesReviewRace(t *testing.T) {
	for _, race := range []bool{false, true} {
		t.Run(map[bool]string{false: "remap_before_review", true: "conflict_after_review"}[race], func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			if err := os.WriteFile(filepath.Join(e.dir, "INTENT.md"), []byte("Extend retained editing."), 0600); err != nil {
				t.Fatal(err)
			}
			addConflict := func() {
				if err := e.rel.CreateStory(&release.Story{ID: "US-1", Title: "Retained different story", Status: release.StoryStatusPending}); err != nil {
					t.Fatal(err)
				}
			}
			if !race {
				addConflict()
			}
			e.mgr.cfg.PlanGenerator = fixedPlanCompletion(`{"goals":[{"id":"G-1","title":"Edit","description":"Validated edit"}],"stories":[{"id":"US-1","title":"New edit capability","goal_id":"G-1","verification_script":"go test ./...","tasks":[{"id":"T-US-1-1","title":"Implement edit","description":"Implement and verify editing","mode":"afk"}]}]}`)
			var reviewed string
			e.mgr.cfg.PlanReviewer = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
				reviewed = prompt
				if race {
					addConflict()
				}
				return `{"approved":true,"assessment":"Required verification represented"}`, nil
			})
			result, err := e.mgr.Plan(context.Background(), PlanRequest{IntentFile: "INTENT.md", NoValidate: true, AutoImport: true, Review: true})
			if race {
				if err == nil || !strings.Contains(err.Error(), "changed task identities require review") {
					t.Fatal("changed plan imported after review", err)
				}
				if len(e.rel.GetTasks()) != 0 {
					t.Fatal("refused import created work")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			story := result.Plan.Stories[0]
			if story.ID == "US-1" || !strings.Contains(reviewed, story.ID) || e.rel.GetTask(story.Tasks[0].ID) == nil {
				t.Fatal("review and imported task identities diverged")
			}
			data, err := os.ReadFile(result.ArtifactPath)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if hex.EncodeToString(hash[:]) != result.ArtifactHash {
				t.Fatal("plan artifact digest changed")
			}
			var retained struct {
				Stories []struct {
					ID string `json:"id"`
				} `json:"stories"`
			}
			if err := json.Unmarshal(data, &retained); err != nil || retained.Stories[0].ID != story.ID {
				t.Fatal("artifact describes different plan", err)
			}
		})
	}
}

type fixedPlanCompletion string

func (f fixedPlanCompletion) Complete(context.Context, string) (string, error) { return string(f), nil }

type countedPlanCompletion struct{ calls int }

func (p *countedPlanCompletion) Complete(context.Context, string) (string, error) {
	p.calls++
	return `{}`, nil
}

func TestReviewedPlanCannotFallbackOutsideAdmittedAdapters(t *testing.T) {
	for _, generatorOnly := range []bool{false, true} {
		provider := &countedPlanCompletion{}
		mgr := &Manager{}
		if generatorOnly {
			mgr.cfg.PlanGenerator = provider
		} else {
			mgr.cfg.PlanReviewer = provider
		}
		if _, err := mgr.Plan(context.Background(), PlanRequest{Review: true}); err == nil || !strings.Contains(err.Error(), "native fallback refused") {
			t.Fatal("partial adapter setup accepted")
		}
		if provider.calls != 0 {
			t.Fatal("inference occurred before checking all planning participants")
		}
	}
}

func TestReviewedPlanImportedOnlyAfterReview(t *testing.T) {
	for _, approved := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "approved"}[approved], func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "INTENT.md"), []byte("Keep existing records; add validated editing."), 0600); err != nil {
				t.Fatal(err)
			}
			store, err := state.NewStore(filepath.Join(dir, ".openexec", "openexec.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			review := `{"approved":false,"assessment":"Missing verification"}`
			if approved {
				review = `{"approved":true,"assessment":"Verification covers the intent"}`
			}
			mgr, err := New(Config{
				WorkDir: dir, StateStore: store,
				PlanGenerator: fixedPlanCompletion(`{"goals":[{"id":"G-1","title":"Edit","description":"Validated edit"}],"stories":[{"id":"US-1","title":"Edit","goal_id":"G-1","verification_script":"go test ./...","tasks":[{"id":"T-1","title":"Vertical edit slice","description":"Implement and verify editing","mode":"afk"}]}]}`),
				PlanReviewer:  fixedPlanCompletion(review),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer mgr.Close()
			result, err := mgr.Plan(context.Background(), PlanRequest{IntentFile: "INTENT.md", NoValidate: true, AutoImport: true, Review: true})
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid != approved || result.Review == nil || result.Review.Approved != approved {
				t.Fatalf("wrong disposition: %#v", result)
			}
			receipt, err := os.ReadFile(result.ReviewArtifactPath)
			if err != nil {
				t.Fatal(err)
			}
			var retained struct {
				PlanDigest string `json:"plan_digest"`
				Review     struct {
					Approved bool `json:"approved"`
				} `json:"review"`
			}
			if err := json.Unmarshal(receipt, &retained); err != nil {
				t.Fatal(err)
			}
			if retained.PlanDigest != result.ArtifactHash || retained.Review.Approved != approved {
				t.Fatal("review evidence not bound to retained plan")
			}
			rel, err := mgr.GetInternalReleaseManager()
			if err != nil {
				t.Fatal(err)
			}
			if (rel.GetTask("T-1") != nil) != approved {
				t.Fatal("review boundary did not control import")
			}
			if err := rel.Load(); err != nil {
				t.Fatal(err)
			}
			if (rel.GetTask("T-1") != nil) != approved {
				t.Fatal("durable import differs from review disposition")
			}
		})
	}
}
