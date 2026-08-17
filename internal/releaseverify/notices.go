package releaseverify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var dockerfileNoticeRequirements = []struct {
	instruction string
	fragment    string
}{
	{instruction: "COPY", fragment: "COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/loomarr/"},
	{instruction: "LABEL", fragment: `org.opencontainers.image.documentation="https://github.com/mantonx/loomarr/blob/main/THIRD_PARTY_NOTICES.md"`},
	{instruction: "LABEL", fragment: `org.opencontainers.image.licenses="MIT AND GPL-3.0-or-later"`},
}

var noticeTextRequirements = map[string][]string{
	"THIRD_PARTY_NOTICES.md": {
		"**GPL-3.0-or-later** for the combined executable",
		"https://github.com/yt-dlp/yt-dlp/blob/master/README.md#license",
		"https://github.com/yt-dlp/yt-dlp/blob/master/THIRD_PARTY_LICENSES.txt",
		"## Open redistribution review — beta blockers",
		"exact corresponding source",
		"pin the runtime and build base images by digest",
		"DejaVu font license",
		"Prometheus `NOTICE`",
		"final qualified legal/NOTICE review",
		"inventory, not release clearance",
	},
	"README.md": {
		"Loomarr source is [MIT](LICENSE)",
		"`/usr/share/doc/loomarr/`",
	},
}

var forbiddenNoticeClaims = []string{
	"Under the GPL this is\n*mere aggregation*",
	"does **not** make Loomarr a derivative work",
	"| `yt-dlp` | https://github.com/yt-dlp/yt-dlp | The Unlicense (public domain) |",
}

// VerifyNotices ensures the release image packages its license inventory and
// cannot silently turn unresolved redistribution work into a claim of closure.
func VerifyNotices(root string) error {
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		return fmt.Errorf("read Dockerfile: %w", err)
	}
	instructions := dockerfileInstructions(string(dockerfile))
	for _, requirement := range dockerfileNoticeRequirements {
		found := false
		for _, instruction := range instructions {
			if strings.HasPrefix(instruction, requirement.instruction+" ") && strings.Contains(instruction, requirement.fragment) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("dockerfile has no active %s instruction containing release-notice evidence %q", requirement.instruction, requirement.fragment)
		}
	}

	contents := make(map[string]string, len(noticeTextRequirements))
	for name, required := range noticeTextRequirements {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		contents[name] = string(data)
		for _, fragment := range required {
			if !strings.Contains(contents[name], fragment) {
				return fmt.Errorf("%s is missing required release-notice evidence %q", name, fragment)
			}
		}
	}
	for _, claim := range forbiddenNoticeClaims {
		if strings.Contains(contents["THIRD_PARTY_NOTICES.md"], claim) {
			return fmt.Errorf("THIRD_PARTY_NOTICES.md reintroduced unsupported claim %q", claim)
		}
	}
	return nil
}

func dockerfileInstructions(contents string) []string {
	var instructions []string
	var current string
	for _, line := range strings.Split(contents, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		continued := strings.HasSuffix(trimmed, "\\")
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "\\"))
		if current == "" {
			current = trimmed
		} else {
			current += " " + trimmed
		}
		if !continued {
			instructions = append(instructions, strings.Join(strings.Fields(current), " "))
			current = ""
		}
	}
	if current != "" {
		instructions = append(instructions, strings.Join(strings.Fields(current), " "))
	}
	return instructions
}
