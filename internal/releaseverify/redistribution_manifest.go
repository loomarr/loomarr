package releaseverify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const redistributionManifestPath = "docs/engineering/evidence/redistribution-manifest-v1.json"

type redistributionManifest struct {
	SchemaVersion int                      `json:"schema_version"`
	Purpose       string                   `json:"purpose"`
	Artifacts     []redistributionArtifact `json:"artifacts"`
}
type redistributionArtifact struct {
	Name     string                 `json:"name"`
	Assets   []redistributionAsset  `json:"assets"`
	Upstream redistributionUpstream `json:"upstream"`
}
type redistributionAsset struct {
	Architecture string `json:"architecture"`
	Asset        string `json:"asset"`
	AssetURL     string `json:"asset_url"`
	SHA256       string `json:"sha256"`
}
type redistributionUpstream struct {
	ReleaseURL         string   `json:"release_url"`
	ReleaseRev         string   `json:"release_revision"`
	ReleaseCommit      string   `json:"release_commit"`
	SourceURL          string   `json:"source_url"`
	SourceRev          string   `json:"source_revision"`
	BuildURL           string   `json:"build_url"`
	BuildRev           string   `json:"build_revision"`
	BuildID            string   `json:"build_id"`
	RetentionClass     string   `json:"retention_class,omitempty"`
	RetentionPolicyURL string   `json:"retention_policy_url,omitempty"`
	RetentionPolicyRev string   `json:"retention_policy_revision,omitempty"`
	LicenseURLs        []string `json:"license_urls"`
}

var redistributionDockerfileArgs = regexp.MustCompile(`(?m)^ARG ([A-Z0-9_]+)=(\S+)$`)
var immutableGitRevision = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// VerifyRedistributionManifest binds release evidence to active Dockerfile inputs without downloads.
func VerifyRedistributionManifest(root string) error {
	data, err := os.ReadFile(filepath.Join(root, redistributionManifestPath))
	if err != nil {
		return fmt.Errorf("read redistribution manifest: %w", err)
	}
	var manifest redistributionManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("parse redistribution manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("redistribution manifest must contain exactly one JSON document")
	}
	if manifest.SchemaVersion != 1 || !strings.Contains(manifest.Purpose, "traceability evidence only") || !strings.Contains(manifest.Purpose, "legal clearance") {
		return errors.New("redistribution manifest must remain traceability-only and disclaim legal clearance")
	}
	if len(manifest.Artifacts) != 2 {
		return errors.New("redistribution manifest must contain exactly two artifacts")
	}
	artifacts := make(map[string]redistributionArtifact, 2)
	for _, artifact := range manifest.Artifacts {
		if artifact.Name != "ffmpeg" && artifact.Name != "yt-dlp" || artifacts[artifact.Name].Name != "" {
			return fmt.Errorf("invalid or duplicate artifact %q", artifact.Name)
		}
		if err := verifyRedistributionArtifact(artifact); err != nil {
			return fmt.Errorf("artifact %s: %w", artifact.Name, err)
		}
		artifacts[artifact.Name] = artifact
	}
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		return fmt.Errorf("read Dockerfile: %w", err)
	}
	args := make(map[string]string)
	for _, match := range redistributionDockerfileArgs.FindAllStringSubmatch(string(dockerfile), -1) {
		args[match[1]] = match[2]
	}
	ffmpeg, ytdlp := artifacts["ffmpeg"], artifacts["yt-dlp"]
	if args["FFMPEG_RELEASE"] != ffmpeg.Upstream.ReleaseRev || args["FFMPEG_BUILD_ID"] != ffmpeg.Upstream.BuildID || args["FFMPEG_AMD64_SHA256"] != assetSHA256(ffmpeg, "amd64") || args["FFMPEG_ARM64_SHA256"] != assetSHA256(ffmpeg, "arm64") {
		return errors.New("dockerfile FFmpeg inputs differ from redistribution manifest")
	}
	if args["YTDLP_VERSION"] != ytdlp.Upstream.ReleaseRev || args["YTDLP_AMD64_SHA256"] != assetSHA256(ytdlp, "amd64") || args["YTDLP_ARM64_SHA256"] != assetSHA256(ytdlp, "arm64") {
		return errors.New("dockerfile yt-dlp inputs differ from redistribution manifest")
	}
	instructions := dockerfileInstructions(string(dockerfile))
	lastFrom := -1
	for i, instruction := range instructions {
		if strings.HasPrefix(instruction, "FROM ") {
			lastFrom = i
		}
	}
	if lastFrom < 0 {
		return errors.New("dockerfile has no final runtime stage")
	}
	var runtimeRun string
	for _, instruction := range instructions[lastFrom+1:] {
		if strings.HasPrefix(instruction, "RUN ") && strings.Contains(instruction, "YTDLP_ASSET=") && strings.Contains(instruction, "FFMPEG_BUILD=") {
			runtimeRun = instruction
		}
	}
	if runtimeRun == "" {
		return errors.New("dockerfile final runtime stage has no active RUN recipe")
	}
	if err := rejectShellComments(runtimeRun); err != nil {
		return err
	}
	for _, recipe := range []string{
		"amd64) YTDLP_ASSET=yt-dlp_linux; YTDLP_SHA256=\"$YTDLP_AMD64_SHA256\"",
		"arm64) YTDLP_ASSET=yt-dlp_linux_aarch64; YTDLP_SHA256=\"$YTDLP_ARM64_SHA256\"",
		"amd64) YTDLP_ASSET=yt-dlp_linux; YTDLP_SHA256=\"$YTDLP_AMD64_SHA256\"; DENO_ARCH=x86_64; DENO_SHA256=\"$DENO_AMD64_SHA256\"; FFMPEG_ARCH=linux64; FFMPEG_SHA256=\"$FFMPEG_AMD64_SHA256\"",
		"arm64) YTDLP_ASSET=yt-dlp_linux_aarch64; YTDLP_SHA256=\"$YTDLP_ARM64_SHA256\"; DENO_ARCH=aarch64; DENO_SHA256=\"$DENO_ARM64_SHA256\"; FFMPEG_ARCH=linuxarm64; FFMPEG_SHA256=\"$FFMPEG_ARM64_SHA256\"",
		`FFMPEG_BUILD="ffmpeg-${FFMPEG_BUILD_ID}-${FFMPEG_ARCH}-gpl-${FFMPEG_SUFFIX}"`,
		`"https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/${YTDLP_ASSET}"`,
		`-o /usr/local/bin/yt-dlp`,
		`echo "${YTDLP_SHA256} /usr/local/bin/yt-dlp" | sha256sum -c -`,
		`"https://github.com/BtbN/FFmpeg-Builds/releases/download/${FFMPEG_RELEASE}/${FFMPEG_BUILD}.tar.xz"`,
		`-o /tmp/ffmpeg.tar.xz`,
		`echo "${FFMPEG_SHA256} /tmp/ffmpeg.tar.xz" | sha256sum -c -`,
	} {
		if !strings.Contains(runtimeRun, recipe) {
			return fmt.Errorf("dockerfile active redistribution recipe is missing %q", recipe)
		}
	}
	return nil
}

