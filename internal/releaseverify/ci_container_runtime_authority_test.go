package releaseverify

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/testcontainers/testcontainers-go"
)

func TestVerifyCIContainerDownloadsClosesTestcontainersRuntimeImageSubstitution(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"missing runtime Hub authority": func(t *testing.T, root string) {
			path := filepath.Join(root, "mk", "store.mk")
			source := readFixtureFile(t, path)
			const authority = "TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX=docker.io "
			if !strings.Contains(source, authority) {
				t.Fatal("repository fixture lacks the runtime Hub image authority")
			}
			writeFixtureFile(t, path, strings.Replace(source, authority, "", 1))
		},
		"attacker runtime Hub authority": func(t *testing.T, root string) {
			path := filepath.Join(root, "mk", "store.mk")
			source := readFixtureFile(t, path)
			const authority = "TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX=docker.io"
			if !strings.Contains(source, authority) {
				t.Fatal("repository fixture lacks the runtime Hub image authority")
			}
			writeFixtureFile(t, path, strings.Replace(source, authority, "TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX=attacker.invalid", 1))
		},
		"registry-qualified Postgres pin": func(t *testing.T, root string) {
			path := filepath.Join(root, filepath.FromSlash(postgresImageAuthorityPath))
			source := readFixtureFile(t, path)
			if strings.HasPrefix(strings.TrimSpace(source), "docker.io/") {
				t.Fatal("repository fixture lacks an implicit Docker Hub Postgres pin")
			}
			writeFixtureFile(t, path, "docker.io/"+source)
		},
		"unqualified Ryuk pin": func(t *testing.T, root string) {
			path := filepath.Join(root, filepath.FromSlash(ryukImageAuthorityPath))
			source := readFixtureFile(t, path)
			if !strings.HasPrefix(strings.TrimSpace(source), "docker.io/") {
				t.Fatal("repository fixture lacks a registry-qualified Ryuk pin")
			}
			writeFixtureFile(t, path, strings.TrimPrefix(source, "docker.io/"))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			mutate(t, root)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a runtime-transformable Testcontainers image authority")
			}
		})
	}
}

func TestTestcontainersHubPrefixRequiresUnqualifiedPostgresAuthority(t *testing.T) {
	const want = "library/postgres:16-alpine"

	_, source, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(source), "..", "..")
	storeMakefile := readFixtureFile(t, filepath.Join(root, "mk", "store.mk"))
	if !strings.Contains(storeMakefile, "TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX=docker.io") {
		t.Fatal("test-pg must set the exact Docker Hub runtime authority")
	}
	postgresImage := strings.TrimSpace(readFixtureFile(t, filepath.Join(root, filepath.FromSlash(postgresImageAuthorityPath))))
	if postgresImage != want {
		t.Fatalf("testcontainers-go v0.44 prepends the Docker Hub prefix to its implicit form: Postgres authority = %q, want %q", postgresImage, want)
	}
}

func TestTestcontainersRuntimeImageAuthorityOverridesAttackerUserProperties(t *testing.T) {
	const helper = "LOOMARR_TESTCONTAINERS_CONFIG_HELPER"
	if os.Getenv(helper) == "1" {
		config := testcontainers.ReadConfig()
		if config.Config.HubImageNamePrefix != "docker.io" {
			t.Fatalf("effective Hub image prefix = %q, want docker.io", config.Config.HubImageNamePrefix)
		}
		if config.Config.RyukDisabled {
			t.Fatal("attacker user properties disabled Ryuk despite the exact runtime environment")
		}
		substituted, err := testcontainers.NewCustomHubSubstitutor("attacker.invalid").Substitute(postgresTestImage)
		if err != nil {
			t.Fatal(err)
		}
		if substituted != postgresTestImage {
			t.Fatalf("effective Postgres image = %q, want %q", substituted, postgresTestImage)
		}
		//nolint:staticcheck // The deprecated symbol is the only public accessor for the pinned dependency's Ryuk default.
		if "docker.io/"+testcontainers.ReaperDefaultImage != testcontainersRyukImage {
			t.Fatalf("effective Ryuk image does not match %q", testcontainersRyukImage)
		}
		return
	}

	home := t.TempDir()
	properties := "hub.image.name.prefix=attacker.invalid/mirror\nryuk.disabled=true\n"
	if err := os.WriteFile(filepath.Join(home, ".testcontainers.properties"), []byte(properties), 0o600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := testkit.ProbeWithFakeDocker(ctx, executable, []string{"-test.run=^TestTestcontainersRuntimeImageAuthorityOverridesAttackerUserProperties$"}, testkit.FakeDockerConfig{
		Environment: []string{
			helper + "=1",
			"HOME=" + home,
			"TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX=docker.io",
			"TESTCONTAINERS_RYUK_DISABLED=false",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Err != nil {
		t.Fatalf("testcontainers runtime configuration probe: %v", result.Err)
	}
}
