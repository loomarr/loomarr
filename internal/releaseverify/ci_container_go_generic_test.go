package releaseverify

import (
	"path/filepath"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsGenericContainerEvasions(t *testing.T) {
	tests := map[string]struct {
		path   string
		source string
	}{
		"GenericContainer request": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() { _, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{
	ContainerRequest: testcontainers.ContainerRequest{Image: os.Getenv("POSTGRES_IMAGE")},
}) }
`,
		},
		"ContainerRequest type alias": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

type request = testcontainers.ContainerRequest
var unchecked = request{Image: os.Getenv("POSTGRES_IMAGE")}
`,
		},
		"forward ContainerRequest alias chain": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

type outer = request
type request = testcontainers.ContainerRequest
var unchecked = outer{Image: os.Getenv("POSTGRES_IMAGE")}
`,
		},
		"post-construction Image assignment": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	var request testcontainers.ContainerRequest
	request.Image = os.Getenv("POSTGRES_IMAGE")
}
`,
		},
		"pointer alias Image assignment": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	var request testcontainers.ContainerRequest
	alias := &request
	alias.Image = os.Getenv("POSTGRES_IMAGE")
}
`,
		},
		"identifier alias Image assignment": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	var request testcontainers.ContainerRequest
	alias := request
	alias.Image = os.Getenv("POSTGRES_IMAGE")
}
`,
		},
		"typed pointer alias Image assignment": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	var request testcontainers.ContainerRequest
	var alias *testcontainers.ContainerRequest = &request
alias.Image = os.Getenv("POSTGRES_IMAGE")
}
`,
		},
		"dereferenced pointer alias Image assignment": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	var request testcontainers.ContainerRequest
	alias := &request
	copy := *alias
	copy.Image = os.Getenv("POSTGRES_IMAGE")
}
`,
		},
		"parenthesized pointer Image assignment": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	var request testcontainers.ContainerRequest
	alias := &request
	(*alias).Image = os.Getenv("POSTGRES_IMAGE")
}
`,
		},
		"whole request replaced through pointer alias": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueBuilder() testcontainers.ContainerRequest { return testcontainers.ContainerRequest{} }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7-alpine"}
	alias := &request
	*alias = opaqueBuilder()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"whole request replaced through parenthesized pointer": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueBuilder() testcontainers.ContainerRequest { return testcontainers.ContainerRequest{} }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7-alpine"}
	alias := &request
	(*alias) = opaqueBuilder()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"whole request replaced through dereferenced alias chain": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueBuilder() testcontainers.ContainerRequest { return testcontainers.ContainerRequest{} }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7-alpine"}
	alias := &request
	copy := alias
	*copy = opaqueBuilder()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"whole request replaced by multi-result assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueBuilder() (error, testcontainers.ContainerRequest) {
	return nil, testcontainers.ContainerRequest{}
}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7-alpine"}
	_, request = opaqueBuilder()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"whole request pointer replaced by multi-result assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueBuilder() (error, testcontainers.ContainerRequest) {
	return nil, testcontainers.ContainerRequest{}
}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7-alpine"}
	alias := &request
	_, *alias = opaqueBuilder()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"nested request selector replaced by multi-result assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueBuilder() (error, testcontainers.ContainerRequest) {
	return nil, testcontainers.ContainerRequest{}
}
func unchecked() {
	generic := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7-alpine"},
	}
	_, generic.ContainerRequest = opaqueBuilder()
	_, _ = testcontainers.GenericContainer(nil, generic)
}
`,
		},
		"request Image replaced by range assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImages() []string { return nil }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	for _, request.Image = range opaqueImages() {
	}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"request Image replaced by range key assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImages() map[string]struct{} { return nil }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	for request.Image, _ = range opaqueImages() {
	}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"nested dereferenced parenthesized Image replaced by range assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImages() []string { return nil }
func unchecked() {
	generic := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7"},
	}
	alias := &generic.ContainerRequest
	for _, ((*alias).Image) = range opaqueImages() {
	}
	_, _ = testcontainers.GenericContainer(nil, generic)
}
`,
		},
		"request Image constructed by compound assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	request.Image = "post"
	request.Image += "gres:17-alpine"
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"whole request escapes to opaque function": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueMutate(*testcontainers.ContainerRequest) {}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	opaqueMutate(&request)
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"request Image escapes to opaque function": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueMutate(*string) {}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	opaqueMutate(&request.Image)
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"whole request pointer alias escapes to opaque function": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueMutate(*testcontainers.ContainerRequest) {}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	alias := &request
	opaqueMutate(alias)
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"request Image pointer alias escapes to opaque function": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueMutate(*string) {}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	alias := &request.Image
	opaqueMutate((alias))
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"request Image pointer receives opaque assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImage() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	image := &request.Image
	*image = opaqueImage()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"parenthesized request Image pointer receives opaque assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImage() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	image := &request.Image
	(*image) = opaqueImage()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"request Image pointer uses compound assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImage() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	image := &request.Image
	*image += opaqueImage()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"request Image pointer receives multi-result assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImages() (error, string) { return nil, "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	image := &request.Image
	_, *image = opaqueImages()
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"request Image pointer receives range value": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImages() []string { return nil }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	image := &request.Image
	for _, *image = range opaqueImages() {
	}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"nested request Image pointer receives range key": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueImages() map[string]struct{} { return nil }
func unchecked() {
	generic := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7"},
	}
	image := &generic.ContainerRequest.Image
	nested := image
	for (*nested), _ = range opaqueImages() {
	}
	_, _ = testcontainers.GenericContainer(nil, generic)
}
`,
		},
		"nested request pointer escapes to opaque function": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueMutate(*testcontainers.ContainerRequest) {}
