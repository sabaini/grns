package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type registeredRoute struct {
	method  string
	path    string
	handler string
}

type boundaryCalls struct {
	service           []string
	attachmentService []string
	gitRefService     []string
	store             []string
}

func TestMutationRoutesUseServiceBoundary(t *testing.T) {
	routes := parseRegisteredRoutes(t)
	handlers := parseServerHandlers(t)

	mutationRoutes := make([]registeredRoute, 0)
	for _, route := range routes {
		if !isMutationMethod(route.method) {
			continue
		}
		if !isTaskMutationPath(route.path) {
			continue
		}
		mutationRoutes = append(mutationRoutes, route)
	}
	if len(mutationRoutes) == 0 {
		t.Fatal("no task mutation routes discovered")
	}

	for _, route := range mutationRoutes {
		fn, ok := handlers[route.handler]
		if !ok {
			t.Fatalf("handler %q for %s %s not found", route.handler, route.method, route.path)
		}
		calls := inspectBoundaryCalls(fn)
		if len(calls.store) > 0 {
			t.Fatalf("handler %q (%s %s) calls s.store directly: %v", route.handler, route.method, route.path, calls.store)
		}
		if len(calls.service) == 0 && len(calls.attachmentService) == 0 && len(calls.gitRefService) == 0 {
			t.Fatalf("handler %q (%s %s) does not call a service boundary", route.handler, route.method, route.path)
		}
	}
}

func parseRegisteredRoutes(t *testing.T) []registeredRoute {
	t.Helper()

	routesPath := filepath.Join(serverPackageDir(t), "routes.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routesPath, nil, 0)
	if err != nil {
		t.Fatalf("parse routes.go: %v", err)
	}

	routes := make([]registeredRoute, 0)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "HandleFunc" || len(call.Args) != 2 {
			return true
		}

		patternLit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || patternLit.Kind != token.STRING {
			return true
		}
		pattern, err := strconv.Unquote(patternLit.Value)
		if err != nil {
			t.Fatalf("unquote route pattern %q: %v", patternLit.Value, err)
		}
		parts := strings.SplitN(pattern, " ", 2)
		if len(parts) != 2 {
			return true
		}

		handlerSel, ok := call.Args[1].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		recv, ok := handlerSel.X.(*ast.Ident)
		if !ok || recv.Name != "s" {
			return true
		}

		routes = append(routes, registeredRoute{
			method:  strings.TrimSpace(parts[0]),
			path:    strings.TrimSpace(parts[1]),
			handler: handlerSel.Sel.Name,
		})
		return true
	})

	return routes
}

func parseServerHandlers(t *testing.T) map[string]*ast.FuncDecl {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(serverPackageDir(t), "handlers*.go"))
	if err != nil {
		t.Fatalf("glob handler files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no handler files found")
	}

	out := make(map[string]*ast.FuncDecl)
	fset := token.NewFileSet()
	for _, filePath := range files {
		file, err := parser.ParseFile(fset, filePath, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filePath, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name == nil || !strings.HasPrefix(fn.Name.Name, "handle") {
				continue
			}
			if !isServerReceiver(fn.Recv) {
				continue
			}
			out[fn.Name.Name] = fn
		}
	}
	return out
}

func inspectBoundaryCalls(fn *ast.FuncDecl) boundaryCalls {
	calls := boundaryCalls{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		chain, ok := selector.X.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		recv, ok := chain.X.(*ast.Ident)
		if !ok || recv.Name != "s" {
			return true
		}

		switch chain.Sel.Name {
		case "service":
			calls.service = append(calls.service, selector.Sel.Name)
		case "attachmentService":
			calls.attachmentService = append(calls.attachmentService, selector.Sel.Name)
		case "gitRefService":
			calls.gitRefService = append(calls.gitRefService, selector.Sel.Name)
		case "store":
			calls.store = append(calls.store, selector.Sel.Name)
		}
		return true
	})
	calls.service = uniqueSorted(calls.service)
	calls.attachmentService = uniqueSorted(calls.attachmentService)
	calls.gitRefService = uniqueSorted(calls.gitRefService)
	calls.store = uniqueSorted(calls.store)
	return calls
}

func isMutationMethod(method string) bool {
	switch method {
	case "POST", "PATCH", "PUT", "DELETE":
		return true
	default:
		return false
	}
}

func isTaskMutationPath(path string) bool {
	return strings.HasPrefix(path, "/v1/projects/")
}

func isServerReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) != 1 {
		return false
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "Server"
}

func serverPackageDir(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
