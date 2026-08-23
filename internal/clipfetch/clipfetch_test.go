package clipfetch_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/loomarr/loomarr/internal/clipfetch"
)

// fakeDL records which sources it was handed and returns scripted results.
type fakeDL struct {
	got     []clipfetch.Source
	fetched int
	err     error
}

func (f *fakeDL) Download(_ context.Context, src clipfetch.Source, _ string) (int, int, error) {
	f.got = append(f.got, src)
	if f.err != nil {
		return 0, 0, f.err
	}
	return f.fetched, 0, nil
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestKindForURL(t *testing.T) {
	cases := map[string]clipfetch.Kind{
		"https://youtube.com/playlist?list=abc":      clipfetch.YouTube,
		"https://youtu.be/xyz":                       clipfetch.YouTube,
		"https://archive.org/details/classic-tv-ads": clipfetch.Archive,
		"https://www.someothersite.com/vid":          clipfetch.YouTube, // default yt-dlp
	}
	for url, want := range cases {
		if got := clipfetch.KindForURL(url); got != want {
			t.Errorf("KindForURL(%q) = %q, want %q", url, got, want)
		}
	}
}

// Run dispatches each source to the downloader for its kind.
func TestRun_DispatchesByKind(t *testing.T) {
	yt, arch := &fakeDL{fetched: 2}, &fakeDL{fetched: 5}
	ing := clipfetch.New(yt, arch, "/drop", discardLog())

	res := ing.Run(context.Background(), []clipfetch.Source{
		{Kind: clipfetch.YouTube, URL: "https://youtube.com/playlist?list=a"},
		{Kind: clipfetch.Archive, URL: "https://archive.org/details/x"},
	})
	if len(yt.got) != 1 || yt.got[0].Kind != clipfetch.YouTube {
		t.Errorf("youtube downloader got %+v", yt.got)
	}
	if len(arch.got) != 1 || arch.got[0].Kind != clipfetch.Archive {
		t.Errorf("archive downloader got %+v", arch.got)
	}
	if res.Fetched != 7 { // 2 + 5
		t.Errorf("fetched = %d, want 7", res.Fetched)
	}
}

// A failing source is counted and logged, never fatal — the rest still run.
func TestRun_ResilientToSourceFailure(t *testing.T) {
	yt := &fakeDL{err: errors.New("playlist gone")}
	arch := &fakeDL{fetched: 3}
	ing := clipfetch.New(yt, arch, "/drop", discardLog())

	res := ing.Run(context.Background(), []clipfetch.Source{
		{Kind: clipfetch.YouTube, URL: "https://youtube.com/bad"},
		{Kind: clipfetch.Archive, URL: "https://archive.org/details/good"},
	})
	if res.Failed != 1 {
		t.Errorf("failed = %d, want 1", res.Failed)
	}
	if res.Fetched != 3 {
		t.Errorf("the good source should still run: fetched = %d, want 3", res.Fetched)
	}
}

// A source that returns no clips AND no error (a nonexistent/typo'd Archive id —
// Archive serves 200 {} for unknown items — or an empty source) must be counted
// as Empty, not silently reported as success. Otherwise the operator sees
// "fetched:0 failed:0" with no reason why nothing landed.
func TestRun_EmptySourceIsSurfaced(t *testing.T) {
	empty := &fakeDL{fetched: 0} // no error, nothing fetched — the silent case
	good := &fakeDL{fetched: 2}
	ing := clipfetch.New(empty, good, "/drop", discardLog())

	res := ing.Run(context.Background(), []clipfetch.Source{
		{Kind: clipfetch.YouTube, URL: "https://youtube.com/nonexistent"},
		{Kind: clipfetch.Archive, URL: "https://archive.org/details/real"},
	})
	if res.Empty != 1 {
		t.Errorf("empty = %d, want 1 (the yield-nothing source surfaced)", res.Empty)
	}
	if res.Failed != 0 {
		t.Errorf("an empty source is not a failure: failed = %d, want 0", res.Failed)
	}
	if res.Fetched != 2 {
		t.Errorf("the good source still runs: fetched = %d, want 2", res.Fetched)
	}
}
