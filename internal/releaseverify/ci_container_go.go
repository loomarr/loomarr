package releaseverify

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	postgresImageAuthorityPath = "internal/testkit/postgresimage/image.txt"
	postgresImageOwnerPath     = "internal/testkit/postgresimage/image.go"
	ryukImageAuthorityPath     = "internal/testkit/ryukimage/image.txt"
	postgresImagePackagePath   = "github.com/loomarr/loomarr/internal/testkit/postgresimage"
	testcontainersPackagePath  = "github.com/testcontainers/testcontainers-go"
	testcontainersPostgresPath = "github.com/testcontainers/testcontainers-go/modules/postgres"
	postgresTestImage          = "library/postgres:16-alpine"
	testcontainersRyukImage    = "docker.io/testcontainers/ryuk:0.14.0"
)

type containerRequestKind uint8

const (
	containerRequestUnknown containerRequestKind = iota
	containerRequest
	genericContainerRequest
)

type requestMutationTarget struct {
	requestKind       containerRequestKind
	image             bool
	acquisitionPolicy bool
}

func verifyPostgresImageAuthority(root string, makefile *activeMakefile) error {
	for _, variable := range []string{"POSTGRES_TEST_IMAGE_FILE", "TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX", "TESTCONTAINERS_RYUK_IMAGE"} {
		if len(makefile.variables[variable]) != 0 {
			return fmt.Errorf("container image authority must not use overridable Make variable %s", variable)
		}
	}
	if err := verifyPostgresImageOwner(root); err != nil {
		return err
	}
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(postgresImageAuthorityPath)))
	if err != nil {
		return fmt.Errorf("read postgres image authority: %w", err)
	}
	if fields := strings.Fields(string(contents)); len(fields) != 1 || fields[0] != postgresTestImage {
		return fmt.Errorf("postgres image authority must contain exactly %s", postgresTestImage)
	}
	ryuk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ryukImageAuthorityPath)))
	if err != nil {
		return fmt.Errorf("read testcontainers cleanup image authority: %w", err)
	}
	if fields := strings.Fields(string(ryuk)); len(fields) != 1 || fields[0] != testcontainersRyukImage {
		return fmt.Errorf("testcontainers cleanup image authority must contain exactly %s", testcontainersRyukImage)
	}
	if err := verifyPostgresContainerSeams(root); err != nil {
		return fmt.Errorf("postgres image authority: %w", err)
	}
	return nil
}

func verifyPostgresContainerSeams(root string) error {
	seams := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative != "." && excludedContainerScanDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || filepath.Ext(path) != ".go" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if generatedGoSource(path, source) {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relative, err)
		}
		fileSeams, err := auditGoContainerFile(file, relative)
		if err != nil {
			return err
		}
		seams += fileSeams
		return nil
	})
	if err != nil {
		return err
	}
	if seams == 0 {
		return fmt.Errorf("no postgres testcontainers callsites found")
	}
	return nil
}

func excludedContainerScanDirectory(name string) bool {
	switch name {
	case ".git", ".worktrees", ".artifacts", ".cache", ".gocache", "node_modules", "vendor", "generated":
		return true
	default:
		return false
	}
}

func generatedGoSource(path string, source []byte) bool {
	name := filepath.Base(path)
	if strings.HasSuffix(name, ".gen.go") || strings.HasSuffix(name, "_generated.go") {
		return true
	}
	prefix := source
	if len(prefix) > 512 {
		prefix = prefix[:512]
	}
	return bytes.Contains(prefix, []byte("Code generated")) && bytes.Contains(prefix, []byte("DO NOT EDIT"))
}

type goContainerAudit struct {
	relative           string
	postgresAlias      string
	testcontainers     string
	authorityAlias     string
	requestAliases     map[string]containerRequestKind
	requestVariables   map[string]containerRequestKind
	requestPointers    map[string]requestMutationTarget
	postgresRunRefs    int
	postgresRunCalls   int
	genericRunRefs     int
	genericRunCalls    int
	genericCreateRefs  int
	genericCreateCalls int
	seams              int
	contractErr        error
}

