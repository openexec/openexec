package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTracker(t *testing.T) {
	repoPath := setupTestRepo(t)
	client := NewClient(Config{Enabled: true, RepoPath: repoPath})
	
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "tracker.json")

	tracker, err := NewTracker(client, storePath)
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}

	t.Run("LinkCommitToTask", func(t *testing.T) {
		err := tracker.LinkCommitToTask("hash123", "T-001")
		if err != nil {
			t.Fatalf("LinkCommitToTask failed: %v", err)
		}

		commits := tracker.GetTaskCommits("T-001")
		if len(commits) != 1 || commits[0] != "hash123" {
			t.Errorf("unexpected commits: %v", commits)
		}

		taskID := tracker.GetCommitTask("hash123")
		if taskID != "T-001" {
			t.Errorf("got task %q, want %q", taskID, "T-001")
		}
	})

	t.Run("Story and Release tracking", func(t *testing.T) {
		tracker.SetStoryBranch("S-001", "feature/S-001")
		if tracker.GetStoryBranch("S-001") != "feature/S-001" {
			t.Error("SetStoryBranch failed")
		}

		tracker.SetReleaseBranch("1.0.0", "release/1.0.0")
		if tracker.GetReleaseBranch("1.0.0") != "release/1.0.0" {
			t.Error("SetReleaseBranch failed")
		}

		tracker.SetReleaseTag("1.0.0", "v1.0.0")
		if tracker.GetReleaseTag("1.0.0") != "v1.0.0" {
			t.Error("SetReleaseTag failed")
		}
	})

	t.Run("Persistence", func(t *testing.T) {
		// New tracker with same store path
		tracker2, err := NewTracker(client, storePath)
		if err != nil {
			t.Fatalf("failed to reload tracker: %v", err)
		}
		if tracker2.GetStoryBranch("S-001") != "feature/S-001" {
			t.Error("reloaded tracker missing state")
		}
	})
}

func TestAutoLinkCommitsFromMessages(t *testing.T) {
	repoPath := setupTestRepo(t)
	client := NewClient(Config{Enabled: true, RepoPath: repoPath})
	
	// Add a commit with task ID in message
	err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("data"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "file.txt")
	runGit(t, repoPath, "commit", "-m", "Implement T-002: new feature")

	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "tracker.json")
	tracker, _ := NewTracker(client, storePath)

	linked, err := tracker.AutoLinkCommitsFromMessages("")
	if err != nil {
		t.Fatalf("AutoLinkCommitsFromMessages failed: %v", err)
	}

	if linked == 0 {
		t.Error("expected at least one commit to be linked")
	}

	commits := tracker.GetTaskCommits("T-002")
	if len(commits) == 0 {
		t.Error("T-002 should have linked commits")
	}
}

func TestMergeStoryToRelease(t *testing.T) {
	repoPath := setupTestRepo(t)
	client := NewClient(Config{Enabled: true, RepoPath: repoPath})
	
	mainBranch, _ := client.CurrentBranch()
	
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "tracker.json")
	tracker, _ := NewTracker(client, storePath)

	// Setup release branch
	_, err := tracker.CreateReleaseBranch("1.1.0", mainBranch)
	if err != nil {
		t.Fatalf("CreateReleaseBranch failed: %v", err)
	}

	// Setup story branch
	_, err = tracker.CreateStoryBranch("S-002", mainBranch)
	if err != nil {
		t.Fatalf("CreateStoryBranch failed: %v", err)
	}

	// Add work to story branch
	err = os.WriteFile(filepath.Join(repoPath, "story.txt"), []byte("story work"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "story.txt")
	runGit(t, repoPath, "commit", "-m", "Story work")

	// Merge story to release
	info, err := tracker.MergeStoryToRelease("S-002", "1.1.0")
	if err != nil {
		t.Fatalf("MergeStoryToRelease failed: %v", err)
	}

	if info.SourceBranch != "feature/S-002" {
		t.Errorf("got source branch %q, want %q", info.SourceBranch, "feature/S-002")
	}
	if info.TargetBranch != "release/1.1.0" {
		t.Errorf("got target branch %q, want %q", info.TargetBranch, "release/1.1.0")
	}

	// Verify we are on release branch
	branch, _ := client.CurrentBranch()
	if branch != "release/1.1.0" {
		t.Errorf("expected to be on release/1.1.0, got %q", branch)
	}
}