func unchecked() {
	generic := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7"},
	}
	opaqueMutate(&(generic.ContainerRequest))
	_, _ = testcontainers.GenericContainer(nil, generic)
}
`,
		},
		"dereferenced parenthesized pointer escapes to opaque function": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueMutate(*testcontainers.ContainerRequest) {}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	alias := &request
	opaqueMutate(&((*alias)))
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"chained request pointer alias escapes to opaque function": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueMutate(*testcontainers.ContainerRequest) {}
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	first := &request
	second := first
	opaqueMutate(second)
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"reflection pointer mutation": {
			source: `package store

import (
	"reflect"
	"github.com/testcontainers/testcontainers-go"
)

func opaqueImage() string { return "" }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	reflect.ValueOf(&request).Elem().FieldByName("Image").SetString(opaqueImage())
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"unsafe pointer escape": {
			source: `package store

import (
	"unsafe"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	pointer := unsafe.Pointer(&request)
	_ = pointer
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"whole request replaced by range assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueRequests() []testcontainers.ContainerRequest { return nil }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	for _, request = range opaqueRequests() {
	}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"dereferenced request replaced by range assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueRequests() []testcontainers.ContainerRequest { return nil }
func unchecked() {
	request := testcontainers.ContainerRequest{Image: "redis:7"}
	alias := &request
	for _, (*alias) = range opaqueRequests() {
	}
	_, _ = testcontainers.GenericContainer(nil, testcontainers.GenericContainerRequest{ContainerRequest: request})
}
`,
		},
		"nested request selector replaced by range assignment": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func opaqueRequests() []testcontainers.ContainerRequest { return nil }
func unchecked() {
	generic := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7"},
	}
	for _, generic.ContainerRequest = range opaqueRequests() {
	}
	_, _ = testcontainers.GenericContainer(nil, generic)
}
`,
		},
		"nested request pointer alias Image assignment": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	generic := testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7-alpine"}}
	alias := &generic.ContainerRequest
	alias.Image = os.Getenv("IMAGE")
}
`,
		},
		"nested request dereferenced pointer Image assignment": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	generic := testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7-alpine"}}
	alias := &generic.ContainerRequest
	(*alias).Image = os.Getenv("IMAGE")
}
`,
		},
		"nested request parenthesized selector alias": {
			source: `package store

import (
	"os"
	"github.com/testcontainers/testcontainers-go"
)

func unchecked() {
	generic := testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{Image: "redis:7-alpine"}}
	alias := &(generic.ContainerRequest)
	alias.Image = os.Getenv("IMAGE")
}
`,
		},
		"opaque request builder": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

func request() testcontainers.GenericContainerRequest { return testcontainers.GenericContainerRequest{} }
func unchecked() { _, _ = testcontainers.GenericContainer(nil, request()) }
`,
		},
		"GenericContainer function value": {
			source: `package store

import "github.com/testcontainers/testcontainers-go"

var unchecked = testcontainers.GenericContainer
`,
		},
		"cmd package outside internal": {
			path: "cmd/unchecked/main_test.go",
			source: `package unchecked

import "github.com/testcontainers/testcontainers-go/modules/postgres"

func unchecked() { _, _ = postgres.Run(nil, "postgres:17-alpine") }
`,
		},
		"tooling package outside internal": {
			path: "tooling/unchecked/container_test.go",
			source: `package unchecked

import "github.com/testcontainers/testcontainers-go/modules/postgres"

var unchecked = postgres.Run
`,
		},
		"future top-level package": {
			path: "futurepkg/container_test.go",
			source: `package futurepkg

import "github.com/testcontainers/testcontainers-go"

func unchecked() { _, _ = testcontainers.Run(nil, "postgres:17-alpine") }
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := test.path
			if path == "" {
				path = "internal/store/unchecked_postgres_test.go"
			}
			writeFixtureFile(t, filepath.Join(root, filepath.FromSlash(path)), test.source)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an opaque container-acquisition seam")
			}
		})
	}
}
