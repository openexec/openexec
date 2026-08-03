package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

type recordingRepositoryContextPublisher struct {
	called     bool
	projection knowledge.RepositoryContextProjection
}

func (p *recordingRepositoryContextPublisher) Publish(_ context.Context, _, _, _ string, projection knowledge.RepositoryContextProjection) error {
	p.called = true
	p.projection = projection
	return nil
}

func TestPublishRefreshesGraphBeforeSendingRepositoryContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	identity, err := store.EnsureRepositoryIdentity(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	publisher := &recordingRepositoryContextPublisher{}
	projection, err := refreshAndPublishRepositoryContext(context.Background(), store, identity, "https://console.example", "project-1", "token", nil, "", "", "", publisher)
	if err != nil {
		t.Fatal(err)
	}
	if !publisher.called {
		t.Fatal("repository context was not published")
	}
	if projection.GraphVersion == "" || publisher.projection.GraphVersion != projection.GraphVersion {
		t.Fatalf("published graph version = %q, projection = %q", publisher.projection.GraphVersion, projection.GraphVersion)
	}
	state, err := store.CurrentRepositoryState(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if state.GraphVersion != projection.GraphVersion || state.Freshness != knowledge.FreshnessCurrent {
		t.Fatalf("persisted graph state = %#v, projection = %#v", state, projection)
	}
}