func auditGoContainerFile(file *ast.File, relative string) (int, error) {
	if err := rejectPostgresImageLiterals(file, relative); err != nil {
		return 0, err
	}
	audit := &goContainerAudit{
		relative:         relative,
		postgresAlias:    goImportAlias(file, testcontainersPostgresPath),
		testcontainers:   goImportAlias(file, testcontainersPackagePath),
		authorityAlias:   goImportAlias(file, postgresImagePackagePath),
		requestAliases:   make(map[string]containerRequestKind),
		requestVariables: make(map[string]containerRequestKind),
		requestPointers:  make(map[string]requestMutationTarget),
	}
	if audit.postgresAlias == "." || audit.testcontainers == "." || audit.authorityAlias == "." {
		return 0, fmt.Errorf("%s must not dot-import container authority packages", relative)
	}
	if shadowed := shadowedContainerImportAlias(file, audit.postgresAlias, audit.testcontainers, audit.authorityAlias); shadowed != "" {
		return 0, fmt.Errorf("%s must not shadow container authority import %s", relative, shadowed)
	}
	audit.discoverRequestAliases(file)
	audit.discoverRequestVariables(file)
	ast.Inspect(file, audit.inspect)
	if audit.contractErr != nil {
		return 0, audit.contractErr
	}
	if audit.postgresRunRefs != audit.postgresRunCalls || audit.genericRunRefs != audit.genericRunCalls || audit.genericCreateRefs != audit.genericCreateCalls {
		return 0, fmt.Errorf("%s must call testcontainers acquisition functions directly", relative)
	}
	return audit.seams, nil
}

func shadowedContainerImportAlias(file *ast.File, aliases ...string) string {
	protected := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		if alias != "" && alias != "." && alias != "_" {
			protected[alias] = struct{}{}
		}
	}
	var shadowed string
	check := func(identifier *ast.Ident) {
		if identifier == nil || shadowed != "" {
			return
		}
		if _, protectedName := protected[identifier.Name]; protectedName {
			shadowed = identifier.Name
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if shadowed != "" {
			return false
		}
		switch typed := node.(type) {
		case *ast.AssignStmt:
			if typed.Tok == token.DEFINE {
				for _, expression := range typed.Lhs {
					identifier, _ := expression.(*ast.Ident)
					check(identifier)
				}
			}
		case *ast.RangeStmt:
			if typed.Tok == token.DEFINE {
				for _, expression := range []ast.Expr{typed.Key, typed.Value} {
					identifier, _ := expression.(*ast.Ident)
					check(identifier)
				}
			}
		case *ast.ValueSpec:
			for _, name := range typed.Names {
				check(name)
			}
		case *ast.Field:
			for _, name := range typed.Names {
				check(name)
			}
		}
		return shadowed == ""
	})
	return shadowed
}

func (audit *goContainerAudit) discoverRequestAliases(file *ast.File) {
	changed := true
	for changed {
		changed = false
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if audit.requestAliases[typeSpec.Name.Name] != containerRequestUnknown {
					continue
				}
				if kind := audit.requestKind(typeSpec.Type); kind != containerRequestUnknown {
					audit.requestAliases[typeSpec.Name.Name] = kind
					changed = true
				}
			}
		}
	}
}

