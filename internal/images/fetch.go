package images

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// The fetch and rehydrate jobs (§22, §18.1 — V52 phase 3b).
//
// `Adopt` records a remote image and deliberately does NOT download it: a guide page adopting
// fifty posters would put TMDB's latency on Loomarr's own page load. This is what turns those rows
// into bytes, and it is the one place in the product that makes an outbound request driven by
// stored input — which is why the SSRF guard below is a hard gate rather than a precaution.

// fetchBatch bounds one pass's work list.
//
// ⚠ A constant rather than a setting. §15 owns configuration and the operator-facing knob already
// exists in a better form: `images.remote_max_concurrency` decides how hard a pass hits upstream,
// while this only decides how much of the backlog one tick takes. Making both configurable would
// give an operator two dials that interact and no way to reason about either.
const fetchBatch = 64

// fetchTimeout is the per-image ceiling, matching §6's per-service timeout posture. It bounds the
// whole exchange — connect, redirects and body — because a server that trickles bytes forever is
// the failure a connect timeout does not catch.
const fetchTimeout = 30 * time.Second

// maxFetchRedirects is deliberately small. Artwork CDNs redirect once, occasionally twice; a chain
// longer than this is either a loop or something trying to walk us somewhere.
const maxFetchRedirects = 3

// FetchStore is the persistence the fetch and rehydrate jobs need, on top of the serve path's.
//
// ⚠ A superset of Store rather than a widening of it. `New` should keep demanding only what
// serving an image requires — a Service built for a request path has no business being handed a
// method that deletes rows.
type FetchStore interface {
	Store
	// ListAwaitingFetch is the pending queue: remote rows whose bytes have never landed.
	ListAwaitingFetch(ctx context.Context, limit int) ([]Image, error)
	// ListByOrigin is rehydrate's work list — every row of an origin, whatever its fetch state,
	// because a file can go missing under a row in any state.
	ListByOrigin(ctx context.Context, origin Origin, limit int) ([]Image, error)
	// RepointRefs moves an image's refs onto another hash. The re-key — see fetchOne.
	RepointRefs(ctx context.Context, from, to string) error
	DeleteImage(ctx context.Context, hash string) error
}

// FetchResult is what one pass did, for the log line and the job's return.
type FetchResult struct {
	Considered int
	Fetched    int
	Failed     int
	// Rekeyed counts images whose identity changed because the bytes were not what the previous
	// fetch had stored. Worth counting separately: it is normal on first fetch and a signal
	// upstream replaced the artwork when it happens during a rehydrate or a TTL refresh.
	Rekeyed int
}

// Fetcher pulls bytes for remote images.
type Fetcher struct {
	svc   *Service
	store FetchStore
	log   *slog.Logger

	// enabled is `images.remote_fetch_enabled`, read per run so turning it off takes effect on the
	// next tick rather than the next restart.
	enabled func() bool
	// concurrency is `images.remote_max_concurrency`. ⚠ TMDB caps a client at 20 simultaneous
	// connections per IP (§22); the declared default sits well below it, and this is read live so
	// an operator throttling a saturated link does not have to restart.
	concurrency func() int
	// allowHosts is the SSRF allowlist — see checkURL. Derived at wiring from what the install is
	// actually configured to talk to, never operator free-text.
	allowHosts func() []string

	client *http.Client
}

// NewFetcher builds the fetcher. A nil logger is replaced with a discarding one so a test does not
// have to supply one.
func NewFetcher(svc *Service, st FetchStore, enabled func() bool, concurrency func() int, allowHosts func() []string, log *slog.Logger) *Fetcher {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	f := &Fetcher{svc: svc, store: st, log: log, enabled: enabled, concurrency: concurrency, allowHosts: allowHosts}
	f.client = &http.Client{
		Timeout:       fetchTimeout,
		CheckRedirect: f.checkRedirect,
	}
	return f
}

// FetchPending downloads the bytes for adopted-but-unfetched rows.
func (f *Fetcher) FetchPending(ctx context.Context) (FetchResult, error) {
	if !f.enabled() {
		return FetchResult{}, nil
	}
	pending, err := f.store.ListAwaitingFetch(ctx, fetchBatch)
	if err != nil {
		return FetchResult{}, err
	}
	res, _ := f.fetchAll(ctx, pending)
	return res, nil
}

