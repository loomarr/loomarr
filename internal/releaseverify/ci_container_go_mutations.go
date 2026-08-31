package releaseverify

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

func (audit *goContainerAudit) allowedPostgresCustomizers(call *ast.CallExpr) bool {
	if call.Ellipsis.IsValid() {
		return false
	}
	for _, expression := range call.Args[2:] {
		customizer, ok := unwrapParentheses(expression).(*ast.CallExpr)
		if !ok || customizer.Ellipsis.IsValid() {
			return false
		}
		switch {
		case isPackageSelector(customizer.Fun, audit.postgresAlias, "WithDatabase"):
		case isPackageSelector(customizer.Fun, audit.postgresAlias, "WithUsername"):
		case isPackageSelector(customizer.Fun, audit.postgresAlias, "WithPassword"):
		case isPackageSelector(customizer.Fun, audit.testcontainers, "WithWaitStrategy"):
		default:
			return false
		}
	}
	return true
}

func (audit *goContainerAudit) requestPointerEscapesCall(call *ast.CallExpr) bool {
	for _, argument := range call.Args {
		if audit.isRequestMutationPointer(argument) {
			return true
		}
	}
	if selector, ok := unwrapParentheses(call.Fun).(*ast.SelectorExpr); ok {
		return audit.isRequestMutationPointer(selector.X)
	}
	return false
}

// The Postgres image authority is deliberately value-returning. There is no
// approved pointer-taking authority seam, so every pointer to an audited
// request or Image passed across a call boundary is opaque and rejected.
func (audit *goContainerAudit) isRequestMutationPointer(expression ast.Expr) bool {
	_, pointer := audit.requestMutationPointer(expression)
	return pointer
}

func (audit *goContainerAudit) requestMutationPointer(expression ast.Expr) (requestMutationTarget, bool) {
	unwrapped := unwrapParentheses(expression)
	if identifier, ok := unwrapped.(*ast.Ident); ok {
		target, tracked := audit.requestPointers[identifier.Name]
		return target, tracked
	}
	address, ok := unwrapped.(*ast.UnaryExpr)
	if !ok || address.Op != token.AND {
		return requestMutationTarget{}, false
	}
	return audit.classifyRequestMutationTarget(address.X)
}

func (audit *goContainerAudit) inspectRequestComposite(composite *ast.CompositeLit, kind containerRequestKind) {
	if kind == genericContainerRequest {
		for _, element := range composite.Elts {
			field, ok := element.(*ast.KeyValueExpr)
			if !ok || fieldName(field.Key) != "ContainerRequest" {
				continue
			}
			if !audit.auditedContainerRequest(field.Value) {
				audit.fail("GenericContainerRequest.ContainerRequest must be structurally audited")
			}
		}
		return
	}
	for _, element := range composite.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch fieldName(field.Key) {
		case "Image":
			audit.inspectImage(field.Value, "testcontainers.ContainerRequest Image")
		case "AlwaysPullImage", "ImageSubstitutors":
			audit.fail("testcontainers.ContainerRequest must not override bounded image acquisition policy")
		}
	}
}

func (audit *goContainerAudit) inspectRequestAssignment(assignment *ast.AssignStmt) {
	for index, left := range assignment.Lhs {
		target, tracked := audit.classifyRequestMutationTarget(left)
		if !tracked {
			continue
		}
		if assignment.Tok != token.ASSIGN && assignment.Tok != token.DEFINE {
			audit.fail("tracked container request must not use compound assignment")
			return
		}
		if len(assignment.Lhs) != len(assignment.Rhs) {
			audit.fail("tracked container request must not receive an unmatched multi-result value")
			return
		}
		if target.image {
			audit.inspectImage(assignment.Rhs[index], "post-construction ContainerRequest.Image")
			continue
		}
		if target.acquisitionPolicy {
			audit.fail("post-construction ContainerRequest must not override bounded image acquisition policy")
			return
		}
		if audit.requestExpressionKind(assignment.Rhs[index]) != target.requestKind {
			audit.fail("container request variable must not receive an opaque value")
		}
	}
}

