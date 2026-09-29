// Command retentioncoverage inventories Go function bodies without line-diff heuristics.
package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
)

type function struct {
	Name   string         `json:"name"`
	Start  token.Position `json:"start"`
	End    token.Position `json:"end"`
	Source string         `json:"source"`
}

func main() {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, os.Args[1], nil, 0)
	if err != nil {
		panic(err)
	}
	result := []function{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		if fn.Recv != nil {
			var receiver bytes.Buffer
			if err := format.Node(&receiver, fs, fn.Recv.List[0].Type); err != nil {
				panic(err)
			}
			name = "(" + receiver.String() + ")." + name
		}
		var source bytes.Buffer
		if err := format.Node(&source, fs, fn); err != nil {
			panic(err)
		}
		result = append(result, function{name, fs.Position(fn.Body.Pos()), fs.Position(fn.Body.End()), source.String()})
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		panic(err)
	}
}