// Rehydrate re-fetches remote images whose files are gone — the post-restore path (§22).
//
// ⚠ It exists because NOTHING else notices a missing file. The database survives a restore intact
// while `/data/images` may not, and every affected row otherwise looks perfectly healthy: it has a
// hash, dimensions, a placeholder, and a rendition request that quietly 404s. The disk is the only
// place the truth lives, so this is a stat sweep rather than a query.
func (f *Fetcher) Rehydrate(ctx context.Context) (FetchResult, error) {
	if !f.enabled() {
		return FetchResult{}, nil
	}
	rows, err := f.store.ListByOrigin(ctx, OriginRemote, fetchBatch)
	if err != nil {
		return FetchResult{}, err
	}

	missing := make([]Image, 0, len(rows))
	for _, img := range rows {
		if img.SourceURL == "" {
			continue // nothing to re-fetch from; the GC's warning covers this case
		}
		path, err := f.svc.blob.OriginalPath(img.Hash, extForMIME(img.MIME))
		if err != nil {
			continue
		}
		if _, ok := f.svc.blob.Stat(path); !ok {
			missing = append(missing, img)
		}
	}
	if len(missing) == 0 {
		return FetchResult{}, nil
	}
	f.log.Info("rehydrating images whose files are missing", "count", len(missing))
	res, _ := f.fetchAll(ctx, missing)
	return res, nil
}

// FetchNow fetches a work list synchronously for an INTERACTIVE caller and returns the resulting
// rows keyed by source URL. Anything that does not land inside `budget` is simply absent.
//
// ⚠ **Keyed by source URL rather than by hash, and that is the entire reason it returns a map.**
// fetchOne RE-KEYS: the row `Adopt` created under a hash of the URL is deleted the moment real
// bytes arrive and their content hash becomes the identity. So a caller holding the record `Adopt`
// handed back is holding a hash that no longer resolves — it would render a 404 and look exactly
// like a broken image. `source_url` is the one field carried across the re-key unchanged, which
// makes it the only key a caller can still match on afterwards.
//
// ⚠ **This is a deliberate exception to the rule stated at the top of this file, not a repeal of
// it.** Adopt does not download because a guide page adopting fifty posters would put TMDB's
// latency on Loomarr's page load. The icon picker is the opposite shape: a dozen images, on an
// explicit operator action, where the whole point of the screen is to look at them. Fetching there
// is not new latency either — it is the same dozen downloads the browser was making itself before
// §22, moved server-side so they stop being third-party requests from the operator's browser.
//
// Best-effort by construction: a miss is never an error. `images-fetch` runs every minute and owns
// whatever did not make it, so the caller renders a placeholder for the difference and the screen
// is complete a moment later.
func (f *Fetcher) FetchNow(ctx context.Context, work []Image, budget time.Duration) map[string]Image {
	out := make(map[string]Image, len(work))
	if !f.enabled() {
		return out
	}

	todo := make([]Image, 0, len(work))
	for _, img := range work {
		if img.SourceURL == "" {
			continue
		}
		if !img.OriginFetchedAt.IsZero() {
			out[img.SourceURL] = img // already warm — nothing to download
			continue
		}
		todo = append(todo, img)
	}
	if len(todo) == 0 {
		return out
	}

	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	_, byURL := f.fetchAll(ctx, todo)
	for u, rec := range byURL {
		out[u] = rec
	}
	return out
}

