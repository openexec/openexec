package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The whole point of type checking: two methods named Error are different
// symbols, and a call site names exactly one of them. Matching by spelling gave
// all eighteen of openexec's Error methods every mention of the word.
func TestGoTypedReferencesDistinguishSameNamedMethods(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module sample\n\ngo 1.21\n")
	writeTestFile(t, root, "alpha/alpha.go",
		"package alpha\n\ntype Failure struct{}\n\nfunc (Failure) Error() string { return \"alpha\" }\n")
	writeTestFile(t, root, "beta/beta.go",
		"package beta\n\ntype Failure struct{}\n\nfunc (Failure) Error() string { return \"beta\" }\n")
	// Calls alpha's Error and nothing of beta's.
	writeTestFile(t, root, "main.go",
		"package main\n\nimport \"sample/alpha\"\n\nfunc main() {\n\tvar f alpha.Failure\n\t_ = f.Error()\n}\n")

	references, err := goTypedReferences(context.Background(), root)
	if err != nil {
		t.Skipf("module did not type-check in this environment: %v", err)
	}
	var toAlpha, toBeta int
	for _, list := range references {
		for _, reference := range list {
			if reference.TargetName != "Error" {
				continue
			}
			switch reference.TargetPath {
			case "alpha/alpha.go":
				toAlpha++
			case "beta/beta.go":
				toBeta++
			}
		}
	}
	if toAlpha == 0 {
		t.Fatalf("the call to alpha's Error was not resolved: %#v", references)
	}
	if toBeta != 0 {
		t.Fatalf("a call to alpha's Error was attributed to beta's: %d", toBeta)
	}
}

// A repository whose dependencies are absent cannot be type-checked, and the
// honest answer is the coarser graph rather than a confident wrong one.
func TestGoTypedReferencesDegradeWhenTheModuleDoesNotBuild(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module sample\n\ngo 1.21\n")
	writeTestFile(t, root, "broken.go", "package main\n\nimport \"example.invalid/absent\"\n\nfunc main() { absent.Gone() }\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	references, err := goTypedReferences(context.Background(), root)
	if err == nil && len(references) > 0 {
		for _, list := range references {
			for _, reference := range list {
				if reference.Resolution == ResolutionCompilerExact && reference.TargetName == "Gone" {
					t.Fatal("an unresolvable import produced a compiler-exact reference")
				}
			}
		}
	}
}