func TestTracker_MergeStory_Conflict(t *testing.T) {
	repoPath := setupTestRepo(t)
	client := NewClient(Config{Enabled: true, RepoPath: repoPath})
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "tracker.json")
	tracker, _ := NewTracker(client, storePath)

	mainBranch, _ := client.CurrentBranch()
	tracker.CreateReleaseBranch("1.2.0", mainBranch)
	
	// Create conflict
	os.WriteFile(filepath.Join(repoPath, "conflict.txt"), []byte("base"), 0644)
	runGit(t, repoPath, "add", "conflict.txt")
	runGit(t, repoPath, "commit", "-m", "Base")

	tracker.CreateStoryBranch("S-CONFL", "release/1.2.0")
	os.WriteFile(filepath.Join(repoPath, "conflict.txt"), []byte("story"), 0644)
	runGit(t, repoPath, "add", "conflict.txt")
	runGit(t, repoPath, "commit", "-m", "Story change")

	client.Checkout("release/1.2.0")
	os.WriteFile(filepath.Join(repoPath, "conflict.txt"), []byte("release"), 0644)
	runGit(t, repoPath, "add", "conflict.txt")
	runGit(t, repoPath, "commit", "-m", "Release change")

	_, err := tracker.MergeStoryToRelease("S-CONFL", "1.2.0")
	if err == nil {
		t.Error("expected merge conflict error")
	}
	if !IsMergeConflict(err) {
		t.Errorf("expected IsMergeConflict to be true, got %v", err)
	}
}

func TestTracker_TagRelease(t *testing.T) {
	repoPath := setupTestRepo(t)
	client := NewClient(Config{Enabled: true, RepoPath: repoPath})
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "tracker.json")
	tracker, _ := NewTracker(client, storePath)

	mainBranch, _ := client.CurrentBranch()
	tracker.CreateReleaseBranch("2.0.0", mainBranch)

	err := tracker.TagRelease("2.0.0", "Final 2.0.0")
	if err != nil {
		t.Fatalf("TagRelease failed: %v", err)
	}

	if !client.TagExists("v2.0.0") {
		t.Error("Tag missing in git")
	}
	if tracker.GetReleaseTag("2.0.0") != "v2.0.0" {
		t.Error("Tag missing in tracker state")
	}
}

func TestTracker_StoryMergeInfo(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "tracker.json")
	client := NewClient(Config{Enabled: true})
	tracker, _ := NewTracker(client, storePath)

	info := &MergeInfo{
		MergeCommit: "hash123",
	}
	tracker.RecordStoryMerge("S-001", info)
	
	merge := tracker.GetStoryMerge("S-001")
	if merge == nil || merge.MergeCommit != "hash123" {
		t.Errorf("GetStoryMerge failed: %v", merge)
	}
}

func TestNewTracker_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "non-existent.json")
	client := NewClient(Config{Enabled: false})
	
	// Should not fail if file doesn't exist, just create empty state
	tracker, err := NewTracker(client, storePath)
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}
	if tracker.GetStoryBranch("ANY") != "" {
		t.Error("expected empty state")
	}
}

func TestTracker_GetState(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "state.json")
	client := NewClient(Config{Enabled: true})
	tracker, _ := NewTracker(client, storePath)
	
	tracker.SetStoryBranch("S1", "branch1")
	state := tracker.GetState()
	if state.StoryBranches["S1"] != "branch1" {
		t.Error("GetState missing data")
	}
}

func TestTracker_LoadError(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "bad.json")
	os.WriteFile(storePath, []byte("invalid json"), 0644)
	client := NewClient(Config{Enabled: false})
	
	_, err := NewTracker(client, storePath)
	if err == nil {
		t.Error("expected error loading invalid json")
	}
}

func TestTracker_MergeStory_Empty(t *testing.T) {
	client := NewClient(Config{Enabled: true})
	tracker, _ := NewTracker(client, "tracker.json")
	
	_, err := tracker.MergeStoryToRelease("S-NONE", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "no branch found") {
		t.Errorf("expected 'no branch' error, got %v", err)
	}
}

func TestTracker_TagRelease_Existing(t *testing.T) {
	repoPath := setupTestRepo(t)
	client := NewClient(Config{Enabled: true, RepoPath: repoPath})
	tracker, _ := NewTracker(client, filepath.Join(t.TempDir(), "t.json"))
	
	client.CreateTag("v1.0.0", "Init")
	err := tracker.TagRelease("1.0.0", "Msg")
	if err != nil {
		t.Errorf("TagRelease should return nil if tag exists, got %v", err)
	}
}

func TestTracker_SaveError(t *testing.T) {
	client := NewClient(Config{Enabled: true})
	// Point to a directory that cannot be created
	tracker, _ := NewTracker(client, "/read-only-root/tracker.json")
	err := tracker.LinkCommitToTask("h", "T")
	if err == nil {
		t.Error("expected save error")
	}
}

func TestMergeConflictError(t *testing.T) {
	err := &MergeConflictError{
		StoryID:      "S1",
		SourceBranch: "src",
		TargetBranch: "tgt",
	}
	if !strings.Contains(err.Error(), "merge conflict in story S1") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}