// fetchAll runs a work list under the concurrency cap.
//
// ⚠ One image's failure is not the pass's failure. A dead poster URL would otherwise leave the
// Tasks row permanently red and stop the other sixty-three images being fetched at all. An error
// comes back only when EVERY attempt failed, which is the shape that distinguishes "one bad URL"
// from "the network is gone" — the second is worth an operator's attention and the first is not.
// It returns the post-fetch records keyed by SOURCE URL alongside the counts. The jobs discard the
// map; FetchNow is the reason it exists — see there for why the key is the URL and not the hash.
func (f *Fetcher) fetchAll(ctx context.Context, work []Image) (FetchResult, map[string]Image) {
	limit := f.concurrency()
	if limit < 1 {
		limit = 1
	}

	var g errgroup.Group
	g.SetLimit(limit)
	results := make([]fetchOutcome, len(work))
	for i, img := range work {
		g.Go(func() error {
			results[i] = f.fetchOne(ctx, img)
			return nil
		})
	}
	_ = g.Wait() // every goroutine returns nil; the outcome is in results

	out := FetchResult{Considered: len(work)}
	byURL := make(map[string]Image, len(work))
	for i, r := range results {
		switch {
		case r.err != nil:
			out.Failed++
			f.log.Warn("image fetch failed", "hash", work[i].Hash, "url", work[i].SourceURL, "err", r.err)
		default:
			out.Fetched++
			if r.rekeyed {
				out.Rekeyed++
			}
			if r.rec.SourceURL != "" {
				byURL[r.rec.SourceURL] = r.rec
			}
		}
	}
	return out, byURL
}

type fetchOutcome struct {
	err     error
	rekeyed bool
	// rec is the row as it exists AFTER the fetch — which is not the row that went in, because
	// fetchOne re-keys. Only FetchNow reads it; the jobs care about counts.
	rec Image
}

// fetchOne downloads one image and reconciles its identity with the bytes that arrived.
//
// ⚠ **The re-key is the subtle half.** `Adopt` cannot know the content hash before the bytes
// exist, so it keys the row on a namespaced hash of the SOURCE URL. When the bytes land, identity
// must become the real content hash or `Cache-Control: immutable` stops being true — a URL would
// keep its name while its content changed, which is precisely what content addressing exists to
// prevent. So this writes a NEW row under the content hash, moves the refs, and deletes the
// placeholder.
//
// The same machinery covers a case that is not a placeholder at all: a TTL refresh or a rehydrate
// of an image upstream has since REPLACED. The hashes differ, the row re-keys, and every derivative
// URL changes — which is the honest outcome, because the bytes really are different.
func (f *Fetcher) fetchOne(ctx context.Context, img Image) fetchOutcome {
	if err := f.checkURL(img.SourceURL); err != nil {
		return fetchOutcome{err: err}
	}

	data, err := f.download(ctx, img.SourceURL)
	if err != nil {
		return fetchOutcome{err: err}
	}

	// ⚠ Decode before anything touches disk, exactly as Ingest does: it is the format allowlist
	// and the only proof the bytes are an image rather than something merely served with an image
	// content type. An upstream that returns an HTML error page with a 200 gets rejected here.
	decoded, mime, err := Decode(data)
	if err != nil {
		return fetchOutcome{err: err}
	}

	hash := HashBytes(data)
	dst, err := f.svc.blob.OriginalPath(hash, extForMIME(mime))
	if err != nil {
		return fetchOutcome{err: err}
	}
	if _, exists := f.svc.blob.Stat(dst); !exists {
		if err := f.svc.blob.Write(dst, data); err != nil {
			return fetchOutcome{err: err}
		}
	}

	now := f.svc.now()
	bounds := decoded.Bounds()
	rec := img // carries origin, source URL, visibility, role, meta and created_at forward
	rec.Hash = hash
	rec.MIME = mime
	rec.Width, rec.Height = bounds.Dx(), bounds.Dy()
	rec.Bytes = int64(len(data))
	rec.Animated = isAnimatedWebP(data, mime)
	rec.Placeholder = Placeholder(decoded)
	rec.DominantHex = DominantHex(decoded)
	rec.OriginFetchedAt = now
	rec.UpdatedAt = now
	rec.LastUsedAt = now
	if err := f.store.PutImage(ctx, rec); err != nil {
		return fetchOutcome{err: err}
	}

	if hash == img.Hash {
		return fetchOutcome{rec: rec} // a re-fetch that produced identical bytes: same row, same URLs
	}

	// ⚠ Order matters and is not interchangeable. The new row exists before the refs move, so no
	// ref ever points at a hash with no row; the refs move before the old row is deleted, because
	// `image_refs` cascades on delete and doing it the other way round would silently drop the
	// association the image existed to record.
	if err := f.store.RepointRefs(ctx, img.Hash, hash); err != nil {
		return fetchOutcome{err: err}
	}
	if err := f.store.DeleteImage(ctx, img.Hash); err != nil {
		return fetchOutcome{err: err}
	}
	// The superseded bytes, if there were any. A placeholder row never had an original; a replaced
	// one did, and leaving it would make the disk grow by one original per upstream artwork change
	// with nothing that ever collects it (the GC evicts derivatives and orphan rows, and this hash
	// no longer has a row to be an orphan).
	if old, err := f.svc.blob.OriginalPath(img.Hash, extForMIME(img.MIME)); err == nil {
		_ = f.svc.blob.Remove(old)
	}
	_ = f.svc.blob.RemoveAllFor(img.Hash)

	return fetchOutcome{rekeyed: true, rec: rec}
}

