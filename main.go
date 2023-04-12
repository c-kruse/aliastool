package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"strconv"
	"strings"
)

func main() {
	fset := token.NewFileSet()
	node, err := parser.ParseDir(fset, "/home/christian/sensu/sensu-go/types", nil, parser.ParseComments)
	if err != nil {
		fmt.Println(err)
		return
	}
	pkg := node["types"]
	type resource struct {
		Name       string
		ImportName string
		ImportPath string
	}
	aliases := make(map[string]resource)
	for _, file := range pkg.Files {
		imports := make(map[string]*ast.BasicLit)
		fileAliases := make(map[string]*ast.SelectorExpr)
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.ValueSpec:
				for i, ns := range n.Names {
					if len(n.Values) <= i {
						break
					}
					vs := n.Values[i]
					if expr, ok := vs.(*ast.SelectorExpr); ok {
						fileAliases[ns.Name] = expr
					}
				}
			case *ast.TypeSpec:
				if !n.Name.IsExported() {
					break
				}
				if expr, ok := n.Type.(*ast.SelectorExpr); ok {
					fileAliases[n.Name.Name] = expr
				}
			case *ast.ImportSpec:
				var name string
				if n.Name != nil {
					name = n.Name.Name
				} else {
					name = strings.Trim(path.Base(n.Path.Value), "\"")
				}
				imports[name] = n.Path
			}
			return true
		})
		for name, sel := range fileAliases {
			xid, ok := sel.X.(*ast.Ident)
			if !ok {
				fmt.Printf("unepxected selector expression: %T\n", sel)
				return
			}
			if i, ok := imports[xid.Name]; ok {
				aliases[name] = resource{
					Name:       sel.Sel.Name,
					ImportName: xid.Name,
					ImportPath: strings.Trim(i.Value, "\""),
				}
			}
		}
	}
	fset = token.NewFileSet()
	node, err = parser.ParseDir(fset, os.Args[1], nil, parser.ParseComments)
	if err != nil {
		fmt.Println(err)
		return
	}
	for pname, pkg := range node {
		for fname, file := range pkg.Files {
			var changed bool
			var importName string
			imports := make(map[string]string)
			toAdd := make(map[string]string)
			ast.Inspect(file, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.ImportSpec:
					var name string
					p := strings.Trim(n.Path.Value, "\"")
					if n.Name != nil {
						name = n.Name.Name
					} else {
						name = path.Base(p)
					}
					imports[p] = name
					if p == "github.com/sensu/sensu-go/types" {
						importName = name
					}
				case *ast.SelectorExpr:
					if importName == "" {
						break
					}
					if xid, ok := n.X.(*ast.Ident); ok {
						if xid.Name != importName {
							break
						}

						if r, ok := aliases[n.Sel.Name]; ok {
							fmt.Printf("editing %s %s: replacing %+v, with %+v\n", pname, fname, n, r)
							importName := r.ImportName
							if n, ok := imports[r.ImportPath]; ok {
								importName = n
							} else {
								toAdd[r.ImportPath] = r.ImportName
							}
							xid.Name = importName
							n.Sel.Name = r.Name
							changed = true
						}
					}
				}
				return true
			})
			if len(toAdd) > 0 {
				ast.Inspect(file, func(node ast.Node) bool {
					switch n := node.(type) {
					case *ast.GenDecl:
						if n.Tok == token.IMPORT {
							for importPath, iName := range toAdd {
								iSpec := &ast.ImportSpec{Path: &ast.BasicLit{Value: strconv.Quote(importPath)}}
								if path.Base(importPath) != iName {
									iSpec.Name = &ast.Ident{Name: iName}
								}
								n.Specs = append(n.Specs, iSpec)
							}
							return false
						}
					}
					return true
				})
			}

			if changed {
				f, err := os.Create(fname)
				if err != nil {
					fmt.Println("error rewriting file", err)
					return
				}
				if err := printer.Fprint(f, fset, file); err != nil {
					fmt.Println("error rewriting file", err)
					return
				}
			}
		}
	}
}
