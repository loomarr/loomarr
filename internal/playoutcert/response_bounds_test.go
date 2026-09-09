package playoutcert

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestBoundedReaderAcceptsExactLimitAndRejectsSuffix(t *testing.T) {
	const limit = 16
	got, err := io.ReadAll(newBoundedReader(strings.NewReader(strings.Repeat("x", limit)), limit))
	if err != nil || len(got) != limit {
		t.Fatalf("exact limit: len=%d err=%v", len(got), err)
	}
	got, err = io.ReadAll(newBoundedReader(strings.NewReader(strings.Repeat("x", limit)+"!"), limit))
	if !errors.Is(err, errResponseTooLarge) || len(got) != limit {
		t.Fatalf("suffix: len=%d err=%v", len(got), err)
	}
}

func TestEndpointRejectsOverLimitResponseSuffixes(t *testing.T) {
	fixture := playoutcertfixture.New(t, 1)
	// Exceed the same byte cap with bounded comment lines. Millions of tiny
	// lines measure scanner/allocator throughput before reaching the size bound.
	metricsComment := "#" + strings.Repeat("x", (32<<10)-2) + "\n"
	fixture.ResponsePadding = map[string]string{
		"/v1/system/version": strings.Repeat(" ", 2<<20),
		"/metrics":           strings.Repeat(metricsComment, 128),
	}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, Client: fixture.Server.Client(), RequestTimeout: time.Second}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	var version struct{ Ready bool }
	if err := endpoint.getJSON(context.Background(), "/v1/system/version", true, &version); !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("version suffix error = %v", err)
	}
	if _, err := endpoint.metrics(context.Background()); !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("metrics suffix error = %v", err)
	}
	if _, err := endpoint.metricTotal(context.Background(), "loomarr_playout_session_starts_total"); !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("metric total suffix error = %v", err)
	}
}

func TestEndpointRejectsTrailingJSONAndHLSManifestSuffix(t *testing.T) {
	fixture := playoutcertfixture.New(t, 1)
	fixture.ResponsePadding = map[string]string{
		"/v1/channels/channel/play-url":     "{}",
		"/v1/playout/hls/other/master.m3u8": strings.Repeat("#", 1<<20),
	}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, Client: fixture.Server.Client(), RequestTimeout: time.Second}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, class := endpoint.mint(context.Background(), "channel"); class != "invalid_response" {
		t.Fatalf("trailing JSON class = %q", class)
	}
	signed, _, class := endpoint.mint(context.Background(), "other")
	if class != "ok" {
		t.Fatalf("mint = %q", class)
	}
	if _, ok, class := endpoint.prepared(context.Background(), signed); ok || class != "body_failed" {
		t.Fatalf("manifest suffix: ok=%t class=%q", ok, class)
	}
}

func TestPreparedHLSReaderRejectsOversizeSegmentInsteadOfEndingEpoch(t *testing.T) {
	fixture := playoutcertfixture.NewPreparedHLS(t, playoutcertfixture.PreparedHLSReplacement)
	fixture.AssetBodies = map[string][]byte{"/a": bytes.Repeat([]byte("x"), 32<<20+1)}
	config := Config{BaseURL: fixture.Server.URL, AdminBearer: fixture.Admin, Client: fixture.Server.Client(), RequestTimeout: time.Second}
	endpoint, err := newEndpoint(config.normalized())
	if err != nil {
		t.Fatal(err)
	}
	signed, _, class := endpoint.mint(context.Background(), "prepared")
	if class != "ok" {
		t.Fatalf("mint class = %q", class)
	}
	reader := newPreparedHLSReader(context.Background(), endpoint, signed)
	n, err := io.Copy(io.Discard, reader)
	if !errors.Is(err, errResponseTooLarge) || n != 188+(32<<20) {
		t.Fatalf("segment result: bytes=%d err=%v", n, err)
	}
}

func TestPreparedHLSAssetClosesOnceWhenEOFOverlapsClose(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	var closes atomic.Int32
	var cancellations atomic.Int32
	body := playoutcertfixture.NewGatedTerminalBody(io.NopCloser(strings.NewReader("")), gate, 0, io.EOF, &closes)
	body.ReadStarted = started
	asset := newAssetReadCloser(&cancelBody{ReadCloser: body, cancel: func() { cancellations.Add(1) }}, body)
	readDone := make(chan error, 1)
	go func() {
		_, err := asset.Read(make([]byte, 1))
		readDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("asset read did not reach terminal gate")
	}
	close(gate)
	closeDone := make(chan error, 1)
	go func() { closeDone <- asset.Close() }()
	if err := <-readDone; !errors.Is(err, io.EOF) {
		t.Fatalf("read error = %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if got := closes.Load(); got != 1 {
		t.Fatalf("underlying closes = %d", got)
	}
	if got := cancellations.Load(); got != 1 {
		t.Fatalf("cancellations = %d", got)
	}
}
