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

// The FFmpeg pin follows BtbN's newest release-series build (scripts/ffmpeg-pin-refresh.sh), so its
// identity is not a researched constant here. What is fixed is the SHAPE: every URL is derived from
// the pinned revisions, the binaries come from Loomarr's own source release, and that release is
// recorded as carrying the binaries, the exact source, the recipe and the original dependency cache.

const (
	btbnRepoURL    = "https://github.com/BtbN/FFmpeg-Builds"
	ffmpegRepoURL  = "https://github.com/FFmpeg/FFmpeg"
	loomarrRepoURL = "https://github.com/loomarr/loomarr"
	ffmpegCache    = "ffmpeg-original-download-cache.zip"
)

var (
	ffmpegBuildID    = regexp.MustCompile(`^n([0-9]+)\.([0-9]+)(\.[0-9]+)?-[0-9]+-g([0-9a-f]{7,40})$`)
	btbnAutobuild    = regexp.MustCompile(`^autobuild-[0-9]{4}(-[0-9]{2}){4}$`)
	ffmpegMirrorTag  = regexp.MustCompile(`^third-party-sources-([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
	fullGitRevision  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	ffmpegCachePart  = regexp.MustCompile(`^` + regexp.QuoteMeta(ffmpegCache) + `\.part[0-9]{2}$`)
	ffmpegPinNotices = regexp.MustCompile(`(?s)<!-- ffmpeg-pin:begin -->\n(.*?)<!-- ffmpeg-pin:end -->`)
)

func ffmpegSeries(buildID string) (string, error) {
	m := ffmpegBuildID.FindStringSubmatch(buildID)
	if m == nil {
		return "", fmt.Errorf("FFmpeg build id %q is not a release-series build", buildID)
	}
	return m[1] + "." + m[2], nil
}

func verifyFFmpegIdentity(artifact redistributionArtifact) error {
	u := artifact.Upstream
	series, err := ffmpegSeries(u.BuildID)
	if err != nil {
		return err
	}
	abbrev := ffmpegBuildID.FindStringSubmatch(u.BuildID)[4]
	if !btbnAutobuild.MatchString(u.ReleaseRev) || u.ReleaseURL != btbnRepoURL+"/releases/tag/"+u.ReleaseRev {
		return errors.New("FFmpeg release must be a BtbN autobuild tag and its URL")
	}
	if !fullGitRevision.MatchString(u.ReleaseCommit) || !fullGitRevision.MatchString(u.SourceRev) || !fullGitRevision.MatchString(u.BuildRev) {
		return errors.New("FFmpeg release, source and recipe revisions must be full commit ids")
	}
	if !strings.HasPrefix(u.SourceRev, abbrev) || u.SourceURL != ffmpegRepoURL+"/commit/"+u.SourceRev || u.BuildURL != btbnRepoURL+"/tree/"+u.BuildRev {
		return errors.New("FFmpeg source or recipe identity differs from the build id")
	}
	if u.RetentionClass != "mirrored" || !ffmpegMirrorTag.MatchString(u.RetentionPolicyRev) || u.RetentionPolicyURL != loomarrRepoURL+"/releases/tag/"+u.RetentionPolicyRev {
		return errors.New("FFmpeg binaries must be retained by a Loomarr third-party-sources release")
	}
	licenses := []string{btbnRepoURL + "/blob/" + u.BuildRev + "/LICENSE", ffmpegRepoURL + "/blob/" + u.SourceRev + "/COPYING.GPLv3"}
	if !slices.Equal(u.LicenseURLs, licenses) {
		return errors.New("license URL set differs from the pinned recipe and source")
	}
	for _, asset := range artifact.Assets {
		arch := map[string]string{"amd64": "linux64", "arm64": "linuxarm64"}[asset.Architecture]
		if asset.Asset != "ffmpeg-"+u.BuildID+"-"+arch+"-gpl-"+series+".tar.xz" {
			return fmt.Errorf("unexpected ffmpeg asset for %s", asset.Architecture)
		}
		if asset.AssetURL != loomarrRepoURL+"/releases/download/"+u.RetentionPolicyRev+"/"+asset.Asset {
			return errors.New("FFmpeg asset URL must be Loomarr's mirror release")
		}
	}
	return nil
}

type ffmpegSourceDistribution struct {
	SchemaVersion int    `json:"schemaVersion"`
	SourceRelease string `json:"sourceRelease"`
	SourceTag     string `json:"sourceTag"`
	ReleaseTarget string `json:"releaseTarget"`
	PublishedAt   string `json:"publishedAt"`
	Scope         string `json:"scope"`
	Assets        []struct {
		Name   string `json:"name"`
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
		URL    string `json:"url"`
	} `json:"assets"`
}

// verifyFFmpegSourceRelease proves, offline, that the pinned build's source release is recorded with
// everything the GPL corresponding-source offer needs and with the exact binaries the image fetches.
func verifyFFmpegSourceRelease(root string, artifact redistributionArtifact) error {
	u := artifact.Upstream
	date := ffmpegMirrorTag.FindStringSubmatch(u.RetentionPolicyRev)[1]
	path := filepath.Join(root, "project", "evidence", "source-distribution-"+date+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("FFmpeg pin has no source distribution record: %w", err)
	}
	var dist ffmpegSourceDistribution
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dist); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%s must contain exactly one JSON document", filepath.Base(path))
	}
	if dist.SchemaVersion != 1 || dist.SourceTag != u.RetentionPolicyRev || dist.SourceRelease != u.RetentionPolicyURL || !fullGitRevision.MatchString(dist.ReleaseTarget) || dist.PublishedAt == "" {
		return errors.New("source distribution record does not describe the pinned mirror release")
	}
	recorded := make(map[string]string, len(dist.Assets))
	cacheParts := 0
	for _, asset := range dist.Assets {
		if !sha256Hex.MatchString(asset.SHA256) || asset.Bytes <= 0 || asset.URL != loomarrRepoURL+"/releases/download/"+dist.SourceTag+"/"+asset.Name || recorded[asset.Name] != "" {
			return fmt.Errorf("source release asset %q is not an immutable, digested asset of %s", asset.Name, dist.SourceTag)
		}
		recorded[asset.Name] = asset.SHA256
		if asset.Name == ffmpegCache || ffmpegCachePart.MatchString(asset.Name) {
			cacheParts++
		}
	}
	for _, asset := range artifact.Assets {
		if recorded[asset.Asset] != asset.SHA256 {
			return fmt.Errorf("source release does not carry the pinned %s binary %s", asset.Architecture, asset.Asset)
		}
	}
	for _, name := range []string{"ffmpeg-" + u.SourceRev + ".tar.gz", "ffmpeg-builds-" + u.BuildRev[:8] + ".tar.gz", "README.txt", "SHA256SUMS", "source-package-inventory.json"} {
		if recorded[name] == "" {
			return fmt.Errorf("source release is missing %s", name)
		}
	}
	if cacheParts == 0 {
		return errors.New("source release is missing the original dependency download cache")
	}
	record, err := os.ReadFile(filepath.Join(root, "project", "evidence", "ffmpeg-pin-"+date+".md"))
	if err != nil || !strings.Contains(string(record), u.BuildID) || !strings.Contains(string(record), u.RetentionPolicyRev) {
		return errors.New("FFmpeg pin has no evidence record naming its build and source release")
	}
	notices, err := os.ReadFile(filepath.Join(root, "THIRD_PARTY_NOTICES.md"))
	if err != nil {
		return fmt.Errorf("read THIRD_PARTY_NOTICES.md: %w", err)
	}
	block := ffmpegPinNotices.FindSubmatch(notices)
	if block == nil || !strings.Contains(string(block[1]), u.BuildID) || !strings.Contains(string(block[1]), u.RetentionPolicyRev) {
		return errors.New("THIRD_PARTY_NOTICES.md ffmpeg-pin block does not name the pinned build and source release")
	}
	return nil
}
