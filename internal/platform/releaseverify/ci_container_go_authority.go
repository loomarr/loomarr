package releaseverify

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func verifyPostgresImageOwner(root string) error {
	path := filepath.Join(root, filepath.FromSlash(postgresImageOwnerPath))
	if err := verifyPostgresImagePackageFiles(filepath.Dir(path)); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect postgres image owner: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("postgres image owner must be a regular non-symlink file")
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse postgres image owner: %w", err)
	}
	if file.Name.Name != "postgresimage" {
		return fmt.Errorf("postgres image owner must use package postgresimage")
	}
	if postgresImageBuildConstrained(file) {
		return fmt.Errorf("postgres image owner must be untagged on every supported platform")
	}
	if postgresImageEmbedDirectiveCount(file) != 1 {
		return fmt.Errorf("postgres image owner must contain exactly one go:embed image.txt directive")
	}
	if !exactPostgresImageOwnerImports(file) {
		return fmt.Errorf("postgres image owner must import only embed and strings through their audited forms")
	}

	var imageDeclaration *ast.GenDecl
	var nameFunction *ast.FuncDecl
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.GenDecl:
			if typed.Tok == token.IMPORT {
				continue
			}
			if typed.Tok != token.VAR || imageDeclaration != nil {
				return fmt.Errorf("postgres image owner contains an unaudited declaration")
			}
			imageDeclaration = typed
		case *ast.FuncDecl:
			if nameFunction != nil {
				return fmt.Errorf("postgres image owner contains more than one function")
			}
			nameFunction = typed
		default:
			return fmt.Errorf("postgres image owner contains an unaudited declaration")
		}
	}
	if !exactEmbeddedPostgresImage(imageDeclaration) {
		return fmt.Errorf("postgres image owner must embed only image.txt into the private image string")
	}
	if !exactPostgresImageNameFunction(nameFunction) {
		return fmt.Errorf("postgres image owner Name must return only strings.TrimSpace(image)")
	}
	return nil
}

func verifyPostgresImagePackageFiles(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read postgres image owner package: %w", err)
	}
	productionFiles := 0
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		productionFiles++
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || name != filepath.Base(postgresImageOwnerPath) {
			return fmt.Errorf("postgres image owner package must contain only the untagged production file %s", filepath.Base(postgresImageOwnerPath))
		}
	}
	if productionFiles != 1 {
		return fmt.Errorf("postgres image owner package must contain exactly one production Go owner")
	}
	return nil
}

func postgresImageBuildConstrained(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			text := strings.TrimSpace(comment.Text)
			if strings.HasPrefix(text, "//go:build") || strings.HasPrefix(text, "// +build") {
				return true
			}
		}
	}
	return false
}

func postgresImageEmbedDirectiveCount(file *ast.File) int {
	count := 0
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.TrimSpace(comment.Text) == "//go:embed image.txt" {
				count++
			}
		}
	}
	return count
}

func exactPostgresImageOwnerImports(file *ast.File) bool {
	if len(file.Imports) != 2 {
		return false
	}
	seenEmbed, seenStrings := false, false
	for _, specification := range file.Imports {
		path, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			return false
		}
		switch path {
		case "embed":
			if specification.Name == nil || specification.Name.Name != "_" || seenEmbed {
				return false
			}
			seenEmbed = true
		case "strings":
			if specification.Name != nil || seenStrings {
				return false
			}
			seenStrings = true
		default:
			return false
		}
	}
	return seenEmbed && seenStrings
}

func exactEmbeddedPostgresImage(declaration *ast.GenDecl) bool {
	if declaration == nil || declaration.Doc == nil || len(declaration.Doc.List) != 1 || declaration.Doc.List[0].Text != "//go:embed image.txt" || len(declaration.Specs) != 1 {
		return false
	}
	specification, ok := declaration.Specs[0].(*ast.ValueSpec)
	if !ok || len(specification.Names) != 1 || specification.Names[0].Name != "image" || len(specification.Values) != 0 {
		return false
	}
	typeName, ok := specification.Type.(*ast.Ident)
	return ok && typeName.Name == "string"
}

func exactPostgresImageNameFunction(function *ast.FuncDecl) bool {
	if function == nil || function.Recv != nil || function.Name.Name != "Name" || function.Type.TypeParams != nil ||
		len(function.Type.Params.List) != 0 || function.Type.Results == nil || len(function.Type.Results.List) != 1 || len(function.Body.List) != 1 {
		return false
	}
	resultType, ok := function.Type.Results.List[0].Type.(*ast.Ident)
	if !ok || resultType.Name != "string" || len(function.Type.Results.List[0].Names) != 0 {
		return false
	}
	statement, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) != 1 {
		return false
	}
	call, ok := statement.Results[0].(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() || len(call.Args) != 1 || !isPackageSelector(call.Fun, "strings", "TrimSpace") {
		return false
	}
	argument, ok := call.Args[0].(*ast.Ident)
	return ok && argument.Name == "image"
}
