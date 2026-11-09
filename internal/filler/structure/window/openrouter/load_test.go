package openrouter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDeploymentRequiresExactAuthorityAndRegularFile(t *testing.T) {
	authority := deploymentAuthorityFixture()
	deployment := deploymentFixture(authority)
	raw, err := json.Marshal(deployment)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "deployment.json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDeployment(path, &authority)
	if err != nil || loaded == nil || loaded.SHA256 != deployment.SHA256 {
		t.Fatalf("loaded=%+v error=%v", loaded, err)
	}
	if _, err := LoadDeployment(path, nil); err == nil || !strings.Contains(err.Error(), "authority") {
		t.Fatalf("missing authority error=%v", err)
	}
	other := authority
	other.SHA256 = strings.Repeat("f", 64)
	if _, err := LoadDeployment(path, &other); err == nil {
		t.Fatal("deployment accepted another authority")
	}
	link := filepath.Join(t.TempDir(), "deployment-link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDeployment(link, &authority); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink error=%v", err)
	}
}
