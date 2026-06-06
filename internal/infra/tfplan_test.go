package infra

import (
	"strings"
	"testing"
)

func planJSON(changes string) []byte {
	return []byte(`{"format_version":"1.2","resource_changes":[` + changes + `]}`)
}

func TestDestructiveChanges_DeleteDetected(t *testing.T) {
	got, err := DestructiveChanges(planJSON(
		`{"address":"aws_db_instance.prod","change":{"actions":["delete"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.Contains(got[0], "[delete] aws_db_instance.prod") {
		t.Errorf("got %v", got)
	}
}

func TestDestructiveChanges_ReplacePairLabeled(t *testing.T) {
	// Terraform encodes replace as the ["delete","create"] action pair —
	// there is never a literal "replace" action in the JSON.
	for _, actions := range []string{`["delete","create"]`, `["create","delete"]`} {
		got, err := DestructiveChanges(planJSON(
			`{"address":"aws_instance.web","change":{"actions":` + actions + `}}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !strings.Contains(got[0], "[replace] aws_instance.web") {
			t.Errorf("actions %s: got %v", actions, got)
		}
	}
}

func TestDestructiveChanges_SafeActionsIgnored(t *testing.T) {
	got, err := DestructiveChanges(planJSON(
		`{"address":"a","change":{"actions":["create"]}},` +
			`{"address":"b","change":{"actions":["update"]}},` +
			`{"address":"c","change":{"actions":["no-op"]}},` +
			`{"address":"d","change":{"actions":["read"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("safe actions flagged: %v", got)
	}
}

func TestDestructiveChanges_MalformedIsError(t *testing.T) {
	// An unverifiable plan must be an error, never "no destructive changes".
	if _, err := DestructiveChanges([]byte("not json")); err == nil {
		t.Error("malformed plan json must be an error")
	}
}

func TestDestructiveChanges_EmptyPlan(t *testing.T) {
	got, err := DestructiveChanges([]byte(`{"format_version":"1.2"}`))
	if err != nil || len(got) != 0 {
		t.Errorf("empty plan: got %v, %v", got, err)
	}
}
