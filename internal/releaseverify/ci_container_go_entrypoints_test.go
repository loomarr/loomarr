package releaseverify

import (
	"path/filepath"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsAlternateTestcontainersAcquisitionEntrypoints(t *testing.T) {
	tests := map[string]string{
		"ParallelContainers": `package tooling

import "github.com/testcontainers/testcontainers-go"

func unchecked(requests testcontainers.ParallelContainerRequest) {
	_, _ = testcontainers.ParallelContainers(nil, requests, testcontainers.ParallelContainersOptions{})
}
`,
		"DockerProvider CreateContainer": `package tooling

import "github.com/testcontainers/testcontainers-go"

func unchecked(provider *testcontainers.DockerProvider, request testcontainers.ContainerRequest) {
	_, _ = provider.CreateContainer(nil, request)
}
`,
		"DockerProvider RunContainer": `package tooling

import "github.com/testcontainers/testcontainers-go"

func unchecked(provider *testcontainers.DockerProvider, request testcontainers.ContainerRequest) {
	_, _ = provider.RunContainer(nil, request)
}
`,
		"DockerProvider ReuseOrCreateContainer": `package tooling

import "github.com/testcontainers/testcontainers-go"

func unchecked(provider *testcontainers.DockerProvider, request testcontainers.ContainerRequest) {
	_, _ = provider.ReuseOrCreateContainer(nil, request)
}
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, "tooling", "unchecked_container.go"), source)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted an unaudited Testcontainers acquisition entrypoint")
			}
		})
	}
}
