package releaseverify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyCIContainerDownloadsRequiresExactPostgresImageOwner(t *testing.T) {
	implementations := map[string]string{
		"environment return": `package postgresimage

import "os"

func Name() string { return os.Getenv("POSTGRES_TEST_IMAGE") }
`,
		"transformed embedded return": `package postgresimage

import (
	_ "embed"
	"strings"
)

//go:embed image.txt
var image string

func Name() string {
	return strings.ReplaceAll(strings.TrimSpace(image), "postgres", "attacker")
}
`,
		"alternate literal return": `package postgresimage

func Name() string { return "attacker.invalid/postgres:pinned" }
`,
	}

	t.Run("missing owner", func(t *testing.T) {
		root := writeCIContainerDownloadsFixture(t)
		path := filepath.Join(root, "internal", "testkit", "postgresimage", "image.go")
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := VerifyCIContainerDownloads(root); err == nil {
			t.Fatal("VerifyCIContainerDownloads accepted a missing Postgres image owner")
		}
	})

	for name, implementation := range implementations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, "internal", "testkit", "postgresimage", "image.go"), implementation)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a mutable Postgres image owner")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRequiresUniqueUntaggedPostgresImageOwner(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"build-constrained canonical owner": func(t *testing.T, root string) {
			path := filepath.Join(root, "internal", "testkit", "postgresimage", "image.go")
			writeFixtureFile(t, path, "//go:build !linux\n\n"+readFixtureFile(t, path))
		},
		"alternate production file": func(t *testing.T, root string) {
			writeFixtureFile(t, filepath.Join(root, "internal", "testkit", "postgresimage", "alternate.go"), "package postgresimage\n\nvar alternate = true\n")
		},
		"additional embed owner": func(t *testing.T, root string) {
			writeFixtureFile(t, filepath.Join(root, "internal", "testkit", "postgresimage", "alternate.go"), `package postgresimage

import _ "embed"

//go:embed image.txt
var alternateImage string
`)
		},
		"alternate Name owner": func(t *testing.T, root string) {
			writeFixtureFile(t, filepath.Join(root, "internal", "testkit", "postgresimage", "image_linux.go"), `//go:build linux

package postgresimage

import "os"

func Name() string { return os.Getenv("POSTGRES_TEST_IMAGE") }
`)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			mutate(t, root)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted multiple or build-dependent Postgres image owners")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsIgnoresTestOnlyPostgresImageFilesForOwnership(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, "internal", "testkit", "postgresimage", "image_test.go"), `package postgresimage

import "testing"

func TestNameIsPresent(t *testing.T) { _ = Name() }
`)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("test-only Postgres image file changed production ownership: %v", err)
	}
}

func TestVerifyCIContainerDownloadsRejectsPostgresAuthorityBypasses(t *testing.T) {
	tests := map[string]string{
		"postgres Run function value": `package store

import "github.com/testcontainers/testcontainers-go/modules/postgres"

var uncheckedPostgresRun = postgres.Run
`,
		"deprecated postgres RunContainer": `package store

import "github.com/testcontainers/testcontainers-go/modules/postgres"

func unchecked() { _, _ = postgres.RunContainer(nil) }
`,
		"deprecated postgres RunContainer function value": `package store

import "github.com/testcontainers/testcontainers-go/modules/postgres"

var unchecked = postgres.RunContainer
`,
		"aliased deprecated postgres RunContainer": `package store

import pg "github.com/testcontainers/testcontainers-go/modules/postgres"

func unchecked() { _, _ = pg.RunContainer(nil) }
`,
		"wrapper-owned postgres literal": `package store

func uncheckedPostgresImage() string { return "postgres:17-alpine" }
`,
		"generic testcontainers request": `package store

import "github.com/testcontainers/testcontainers-go"

var uncheckedPostgresRequest = testcontainers.ContainerRequest{Image: "postgres:17-alpine"}
`,
		"generic testcontainers Run": `package store

import "github.com/testcontainers/testcontainers-go"

func uncheckedPostgresContainer() { _, _ = testcontainers.Run(nil, "postgres:17-alpine") }
`,
		"generic testcontainers opaque Image": `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

var uncheckedPostgresRequest = testcontainers.ContainerRequest{Image: os.Getenv("POSTGRES_TEST_IMAGE")}
`,
		"generic testcontainers Run function value": `package store

import "github.com/testcontainers/testcontainers-go"

var uncheckedGenericRun = testcontainers.Run
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, "internal", "store", "unchecked_postgres_test.go"), source)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a Postgres testcontainers authority bypass")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsImageAndPullCustomizers(t *testing.T) {
	tests := map[string]string{
		"postgres WithImage": `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func unchecked() { _, _ = postgres.Run(nil, postgresimage.Name(), testcontainers.WithImage("other.invalid/image:pinned")) }
`,
		"postgres WithAlwaysPull": `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func unchecked() { _, _ = postgres.Run(nil, postgresimage.Name(), testcontainers.WithAlwaysPull()) }
`,
		"aliased image customizer": `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	tc "github.com/testcontainers/testcontainers-go"
	pg "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func unchecked() { _, _ = pg.Run(nil, postgresimage.Name(), tc.WithImage("other.invalid/image:pinned")) }
`,
		"nested image customizer": `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func wrap(value testcontainers.ContainerCustomizer) testcontainers.ContainerCustomizer { return value }
func unchecked() { _, _ = postgres.Run(nil, postgresimage.Name(), wrap(testcontainers.WithImage("other.invalid/image:pinned"))) }
`,
		"customizer variable": `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func unchecked() {
	customizer := testcontainers.WithAlwaysPull()
	_, _ = postgres.Run(nil, postgresimage.Name(), customizer)
}
`,
		"variadic customizers": `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func unchecked() {
	customizers := []testcontainers.ContainerCustomizer{testcontainers.WithImage("other.invalid/image:pinned")}
	_, _ = postgres.Run(nil, postgresimage.Name(), customizers...)
}
`,
		"opaque postgres customizer": `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func opaque() testcontainers.ContainerCustomizer { return testcontainers.WithAlwaysPull() }
func unchecked() { _, _ = postgres.Run(nil, postgresimage.Name(), opaque()) }
`,
		"shadowed allowed customizer selector": `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

type malicious struct{}
func (malicious) WithWaitStrategy(any) tc.ContainerCustomizer { return tc.WithAlwaysPull() }
func unchecked() {
	tc := malicious{}
	_, _ = postgres.Run(nil, postgresimage.Name(), tc.WithWaitStrategy(nil))
}
`,
		"generic Run image customizer": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() { _, _ = testcontainers.Run(nil, "redis:7", testcontainers.WithImage("other.invalid/image:pinned")) }
`,
		"generic Run pull customizer": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() { _, _ = testcontainers.Run(nil, "redis:7", testcontainers.WithAlwaysPull()) }
`,
		"generic Run unknown customizer": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() { _, _ = testcontainers.Run(nil, "redis:7", testcontainers.WithEnv(map[string]string{"SAFE": "value"})) }
`,
		"GenericContainer always-pull request": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7", AlwaysPullImage: true}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"GenericContainer image substitutors": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7", ImageSubstitutors: opaqueSubstitutors()}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		"post-construction pull policy": `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	request.AlwaysPullImage = true
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, "internal", "store", "unchecked_customizer_test.go"), source)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an image-authority constructor customizer outside the closed policy")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsAllowsExactPostgresCustomizers(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, "internal", "store", "allowed_customizer_test.go"), `package store

import (
	"github.com/loomarr/loomarr/internal/testkit/postgresimage"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func allowed() {
	_, _ = postgres.Run(nil, postgresimage.Name(),
		postgres.WithDatabase("loomarr"),
		postgres.WithUsername("loomarr"),
		postgres.WithPassword("loomarr"),
		testcontainers.WithWaitStrategy(nil),
	)
}
`)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("exact Postgres customizers: %v", err)
	}
}