func (audit *goContainerAudit) inspectRequestRange(statement *ast.RangeStmt) {
	if statement.Tok != token.ASSIGN {
		return
	}
	for _, expression := range []ast.Expr{statement.Key, statement.Value} {
		if _, tracked := audit.classifyRequestMutationTarget(expression); tracked {
			audit.fail("tracked container request must not receive an opaque range value")
			return
		}
	}
}

func (audit *goContainerAudit) classifyRequestMutationTarget(expression ast.Expr) (requestMutationTarget, bool) {
	if expression == nil {
		return requestMutationTarget{}, false
	}
	unwrapped := unwrapParentheses(expression)
	if dereference, ok := unwrapped.(*ast.StarExpr); ok {
		if target, pointer := audit.requestMutationPointer(dereference.X); pointer {
			return target, true
		}
	}
	if selector, ok := unwrapped.(*ast.SelectorExpr); ok && audit.requestExpressionKind(selector.X) == containerRequest {
		switch selector.Sel.Name {
		case "Image":
			return requestMutationTarget{requestKind: containerRequest, image: true}, true
		case "AlwaysPullImage", "ImageSubstitutors":
			return requestMutationTarget{requestKind: containerRequest, acquisitionPolicy: true}, true
		}
	}
	if kind := audit.requestExpressionKind(expression); kind != containerRequestUnknown {
		return requestMutationTarget{requestKind: kind}, true
	}
	return requestMutationTarget{}, false
}

