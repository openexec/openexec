// Command coverage-scope inventories complete Go function bodies, including closures.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

type Function struct {
	File  string `json:"file"`
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Body  string `json:"body"`
}

func main() {
	var sources map[string]string
	if err := json.NewDecoder(os.Stdin).Decode(&sources); err != nil {
		panic(err)
	}
	functions := []Function{}
	for path, source := range sources {
		fs := token.NewFileSet()
		file, err := parser.ParseFile(fs, path, source, 0)
		if err != nil {
			panic(err)
		}
		for _, decl := range file.Decls {
			f, ok := decl.(*ast.FuncDecl)
			if !ok || f.Body == nil {
				continue
			}
			name := f.Name.Name
			if f.Recv != nil {
				typ := f.Recv.List[0].Type
				if ptr, ok := typ.(*ast.StarExpr); ok {
					typ = ptr.X
				}
				name = fmt.Sprintf("%s.%s", typ.(*ast.Ident).Name, name)
			}
			start, end := fs.Position(f.Body.Pos()), fs.Position(f.Body.End())
			functions = append(functions, Function{path, name, start.Line, end.Line, source[fs.Position(f.Pos()).Offset:end.Offset]})
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(functions); err != nil {
		panic(err)
	}
}