func rejectShellComments(command string) error {
	var quote byte
	escaped := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '#' && (i == 0 || strings.ContainsRune(" \t;|&()", rune(command[i-1]))) {
			return errors.New("dockerfile acquisition RUN contains an unquoted shell comment")
		}
	}
	return nil
}

func verifyRedistributionArtifact(artifact redistributionArtifact) error {
	if len(artifact.Assets) != 2 || artifact.Upstream.ReleaseURL == "" || artifact.Upstream.SourceURL == "" || artifact.Upstream.BuildURL == "" || len(artifact.Upstream.LicenseURLs) == 0 {
		return errors.New("requires two assets and release, source, build, and license references")
	}
	seen := map[string]bool{}
	for _, asset := range artifact.Assets {
		if asset.Architecture != "amd64" && asset.Architecture != "arm64" || seen[asset.Architecture] || asset.Asset == "" || !strings.HasPrefix(asset.AssetURL, "https://") || strings.Contains(asset.AssetURL, "latest") || strings.Contains(asset.AssetURL, "master") || strings.Contains(asset.AssetURL, "main") || !sha256Hex.MatchString(asset.SHA256) {
			return errors.New("invalid or duplicate architecture asset")
		}
		seen[asset.Architecture] = true
	}
	if !seen["amd64"] || !seen["arm64"] || !immutableGitRevision.MatchString(artifact.Upstream.ReleaseCommit) || !immutableGitRevision.MatchString(artifact.Upstream.SourceRev) || !immutableGitRevision.MatchString(artifact.Upstream.BuildRev) {
		return errors.New("release, source, and build revisions must be immutable git revisions")
	}
	want := map[string]struct{ release, releaseCommit, source, build, releaseRev, sourceRev, buildRev string }{
		"ffmpeg": {"https://github.com/BtbN/FFmpeg-Builds/releases/tag/autobuild-2026-07-31-14-10", "a99e8230eae00d1cee38f23076a7a1f55cd984e2", "https://github.com/FFmpeg/FFmpeg/commit/9b6c8969e05b4f0b29f0f85cd501be6b3e582e6b", "https://github.com/BtbN/FFmpeg-Builds/tree/a99e8230eae00d1cee38f23076a7a1f55cd984e2", "autobuild-2026-07-31-14-10", "9b6c8969e05b4f0b29f0f85cd501be6b3e582e6b", "a99e8230eae00d1cee38f23076a7a1f55cd984e2"},
		"yt-dlp": {"https://github.com/yt-dlp/yt-dlp/releases/tag/2026.08.19", "3a08beaf031ab68f966401ead017ac81fe8486cf", "https://github.com/yt-dlp/yt-dlp/tree/3a08beaf031ab68f966401ead017ac81fe8486cf", "https://github.com/yt-dlp/yt-dlp/blob/3a08beaf031ab68f966401ead017ac81fe8486cf/bundle/pyinstaller.py", "2026.08.19", "3a08beaf031ab68f966401ead017ac81fe8486cf", "3a08beaf031ab68f966401ead017ac81fe8486cf"},
	}
	w, ok := want[artifact.Name]
	if !ok || artifact.Upstream.ReleaseURL != w.release || artifact.Upstream.ReleaseCommit != w.releaseCommit || artifact.Upstream.SourceURL != w.source || artifact.Upstream.BuildURL != w.build || artifact.Upstream.ReleaseRev != w.releaseRev || artifact.Upstream.SourceRev != w.sourceRev || artifact.Upstream.BuildRev != w.buildRev {
		return errors.New("upstream release, source, or build identity differs from the researched authority")
	}
	if artifact.Name == "ffmpeg" {
		const policyRev = "a99e8230eae00d1cee38f23076a7a1f55cd984e2"
		if artifact.Upstream.RetentionClass != "monthly" || artifact.Upstream.RetentionPolicyRev != policyRev || artifact.Upstream.RetentionPolicyURL != "https://github.com/BtbN/FFmpeg-Builds/blob/"+policyRev+"/util/prunetags.sh" {
			return errors.New("FFmpeg release is not bound to BtbN's retained-monthly policy")
		}
	} else if artifact.Upstream.RetentionClass != "" || artifact.Upstream.RetentionPolicyURL != "" || artifact.Upstream.RetentionPolicyRev != "" {
		return errors.New("unexpected retention policy on non-BtbN artifact")
	}
	expectedAssets := map[string]map[string]string{
		"ffmpeg": {"amd64": "ffmpeg-n8.1.2-34-g9b6c8969e0-linux64-gpl-8.1.tar.xz", "arm64": "ffmpeg-n8.1.2-34-g9b6c8969e0-linuxarm64-gpl-8.1.tar.xz"},
		"yt-dlp": {"amd64": "yt-dlp_linux", "arm64": "yt-dlp_linux_aarch64"},
	}
	expectedLicenses := map[string][]string{
		"ffmpeg": {"https://github.com/BtbN/FFmpeg-Builds/blob/a99e8230eae00d1cee38f23076a7a1f55cd984e2/LICENSE", "https://github.com/FFmpeg/FFmpeg/blob/9b6c8969e05b4f0b29f0f85cd501be6b3e582e6b/COPYING.GPLv3"},
		"yt-dlp": {"https://github.com/yt-dlp/yt-dlp/blob/3a08beaf031ab68f966401ead017ac81fe8486cf/LICENSE", "https://github.com/yt-dlp/yt-dlp/blob/3a08beaf031ab68f966401ead017ac81fe8486cf/THIRD_PARTY_LICENSES.txt", "https://github.com/yt-dlp/yt-dlp/releases/download/2026.08.19/SHA2-256SUMS"},
	}
	if !slices.Equal(artifact.Upstream.LicenseURLs, expectedLicenses[artifact.Name]) {
		return errors.New("license URL set differs from the researched authority")
	}
	for _, licenseURL := range artifact.Upstream.LicenseURLs {
		if !strings.HasPrefix(licenseURL, "https://") || strings.Contains(licenseURL, "latest") || strings.Contains(licenseURL, "master") || strings.Contains(licenseURL, "main") {
			return errors.New("license references must be immutable HTTPS URLs")
		}
	}
	for _, asset := range artifact.Assets {
		if asset.Asset != expectedAssets[artifact.Name][asset.Architecture] {
			return fmt.Errorf("unexpected %s asset for %s", artifact.Name, asset.Architecture)
		}
		if asset.AssetURL != artifact.Upstream.ReleaseURL[:strings.Index(artifact.Upstream.ReleaseURL, "/tag/")]+"/download/"+artifact.Upstream.ReleaseRev+"/"+asset.Asset {
			return fmt.Errorf("asset URL does not match its immutable release and asset name")
		}
	}
	return nil
}

func assetSHA256(artifact redistributionArtifact, architecture string) string {
	for _, asset := range artifact.Assets {
		if asset.Architecture == architecture {
			return asset.SHA256
		}
	}
	return ""
}