// download performs the guarded GET and returns the body, capped.
func (f *Fetcher) download(ctx context.Context, src string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, fmt.Errorf("images: build request for %s: %w", src, err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("images: fetch %s: %w", src, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// ⚠ Not retried here and not distinguished by status. A 429 or a 5xx is upstream asking
		// for less traffic, and the retry that respects that is the NEXT scheduled pass — the row
		// stays in the work list because origin_fetched_at is untouched, so backing off is the
		// default behaviour rather than something this code has to implement.
		return nil, fmt.Errorf("images: fetch %s: upstream returned %s", src, resp.Status)
	}

	max := f.svc.cfg.MaxUploadBytes()
	data, err := readCapped(resp.Body, max)
	if err != nil {
		return nil, fmt.Errorf("images: fetch %s: %w", src, err)
	}
	return data, nil
}

// checkURL is the SSRF gate on the initial request (§22).
//
// ⚠ **The allowlist is the control, and the private-address rule deliberately does NOT apply
// here.** A self-hosted media server is normally ON a private address — that is what self-hosted
// means — so a blanket private-range ban would refuse to fetch exactly the artwork this install
// owns. What makes the first hop safe is that its host came from the install's own configuration,
// not from the image row: an operator pointed Loomarr at that server. Redirects are the opposite
// case and are checked accordingly.
func (f *Fetcher) checkURL(raw string) error {
	if raw == "" {
		return errors.New("images: no source URL to fetch")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("images: unparseable source URL %q: %w", raw, err)
	}
	if u.Scheme != "https" {
		// https only: these fetches carry no credentials, but a plaintext hop is one an attacker
		// on the path can replace with bytes of their choosing, and we store what comes back.
		return fmt.Errorf("images: refusing a non-https source URL %q", raw)
	}
	if !hostAllowed(u.Hostname(), f.allowHosts()) {
		return fmt.Errorf("images: host %q is not one this install fetches images from", u.Hostname())
	}
	return nil
}

// checkRedirect bounds and inspects every hop after the first.
//
// ⚠ A redirect is attacker-influenced in a way the first URL is not: an allowlisted host can be
// made to answer with a Location pointing anywhere, which is the classic route from "we only talk
// to TMDB" to "we just read your metadata service". So each hop must stay https and must not
// resolve into a private, loopback or link-local range.
func (f *Fetcher) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxFetchRedirects {
		return fmt.Errorf("images: too many redirects (%d)", len(via))
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("images: refusing a redirect to %s", req.URL.Scheme)
	}
	host := req.URL.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("images: refusing a redirect into a private address (%s)", host)
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(req.Context(), host)
	if err != nil {
		return fmt.Errorf("images: cannot resolve redirect target %q: %w", host, err)
	}
	for _, a := range addrs {
		if isPrivateIP(a.IP) {
			return fmt.Errorf("images: refusing a redirect into a private address (%s -> %s)", host, a.IP)
		}
	}
	return nil
}

// isPrivateIP covers the ranges an image host has no business being in.
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// hostAllowed matches a host against the allowlist, case-insensitively.
//
// ⚠ Exact match or a dot-anchored suffix — never `strings.Contains` and never a bare suffix. An
// entry of "tmdb.org" must match "image.tmdb.org" and must NOT match "eviltmdb.org", and the dot
// is the whole difference between those two.
func hostAllowed(host string, allow []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, a := range allow {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if host == a || strings.HasSuffix(host, "."+a) {
			return true
		}
	}
	return false
}
