package ingestkit_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/mantonx/loomarr/internal/ingestkit"
)

// fakeDL records which sources it was handed and returns scripted results.
type fakeDL struct {
	got     []ingestkit.Source
	fetched int
	err     error
}

func (f *fakeDL) Download(_ context.Context, src ingestkit.Source, _ string) (int, int, error) {
	f.got = append(f.got, src)
	if f.err != nil {
		return 0, 0, f.err
	}
	return f.fetched, 0, nil
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestKindForURL(t *testing.T) {
	cases := map[string]ingestkit.Kind{
		"https://youtube.com/playlist?list=abc":      ingestkit.YouTube,
		"https://youtu.be/xyz":                       ingestkit.YouTube,
		"https://archive.org/details/classic-tv-ads": ingestkit.Archive,
		"https://www.someothersite.com/vid":          ingestkit.YouTube, // default yt-dlp
	}
	for url, want := range cases {
		if got := ingestkit.KindForURL(url); got != want {
			t.Errorf("KindForURL(%q) = %q, want %q", url, got, want)
		}
	}
}

// Run dispatches each source to the downloader for its kind.
func TestRun_DispatchesByKind(t *testing.T) {
	yt, arch := &fakeDL{fetched: 2}, &fakeDL{fetched: 5}
	ing := ingestkit.New(yt, arch, "/drop", discardLog())

	res := ing.Run(context.Background(), []ingestkit.Source{
		{Kind: ingestkit.YouTube, URL: "https://youtube.com/playlist?list=a"},
		{Kind: ingestkit.Archive, URL: "https://archive.org/details/x"},
	})
	if len(yt.got) != 1 || yt.got[0].Kind != ingestkit.YouTube {
		t.Errorf("youtube downloader got %+v", yt.got)
	}
	if len(arch.got) != 1 || arch.got[0].Kind != ingestkit.Archive {
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
	ing := ingestkit.New(yt, arch, "/drop", discardLog())

	res := ing.Run(context.Background(), []ingestkit.Source{
		{Kind: ingestkit.YouTube, URL: "https://youtube.com/bad"},
		{Kind: ingestkit.Archive, URL: "https://archive.org/details/good"},
	})
	if res.Failed != 1 {
		t.Errorf("failed = %d, want 1", res.Failed)
	}
	if res.Fetched != 3 {
		t.Errorf("the good source should still run: fetched = %d, want 3", res.Fetched)
	}
}
