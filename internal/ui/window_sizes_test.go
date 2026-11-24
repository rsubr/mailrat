package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Every window or dialog the app opens with a literal Resize(fyne.NewSize(w, h))
// must fit the 1470x918 display target (main window, compose, settings,
// contacts, login and dialogs).
func TestDefaultWindowSizesFitDisplay(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Resize" {
				return true
			}
			ns, ok := call.Args[0].(*ast.CallExpr)
			if !ok || len(ns.Args) != 2 {
				return true
			}
			if s, ok := ns.Fun.(*ast.SelectorExpr); !ok || s.Sel.Name != "NewSize" {
				return true
			}
			w, h := numLit(ns.Args[0]), numLit(ns.Args[1])
			if w < 0 || h < 0 {
				return true // computed size, not a default
			}
			found++
			if w > float64(uxWindowW) || h > float64(uxWindowH) {
				t.Errorf("%s: default size %vx%v exceeds %vx%v", fset.Position(call.Pos()), w, h, uxWindowW, uxWindowH)
			}
			return true
		})
	}
	if found < 5 {
		t.Fatalf("only %d literal default sizes found; scan is broken", found)
	}
}

func numLit(e ast.Expr) float64 {
	bl, ok := e.(*ast.BasicLit)
	if !ok {
		return -1
	}
	v, err := strconv.ParseFloat(bl.Value, 64)
	if err != nil {
		return -1
	}
	return v
}
