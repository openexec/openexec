package knowledge

import (
	"context"
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Type-checked Go extraction: the same answer an editor gives.
//
// The AST pass can see that a file mentions the word Error; it cannot see which
// of eighteen methods named Error was meant, because that depends on the type
// of the receiver. Matching by spelling made every one of them share every
// mention, which is how a repository of 11675 symbols produced 565919
// reference edges and a "most depended-on symbol" that was whichever name
// recurred most.
//
// go/packages runs the real type checker, so every identifier resolves to the
// object it actually names, with that object's declaring file. References then
// carry TargetPath and match exactly — the same mechanism the TypeScript
// compiler path already uses.
//
// This requires the module to type-check, which requires its dependencies to
// be present. When that does not hold the caller keeps the AST result: a
// coarser graph is the honest fallback, and a repository that cannot build is
// not one to make confident claims about anyway.

const goTypedPackageLimit = 2000

// errTypedUnavailable means the module did not type-check well enough to trust
// its resolution — not that anything went wrong. The caller keeps the coarser
// AST result and records the grade.
var errTypedUnavailable = errors.New("module did not type-check")

// goTypedReferences returns, per repo-relative file, the references that file
// makes to symbols declared elsewhere in the same module.
func goTypedReferences(ctx context.Context, root string) (map[string][]ExtractedReference, error) {
	config := &packages.Config{
		Context: ctx,
		Dir:     root,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		// Tests carry real call edges — a test is one of the few callers a
		// helper may have, and pretending otherwise is how tests get deleted.
		Tests: true,
	}
	loaded, err := packages.Load(config, "./...")
	if err != nil {
		return nil, err
	}
	if len(loaded) == 0 || len(loaded) > goTypedPackageLimit {
		return nil, errTypedUnavailable
	}
	// A package that failed to type-check yields no Uses map; if that is most
	// of them the module is not really building and the AST result is safer.
	typed := 0
	for _, pkg := range loaded {
		if pkg.TypesInfo != nil && len(pkg.TypesInfo.Uses) > 0 {
			typed++
		}
	}
	if typed == 0 {
		return nil, errTypedUnavailable
	}

	out := map[string][]ExtractedReference{}
	seen := map[string]bool{}
	for _, pkg := range loaded {
		if pkg.TypesInfo == nil || pkg.Fset == nil {
			continue
		}
		for ident, object := range pkg.TypesInfo.Uses {
			reference, from, ok := typedReference(pkg.Fset, root, ident, object)
			if !ok {
				continue
			}
			// The same identifier is reachable through both a package and its
			// test variant; recording it twice would double every count.
			key := from + "\x00" + reference.TargetPath + "\x00" + reference.TargetName + "\x00" +
				string(rune(reference.StartByte))
			if seen[key] {
				continue
			}
			seen[key] = true
			out[from] = append(out[from], reference)
		}
	}
	return out, nil
}

func typedReference(fset *token.FileSet, root string, ident *ast.Ident, object types.Object) (ExtractedReference, string, bool) {
	if object == nil || !object.Pos().IsValid() {
		return ExtractedReference{}, "", false
	}
	// Only objects a package declares are symbols in this graph: locals,
	// parameters and struct fields are not things another file depends on.
	switch object.(type) {
	case *types.Func, *types.TypeName, *types.Const, *types.Var:
	default:
		return ExtractedReference{}, "", false
	}
	if variable, isVar := object.(*types.Var); isVar && !variable.Exported() && variable.Parent() != nil &&
		variable.Parent() != object.Pkg().Scope() {
		return ExtractedReference{}, "", false
	}
	use := fset.Position(ident.Pos())
	declaration := fset.Position(object.Pos())
	if !use.IsValid() || !declaration.IsValid() {
		return ExtractedReference{}, "", false
	}
	from, insideUse := repoRelative(root, use.Filename)
	target, insideTarget := repoRelative(root, declaration.Filename)
	if !insideUse || !insideTarget {
		return ExtractedReference{}, "", false
	}
	if from == target && use.Offset == declaration.Offset {
		return ExtractedReference{}, "", false // the declaration naming itself
	}
	edge := "references"
	if _, isFunc := object.(*types.Func); isFunc {
		edge = "calls"
	}
	return ExtractedReference{
		TargetName:          object.Name(),
		TargetPath:          target,
		TargetStartByte:     declaration.Offset,
		TargetPositionKnown: true,
		StartByte:           use.Offset,
		EndByte:             use.Offset + len(ident.Name),
		EdgeType:            edge,
		Resolution:          ResolutionCompilerExact,
	}, from, true
}

func repoRelative(root, path string) (string, bool) {
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return "", false
	}
	return filepath.ToSlash(relative), true
}
