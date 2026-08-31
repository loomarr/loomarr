package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsRequestPointerAggregateEscapes(t *testing.T) {
	tests := map[string]string{
		"slice aggregate": `package store

import "github.com/testcontainers/testcontainers-go"

func opaque() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	refs := []*testcontainers.ContainerRequest{&request}
	refs[0].Image = opaque()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"array aggregate compound mutation": `package store

import "github.com/testcontainers/testcontainers-go"

func opaque() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	refs := [1]*testcontainers.ContainerRequest{&request}
	refs[0].Image += opaque()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"map aggregate": `package store

import "github.com/testcontainers/testcontainers-go"

func opaque() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	refs := map[string]*testcontainers.ContainerRequest{"main": &request}
	refs["main"].Image = opaque()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"struct aggregate selection": `package store

import "github.com/testcontainers/testcontainers-go"

func opaque() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	holder := struct{ Request *testcontainers.ContainerRequest }{Request: &request}
	holder.Request.Image = opaque()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"interface aggregate": `package store

import "github.com/testcontainers/testcontainers-go"

func opaque() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	var escaped any = &request
	ref := escaped.(*testcontainers.ContainerRequest)
	ref.Image = opaque()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"nested scope aggregate": `package store

import "github.com/testcontainers/testcontainers-go"

func opaque() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	func() {
		refs := []*testcontainers.ContainerRequest{&request}
		refs[0].Image = opaque()
	}()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"slice index assignment": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	refs := make([]*testcontainers.ContainerRequest, 1)
	refs[0] = &request
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"map index assignment through alias": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	alias := &request
	refs := map[string]*testcontainers.ContainerRequest{}
	refs["main"] = alias
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"struct field assignment": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	holder := struct{ Request *testcontainers.ContainerRequest }{}
	holder.Request = &request
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"interface assignment": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	var escaped any
	escaped = &request
	_, _ = escaped, request
}
`,
		"aggregate passed to opaque call": `package store

import "github.com/testcontainers/testcontainers-go"

func opaque([]*testcontainers.ContainerRequest) {}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	opaque([]*testcontainers.ContainerRequest{&request})
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"append escape": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	var refs []*testcontainers.ContainerRequest
	refs = append(refs, &request)
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"return escape": `package store

import "github.com/testcontainers/testcontainers-go"

func escaped() *testcontainers.ContainerRequest {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	return &request
}
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, "internal", "store", "unchecked_aggregate_test.go"), source)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a request pointer aggregate escape")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsUnrelatedLiteralInAllowedFile(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, "internal", "auth", "sso_authentik_test.go"), `package auth

const documentedAuthentikFixture = "postgres:16-alpine"
const unrelatedDuplicate = "postgres:17-alpine"
`)
	if err := VerifyCIContainerDownloads(root); err == nil {
		t.Fatal("VerifyCIContainerDownloads exempted an unrelated Postgres literal in an allowed fixture file")
	}
}

func TestVerifyCIContainerDownloadsAllowsExplicitUnrelatedGenericImage(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, "internal", "cache", "container_test.go"), `package cache

import "github.com/testcontainers/testcontainers-go"

var redisRequest = testcontainers.ContainerRequest{Image: "redis:7-alpine"}

func startRedis() {
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7-alpine"},
	})
}
`)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("explicit unrelated generic image: %v", err)
	}
}

func TestVerifyCIContainerDownloadsAllowsMatchedTrackedRequestAssignment(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, "internal", "cache", "container_assignment_test.go"), `package cache

import "github.com/testcontainers/testcontainers-go"

func startRedis() {
	request := testcontainers.ContainerRequest{Image: "redis:7-alpine"}
	replacement := testcontainers.ContainerRequest{Image: "redis:7-alpine"}
	_, request = 0, replacement
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
	})
}
`)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("matched tracked request assignment: %v", err)
	}
}

func TestVerifyCIContainerDownloadsAllowsExactPostgresImageAuthority(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, "internal", "store", "generic_postgres_test.go"), `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
)

func startPostgresGeneric() {
	request := testcontainers.ContainerRequest{Image: postgresimage.Name()}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
	})
}
`)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("exact Postgres image authority: %v", err)
	}
}

func TestVerifyCIContainerDownloadsAllowsPointerMutationAuthorities(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, "internal", "store", "pointer_authority_test.go"), `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
)

func startExplicitUnrelated() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	image := &request.Image
	*image = "redis:8"
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}

func startExactAuthority() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	image := &request.Image
	(*image) = postgresimage.Name()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}

func startTrackedWholeRequest() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	pointer := &request
	*pointer = testcontainers.ContainerRequest{Image: "redis:8"}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("pointer mutation authorities: %v", err)
	}
}

func TestVerifyCIContainerDownloadsSkipsOnlyRepositoryCachesAndGeneratedGo(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	badSource := `package ignored

import "github.com/testcontainers/testcontainers-go/modules/postgres"

var unchecked = postgres.Run
`
	for _, path := range []string{
		"vendor/example/container.go",
		"generated/example/container.go",
		".artifacts/cache/container.go",
		"futurepkg/container.gen.go",
	} {
		writeFixtureFile(t, filepath.Join(root, filepath.FromSlash(path)), badSource)
	}
	writeFixtureFile(t, filepath.Join(root, "futurepkg", "generated_header.go"), "// Code generated by fixture. DO NOT EDIT.\n"+badSource)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("safe generated/cache exclusions: %v", err)
	}
}

func TestVerifyCIContainerDownloadsIgnoresCommentedWorkflowAcquisition(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "ci-postgres.yml")
	workflow := readFixtureFile(t, workflowPath)
	writeFixtureFile(t, workflowPath, strings.Replace(
		workflow,
		"    steps:\n",
		"    steps:\n      # docker pull busybox:stable is policy prose, not a step\n",
		1,
	))
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("comment-only workflow acquisition evidence: %v", err)
	}
}
