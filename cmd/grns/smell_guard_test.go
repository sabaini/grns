package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const defaultMaxCmdConstructorLines = 100

func TestCommandConstructorsStaySmall(t *testing.T) {
	maxLines := maxCmdConstructorLines()
	fset := token.NewFileSet()

	for _, path := range commandSourceFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Body == nil {
				continue
			}
			if !isCommandConstructor(fn.Name.Name) {
				continue
			}

			start := fset.Position(fn.Body.Lbrace).Line
			end := fset.Position(fn.Body.Rbrace).Line
			length := end - start + 1
			if length > maxLines {
				t.Fatalf("constructor %s in %s is too large: %d lines (max %d)",
					fn.Name.Name, filepath.Base(path), length, maxLines)
			}
		}
	}
}

func commandSourceFiles(t *testing.T) []string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(self)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	return files
}

func isCommandConstructor(name string) bool {
	return strings.HasPrefix(name, "new") && strings.HasSuffix(name, "Cmd")
}

func maxCmdConstructorLines() int {
	value := strings.TrimSpace(os.Getenv("GRNS_MAX_CMD_CONSTRUCTOR_LINES"))
	if value == "" {
		return defaultMaxCmdConstructorLines
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultMaxCmdConstructorLines
	}
	return parsed
}
