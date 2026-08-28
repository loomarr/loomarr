package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyRedistributionManifest(t *testing.T) {
	root := repositoryRoot(t)
	if err := VerifyRedistributionManifest(root); err != nil {
		t.Fatalf("repository redistribution manifest: %v", err)
	}
	manifestPath := filepath.Join(root, redistributionManifestPath)
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	dockerfilePath := filepath.Join(root, "Dockerfile")
	dockerfile, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]struct{ path, text string }{
		"missing architecture":                 {manifestPath, strings.Replace(string(manifest), `"architecture": "arm64"`, `"architecture": "removed"`, 1)},
		"checksum drift":                       {manifestPath, strings.Replace(string(manifest), "17780994c4679806fb227676f66a0af30c6379afc770324829f48f2a379be558", strings.Repeat("0", 64), 1)},
		"release commit drift":                 {manifestPath, strings.Replace(string(manifest), "590a6612d7d961e9258429e501619e0b7d7cbedf", "590a6612d7d961e9258429e501619e0b7d7cbed0", 1)},
		"license set drift":                    {manifestPath, strings.Replace(string(manifest), "COPYING.GPLv3", "LICENSE", 1)},
		"trailing JSON document":               {manifestPath, string(manifest) + "{}\n"},
		"alternate download host":              {dockerfilePath, strings.Replace(string(dockerfile), "github.com/yt-dlp/yt-dlp/releases", "example.com/yt-dlp/yt-dlp/releases", 1)},
		"inline comment URL bypass":            {dockerfilePath, strings.Replace(string(dockerfile), `"https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/${YTDLP_ASSET}"`, `"https://example.invalid/alternate" # "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/${YTDLP_ASSET}"`, 1)},
		"asset mapping drift":                  {dockerfilePath, strings.Replace(string(dockerfile), "YTDLP_ASSET=yt-dlp_linux_aarch64", "YTDLP_ASSET=yt-dlp_linux", 1)},
		"build construction drift":             {dockerfilePath, strings.Replace(string(dockerfile), `FFMPEG_BUILD="ffmpeg-${FFMPEG_BUILD_ID}-${FFMPEG_ARCH}-gpl-${FFMPEG_SUFFIX}"`, `FFMPEG_BUILD="alternate-${FFMPEG_BUILD_ID}"`, 1)},
		"checksum recipe drift":                {dockerfilePath, strings.Replace(string(dockerfile), `echo "${FFMPEG_SHA256}  /tmp/ffmpeg.tar.xz" | sha256sum -c -`, `echo "${FFMPEG_SHA256}  /tmp/ffmpeg.tar.xz"`, 1)},
		"inline comment mapping bypass":        {dockerfilePath, strings.Replace(string(dockerfile), `FFMPEG_ARCH=linux64;    FFMPEG_SHA256="$FFMPEG_AMD64_SHA256"`, `FFMPEG_ARCH=linuxarm64; FFMPEG_SHA256="$FFMPEG_AMD64_SHA256" # FFMPEG_ARCH=linux64;    FFMPEG_SHA256="$FFMPEG_AMD64_SHA256"`, 1)},
		"inline comment checksum bypass":       {dockerfilePath, strings.Replace(string(dockerfile), `echo "${FFMPEG_SHA256}  /tmp/ffmpeg.tar.xz" | sha256sum -c -`, `echo "${FFMPEG_SHA256}  /tmp/ffmpeg.tar.xz" # echo "${FFMPEG_SHA256}  /tmp/ffmpeg.tar.xz" | sha256sum -c -`, 1)},
		"comment cannot satisfy active recipe": {dockerfilePath, strings.Replace(string(dockerfile), `"https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/${YTDLP_ASSET}"`, `"https://example.invalid/alternate"`, 1) + "\n# original recipe retained only in this comment: https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/${YTDLP_ASSET}\n"},
		"FFmpeg architecture mapping drift":    {dockerfilePath, strings.Replace(string(dockerfile), `FFMPEG_ARCH=linux64;    FFMPEG_SHA256="$FFMPEG_AMD64_SHA256"`, `FFMPEG_ARCH=linuxarm64;    FFMPEG_SHA256="$FFMPEG_AMD64_SHA256"`, 1)},
		"Dockerfile version drift":             {dockerfilePath, strings.Replace(string(dockerfile), "ARG FFMPEG_BUILD_ID=n8.1.2-44-g7c533d0f86", "ARG FFMPEG_BUILD_ID=n8.1.2-44-g7c533d0f87", 1)},
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(mutation.path, []byte(mutation.text), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.WriteFile(mutation.path, map[string][]byte{manifestPath: manifest, dockerfilePath: dockerfile}[mutation.path], 0o600); err != nil {
					t.Error(err)
				}
			})
			if err := VerifyRedistributionManifest(root); err == nil {
				t.Fatal("accepted broken redistribution contract")
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(source), "..", "..")
}
