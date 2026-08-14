package playout

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/prepared"
)

type fixedPreparedResolver struct {
	window PreparedWindow
	ok     bool
}

func (r fixedPreparedResolver) ResolvePrepared(context.Context, TuneRequest) (PreparedWindow, bool, error) {
	return r.window, r.ok, nil
}

func preparedSpec(source string) prepared.Specification {
	return prepared.Specification{
		SourceFingerprint: source,
		Rendition: prepared.RenditionContract{
			VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080,
			SegmentDurationMS: 2000, PackagingVersion: 1,
		},
	}
}

func publishHLS(t *testing.T, lib *prepared.Library, spec prepared.Specification) prepared.Publication {
	t.Helper()
	pub, err := lib.Publish(t.Context(), spec, func(_ context.Context, workspace string) (prepared.Output, error) {
		manifest := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:2\n" +
			"#EXT-X-MAP:URI=\"init.mp4\"\n" +
			"#EXTINF:2.000,\nseg-0.m4s\n#EXTINF:2.000,\nseg-1.m4s\n" +
			"#EXTINF:2.000,\nseg-2.m4s\n#EXTINF:2.000,\nseg-3.m4s\n#EXT-X-ENDLIST\n"
		files := []string{preparedManifestName, "init.mp4", "seg-0.m4s", "seg-1.m4s", "seg-2.m4s", "seg-3.m4s"}
		for _, file := range files {
			body := []byte(file)
			if file == preparedManifestName {
				body = []byte(manifest)
			}
			if err := os.WriteFile(filepath.Join(workspace, file), body, 0o600); err != nil {
				return prepared.Output{}, err
			}
		}
		return prepared.Output{Files: files}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestPreparedOriginRendersAKeyedWallClockManifest(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := preparedSpec("source-a")
	pub := publishHLS(t, lib, spec)
	started := time.Unix(1_000, 0).UTC()
	preparedOrigin := newPreparedOrigin(lib, fixedPreparedResolver{
		window: PreparedWindow{Specification: spec, StartedAt: started, Offset: 5 * time.Second}, ok: true,
	})

	presentation, hit, err := preparedOrigin.Tune(t.Context(), TuneRequest{
		ChannelID: "ch-one", Plan: PlanBaseline, Delivery: DeliveryHLS,
	})
	if err != nil || !hit {
		t.Fatalf("Tune = (_, %v, %v), want prepared hit", hit, err)
	}
	manifest := string(presentation.Manifest)
	for _, want := range []string{
		"#EXT-X-MEDIA-SEQUENCE:502", "#EXT-X-DISCONTINUITY-SEQUENCE:1000",
		fmt.Sprintf(`#EXT-X-MAP:URI="%s/init.mp4"`, pub.Key), pub.Key + "/seg-2.m4s",
	} {
		if !strings.Contains(manifest, want) {
			t.Errorf("manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, unwanted := range []string{pub.Key + "/seg-0.m4s", pub.Key + "/seg-1.m4s", "#EXT-X-ENDLIST"} {
		if strings.Contains(manifest, unwanted) {
			t.Errorf("manifest contains %q:\n%s", unwanted, manifest)
		}
	}

	asset, ok, err := preparedOrigin.OpenAsset("ch-one", PlanBaseline, pub.Key+"/seg-2.m4s")
	if err != nil || !ok {
		t.Fatalf("OpenAsset = (_, %v, %v), want hit", ok, err)
	}
	defer func() { _ = asset.Content.Close() }()
	body, err := io.ReadAll(asset.Content)
	if err != nil || string(body) != "seg-2.m4s" {
		t.Fatalf("asset body = %q, err=%v", body, err)
	}
}

func TestOriginPreparedHitBypassesLiveAndMissFallsBack(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := preparedSpec("source-a")
	publishHLS(t, lib, spec)
	hls := &tuneHLS{path: filepath.Join(t.TempDir(), "live.m3u8")}
	if err := os.WriteFile(hls.path, []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}

	hitOrigin := newOrigin(newPreparedOrigin(lib, fixedPreparedResolver{
		window: PreparedWindow{Specification: spec, StartedAt: time.Unix(1_000, 0), Offset: 0}, ok: true,
	}), nil, hls)
	got, err := hitOrigin.Tune(t.Context(), TuneRequest{ChannelID: "prepared", Plan: PlanBaseline, Delivery: DeliveryHLS})
	if err != nil || string(got.Manifest) == "live" || hls.channel != "" {
		t.Fatalf("prepared Tune = (%q, %v), live channel=%q", got.Manifest, err, hls.channel)
	}
	gotAgain, err := hitOrigin.Tune(t.Context(), TuneRequest{
		ChannelID: "another-channel", Plan: PlanBaseline, Delivery: DeliveryHLS,
	})
	if err != nil || string(gotAgain.Manifest) != string(got.Manifest) || hls.channel != "" {
		t.Fatalf("shared prepared Tune = (%q, %v), live channel=%q", gotAgain.Manifest, err, hls.channel)
	}

	missOrigin := newOrigin(newPreparedOrigin(lib, fixedPreparedResolver{ok: false}), nil, hls)
	got, err = missOrigin.Tune(t.Context(), TuneRequest{ChannelID: "fallback", Plan: PlanBaseline, Delivery: DeliveryHLS})
	if err != nil || string(got.Manifest) != "live" || hls.channel != "fallback" {
		t.Fatalf("fallback Tune = (%q, %v), live channel=%q", got.Manifest, err, hls.channel)
	}
}

func TestPreparedManifestCannotReferenceOutsideItsPublication(t *testing.T) {
	t.Parallel()
	spec := preparedSpec("source-a")
	window := PreparedWindow{Specification: spec, StartedAt: time.Unix(1_000, 0)}
	manifest := []byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\n../outside.m4s\n")
	if _, err := renderPreparedManifest(manifest, strings.Repeat("a", 64), []string{"media.m3u8"}, window); err == nil {
		t.Fatal("renderPreparedManifest accepted an asset outside the publication")
	}
}