func (audit *goContainerAudit) discoverRequestVariables(file *ast.File) {
	changed := true
	for changed {
		changed = false
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.ValueSpec:
				declaredKind := audit.requestKind(typed.Type)
				for index, name := range typed.Names {
					kind := declaredKind
					if kind == containerRequestUnknown && index < len(typed.Values) {
						kind = audit.requestExpressionKind(typed.Values[index])
					}
					if kind != containerRequestUnknown && audit.requestVariables[name.Name] == containerRequestUnknown {
						audit.requestVariables[name.Name] = kind
						changed = true
					}
				}
				if len(typed.Names) == len(typed.Values) {
					for index, name := range typed.Names {
						if _, known := audit.requestPointers[name.Name]; known {
							continue
						}
						if target, pointer := audit.requestMutationPointer(typed.Values[index]); pointer {
							if typed.Type == nil || pointerType(typed.Type) {
								audit.requestPointers[name.Name] = target
								changed = true
							}
						}
					}
				}
				if pointerType(typed.Type) {
					if star, ok := typed.Type.(*ast.StarExpr); ok {
						if kind := audit.requestKind(star.X); kind != containerRequestUnknown {
							for _, name := range typed.Names {
								if _, known := audit.requestPointers[name.Name]; !known {
									audit.requestPointers[name.Name] = requestMutationTarget{requestKind: kind}
									changed = true
								}
							}
						}
					}
				}
			case *ast.AssignStmt:
				for index, left := range typed.Lhs {
					name, ok := left.(*ast.Ident)
					if !ok || index >= len(typed.Rhs) || audit.requestVariables[name.Name] != containerRequestUnknown {
						continue
					}
					if kind := audit.requestExpressionKind(typed.Rhs[index]); kind != containerRequestUnknown {
						audit.requestVariables[name.Name] = kind
						changed = true
					}
				}
				if len(typed.Lhs) == len(typed.Rhs) {
					for index, left := range typed.Lhs {
						name, ok := left.(*ast.Ident)
						if !ok {
							continue
						}
						if _, known := audit.requestPointers[name.Name]; known {
							continue
						}
						if typed.Tok != token.DEFINE {
							continue
						}
						if target, pointer := audit.requestMutationPointer(typed.Rhs[index]); pointer {
							audit.requestPointers[name.Name] = target
							changed = true
						}
					}
				}
			}
			return true
		})
	}
}

func (audit *goContainerAudit) inspect(node ast.Node) bool {
	if audit.contractErr != nil {
		return false
	}
	switch typed := node.(type) {
	case *ast.SelectorExpr:
		if isPackageSelector(typed, audit.testcontainers, "ParallelContainers") {
			audit.fail("testcontainers.ParallelContainers bypasses the structurally audited request seam")
			return false
		}
		if audit.testcontainers != "" && isDockerProviderAcquisitionMethod(typed.Sel.Name) {
			// Go syntax does not carry receiver type information. In a file that
			// imports testcontainers, these exported provider methods are therefore
			// rejected conservatively instead of accepting an opaque request through
			// an alias or interface value.
			audit.fail("testcontainers DockerProvider acquisition methods bypass the structurally audited request seam")
			return false
		}
		if isPackageSelector(typed, audit.postgresAlias, "RunContainer") {
			audit.fail("deprecated postgres.RunContainer bypasses the pinned image authority")
			return false
		}
		if isPackageSelector(typed, audit.postgresAlias, "Run") {
			audit.postgresRunRefs++
		}
		if isPackageSelector(typed, audit.testcontainers, "Run") {
			audit.genericRunRefs++
		}
		if isPackageSelector(typed, audit.testcontainers, "GenericContainer") {
			audit.genericCreateRefs++
		}
	case *ast.CallExpr:
		audit.inspectCall(typed)
	case *ast.CompositeLit:
		if audit.requestPointerEscapesComposite(typed) {
			audit.fail("tracked container request pointer must not escape into an aggregate")
			return false
		}
		if kind := audit.requestKind(typed.Type); kind != containerRequestUnknown {
			audit.inspectRequestComposite(typed, kind)
		}
	case *ast.AssignStmt:
		if audit.requestPointerEscapesAssignment(typed) {
			audit.fail("tracked container request pointer must not escape through assignment")
			return false
		}
		audit.inspectRequestAssignment(typed)
	case *ast.RangeStmt:
		audit.inspectRequestRange(typed)
	case *ast.ValueSpec:
		if audit.requestPointerEscapesValueSpec(typed) {
			audit.fail("tracked container request pointer must not escape through a declaration")
			return false
		}
		audit.inspectRequestValues(typed)
	case *ast.ReturnStmt:
		for _, result := range typed.Results {
			if audit.isRequestMutationPointer(result) {
				audit.fail("tracked container request pointer must not escape through return")
				return false
			}
		}
	case *ast.SendStmt:
		if audit.isRequestMutationPointer(typed.Value) {
			audit.fail("tracked container request pointer must not escape through channel send")
			return false
		}
	}
	return audit.contractErr == nil
}

