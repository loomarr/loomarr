package releaseverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyNoticesFailsClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte(strings.Join([]string{
		"FROM scratch",
		"COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/loomarr/",
		`LABEL org.opencontainers.image.documentation="https://github.com/mantonx/loomarr/blob/main/THIRD_PARTY_NOTICES.md" org.opencontainers.image.licenses="MIT AND GPL-3.0-or-later"`,
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, requirements := range noticeTextRequirements {
		if err := os.WriteFile(filepath.Join(root, name), []byte(strings.Join(requirements, "\n")), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := VerifyNotices(root); err != nil {
		t.Fatalf("complete notice fixture: %v", err)
	}

	dockerfilePath := filepath.Join(root, "Dockerfile")
	dockerfile, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("commented runtime copy is not evidence", func(t *testing.T) {
		commented := strings.Replace(string(dockerfile), "COPY LICENSE", "# COPY LICENSE", 1)
		if err := os.WriteFile(dockerfilePath, []byte(commented), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := VerifyNotices(root); err == nil {
			t.Fatal("VerifyNotices accepted a commented-out runtime COPY")
		}
		if err := os.WriteFile(dockerfilePath, dockerfile, 0o600); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("echoed runtime copy is not evidence", func(t *testing.T) {
		echoed := strings.Replace(string(dockerfile), "COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/loomarr/", "RUN echo 'COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/loomarr/'", 1)
		if err := os.WriteFile(dockerfilePath, []byte(echoed), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := VerifyNotices(root); err == nil {
			t.Fatal("VerifyNotices accepted an echoed runtime COPY")
		}
		if err := os.WriteFile(dockerfilePath, dockerfile, 0o600); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("commented OCI label is not evidence", func(t *testing.T) {
		commented := strings.Replace(string(dockerfile), "LABEL org.opencontainers", "# LABEL org.opencontainers", 1)
		if err := os.WriteFile(dockerfilePath, []byte(commented), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := VerifyNotices(root); err == nil {
			t.Fatal("VerifyNotices accepted commented-out OCI labels")
		}
		if err := os.WriteFile(dockerfilePath, dockerfile, 0o600); err != nil {
			t.Fatal(err)
		}
	})

	noticePath := filepath.Join(root, "THIRD_PARTY_NOTICES.md")
	complete, err := os.ReadFile(noticePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("missing open review", func(t *testing.T) {
		incomplete := strings.Replace(string(complete), "final qualified legal/NOTICE review", "review later", 1)
		if err := os.WriteFile(noticePath, []byte(incomplete), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := VerifyNotices(root); err == nil {
			t.Fatal("VerifyNotices accepted a removed legal-review blocker")
		}
	})
	t.Run("unsupported legal conclusion", func(t *testing.T) {
		unsupported := string(complete) + "\n" + forbiddenNoticeClaims[0]
		if err := os.WriteFile(noticePath, []byte(unsupported), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := VerifyNotices(root); err == nil {
			t.Fatal("VerifyNotices accepted the removed mere-aggregation conclusion")
		}
	})
}