func unwrapParentheses(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

func (audit *goContainerAudit) inspectRequestValues(spec *ast.ValueSpec) {
	for index, name := range spec.Names {
		if audit.requestVariables[name.Name] == containerRequestUnknown || index >= len(spec.Values) {
			continue
		}
		if audit.requestExpressionKind(spec.Values[index]) != audit.requestVariables[name.Name] {
			audit.fail("container request variable must not be initialized opaquely")
		}
	}
}

func (audit *goContainerAudit) inspectImage(expression ast.Expr, seam string) {
	if audit.authorityCall(expression) {
		audit.seams++
		return
	}
	if image, constant := goStringConstant(expression); constant && !isPostgresImageReference(image) {
		return
	}
	audit.fail("opaque or Postgres " + seam + " must use postgresimage.Name()")
}

func (audit *goContainerAudit) auditedContainerRequest(expression ast.Expr) bool {
	switch typed := expression.(type) {
	case *ast.CompositeLit:
		if audit.requestKind(typed.Type) != containerRequest {
			return false
		}
		audit.inspectRequestComposite(typed, containerRequest)
		return audit.contractErr == nil
	case *ast.Ident:
		return audit.requestVariables[typed.Name] == containerRequest
	default:
		return false
	}
}

func (audit *goContainerAudit) auditedGenericRequest(expression ast.Expr) bool {
	switch typed := expression.(type) {
	case *ast.CompositeLit:
		if audit.requestKind(typed.Type) != genericContainerRequest {
			return false
		}
		audit.inspectRequestComposite(typed, genericContainerRequest)
		return audit.contractErr == nil
	case *ast.Ident:
		return audit.requestVariables[typed.Name] == genericContainerRequest
	default:
		return false
	}
}

func (audit *goContainerAudit) requestKind(expression ast.Expr) containerRequestKind {
	switch typed := expression.(type) {
	case *ast.SelectorExpr:
		if isPackageSelector(typed, audit.testcontainers, "ContainerRequest") {
			return containerRequest
		}
		if isPackageSelector(typed, audit.testcontainers, "GenericContainerRequest") {
			return genericContainerRequest
		}
	case *ast.Ident:
		return audit.requestAliases[typed.Name]
	case *ast.StarExpr:
		return audit.requestKind(typed.X)
	case *ast.ParenExpr:
		return audit.requestKind(typed.X)
	}
	return containerRequestUnknown
}

func (audit *goContainerAudit) requestExpressionKind(expression ast.Expr) containerRequestKind {
	switch typed := expression.(type) {
	case *ast.CompositeLit:
		return audit.requestKind(typed.Type)
	case *ast.Ident:
		return audit.requestVariables[typed.Name]
	case *ast.SelectorExpr:
		if typed.Sel.Name == "ContainerRequest" && audit.requestExpressionKind(typed.X) == genericContainerRequest {
			return containerRequest
		}
	case *ast.UnaryExpr:
		if typed.Op == token.AND {
			return audit.requestExpressionKind(typed.X)
		}
	case *ast.StarExpr:
		return audit.requestExpressionKind(typed.X)
	case *ast.ParenExpr:
		return audit.requestExpressionKind(typed.X)
	}
	return containerRequestUnknown
}

func (audit *goContainerAudit) authorityCall(expression ast.Expr) bool {
	return isZeroArgPackageCall(expression, audit.authorityAlias, "Name")
}

func (audit *goContainerAudit) fail(message string) {
	if audit.contractErr == nil {
		audit.contractErr = fmt.Errorf("%s %s", audit.relative, message)
	}
}

func fieldName(expression ast.Expr) string {
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

// These are exact, count-bounded literals used by verifier fixtures or by the
// unrelated Authentik Docker fixture. No file receives a blanket exemption.
var (
	postgresTagPrefix    = strings.Join([]string{"post", "gres", ":"}, "")
	postgresDigestPrefix = strings.Join([]string{"post", "gres", "@"}, "")
)

func allowedPostgresImageLiteralAuthorities() map[string]map[string]int {
	return map[string]map[string]int{
		"internal/auth/sso_authentik_test.go": {postgresTagPrefix + "16-alpine": 1},
		"internal/releaseverify/ci_container_downloads_test.go": {
			" " + postgresTagPrefix + "17-alpine":         1,
			"docker://" + postgresTagPrefix + "17-alpine": 1,
		},
		"internal/releaseverify/ci_container_go.go": {
			"library/" + postgresTagPrefix + "16-alpine": 1,
		},
		"internal/releaseverify/ci_container_bootstrap_test.go": {
			"library/" + postgresTagPrefix + "16-alpine\n": 1,
		},
		"internal/releaseverify/ci_container_runtime_authority_test.go": {
			"library/" + postgresTagPrefix + "16-alpine": 1,
		},
	}
}

func rejectPostgresImageLiterals(file *ast.File, relative string) error {
	remaining := make(map[string]int)
	for literal, count := range allowedPostgresImageLiteralAuthorities()[relative] {
		remaining[literal] = count
	}
	var contractErr error
	ast.Inspect(file, func(node ast.Node) bool {
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		value, ok := goStringConstant(expression)
		if !ok || !isPostgresImageReference(value) {
			return true
		}
		if remaining[value] > 0 {
			remaining[value]--
			return true
		}
		contractErr = fmt.Errorf("%s duplicates the Postgres test image authority with literal %q", relative, value)
		return false
	})
	return contractErr
}

func goStringConstant(expression ast.Expr) (string, bool) {
	switch typed := expression.(type) {
	case *ast.BasicLit:
		if typed.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(typed.Value)
		return value, err == nil
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return "", false
		}
		left, leftOK := goStringConstant(typed.X)
		right, rightOK := goStringConstant(typed.Y)
		return left + right, leftOK && rightOK
	default:
		return "", false
	}
}

func isPostgresImageReference(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	if index := strings.LastIndex(value, "/"); index >= 0 {
		value = value[index+1:]
	}
	return strings.HasPrefix(value, postgresTagPrefix) || strings.HasPrefix(value, postgresDigestPrefix)
}

func goImportAlias(file *ast.File, importPath string) string {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != importPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		if importPath == testcontainersPackagePath {
			return "testcontainers"
		}
		return filepath.Base(path)
	}
	return ""
}

func isPackageSelector(expression ast.Expr, packageName, method string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && packageName != "" && identifier.Name == packageName && selector.Sel.Name == method
}

func isZeroArgPackageCall(expression ast.Expr, packageName, function string) bool {
	call, ok := expression.(*ast.CallExpr)
	return ok && len(call.Args) == 0 && isPackageSelector(call.Fun, packageName, function)
}