func isDockerProviderAcquisitionMethod(name string) bool {
	switch name {
	case "CreateContainer", "ReuseOrCreateContainer", "RunContainer":
		return true
	default:
		return false
	}
}

func pointerType(expression ast.Expr) bool {
	_, ok := expression.(*ast.StarExpr)
	return ok
}

func (audit *goContainerAudit) requestPointerEscapesComposite(composite *ast.CompositeLit) bool {
	for _, element := range composite.Elts {
		if field, ok := element.(*ast.KeyValueExpr); ok {
			if audit.isRequestMutationPointer(field.Key) || audit.isRequestMutationPointer(field.Value) {
				return true
			}
			continue
		}
		if audit.isRequestMutationPointer(element) {
			return true
		}
	}
	return false
}

func (audit *goContainerAudit) requestPointerEscapesAssignment(assignment *ast.AssignStmt) bool {
	if len(assignment.Lhs) != len(assignment.Rhs) {
		for _, expression := range assignment.Rhs {
			if audit.isRequestMutationPointer(expression) {
				return true
			}
		}
		return false
	}
	for index, right := range assignment.Rhs {
		if !audit.isRequestMutationPointer(right) {
			continue
		}
		identifier, directAlias := assignment.Lhs[index].(*ast.Ident)
		if !directAlias {
			return true
		}
		if assignment.Tok == token.DEFINE {
			continue
		}
		if _, trackedAlias := audit.requestPointers[identifier.Name]; !trackedAlias {
			return true
		}
	}
	return false
}

func (audit *goContainerAudit) requestPointerEscapesValueSpec(spec *ast.ValueSpec) bool {
	for _, value := range spec.Values {
		if audit.isRequestMutationPointer(value) && spec.Type != nil && !pointerType(spec.Type) {
			return true
		}
	}
	return false
}

func (audit *goContainerAudit) inspectCall(call *ast.CallExpr) {
	if audit.requestPointerEscapesCall(call) {
		audit.fail("tracked container request pointer must not escape to an opaque function")
		return
	}
	switch {
	case isPackageSelector(call.Fun, audit.postgresAlias, "Run"):
		audit.postgresRunCalls++
		if len(call.Args) < 2 || !audit.authorityCall(call.Args[1]) {
			audit.fail("postgres.Run must use postgresimage.Name()")
			return
		}
		if !audit.allowedPostgresCustomizers(call) {
			audit.fail("postgres.Run customizers must use the closed non-acquisition policy")
			return
		}
		audit.seams++
	case isPackageSelector(call.Fun, audit.testcontainers, "Run"):
		audit.genericRunCalls++
		if len(call.Args) < 2 {
			audit.fail("testcontainers.Run must declare an audited image")
			return
		}
		audit.inspectImage(call.Args[1], "testcontainers.Run image")
		if len(call.Args) != 2 || call.Ellipsis.IsValid() {
			audit.fail("testcontainers.Run must not accept unaudited customizers")
		}
	case isPackageSelector(call.Fun, audit.testcontainers, "GenericContainer"):
		audit.genericCreateCalls++
		if len(call.Args) < 2 || !audit.auditedGenericRequest(call.Args[1]) {
			audit.fail("testcontainers.GenericContainer must use a structurally audited request")
		}
	}
}
