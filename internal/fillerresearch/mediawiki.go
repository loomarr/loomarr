package fillerresearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	MediaWikiAdapterVersion = "mediawiki-v2"
	mediaWikiEndpoint       = "https://en.wikipedia.org/w/api.php"
	mediaWikiTimeout        = 12 * time.Second
	mediaWikiMaxBody        = 64 << 10
)

type MediaWikiConfig struct {
	Client               *http.Client
	UserAgent            string
	Endpoint             string
	AllowInsecureTestURL bool
	Now                  func() time.Time
}

// MediaWiki is a fixed-host adapter, not a general HTTP fetcher. Model output never reaches its URL.
type MediaWiki struct {
	client    *http.Client
	endpoint  *url.URL
	userAgent string
	now       func() time.Time
}

func (*MediaWiki) Identity() (string, string) { return "mediawiki", MediaWikiAdapterVersion }

func NewMediaWiki(config MediaWikiConfig) (*MediaWiki, error) {
	raw := strings.TrimSpace(config.Endpoint)
	if raw == "" {
		raw = mediaWikiEndpoint
	}
	endpoint, err := url.Parse(raw)
	loopback := config.AllowInsecureTestURL && endpoint != nil &&
		(endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "localhost" || endpoint.Hostname() == "::1")
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" ||
		(!loopback && (endpoint.Scheme != "https" || endpoint.Hostname() != "en.wikipedia.org" || endpoint.Path != "/w/api.php")) {
		return nil, fmt.Errorf("%w: MediaWiki requires the canonical API endpoint", ErrInvalid)
	}
	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		return nil, fmt.Errorf("%w: MediaWiki requires an identifying User-Agent", ErrInvalid)
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: mediaWikiTimeout}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &MediaWiki{client: sameOriginClient(client, endpoint), endpoint: endpoint, userAgent: userAgent, now: now}, nil
}

type mediaWikiResponse struct {
	Query struct {
		Pages []struct {
			Title   string `json:"title"`
			Extract string `json:"extract"`
			FullURL string `json:"fullurl"`
		} `json:"pages"`
	} `json:"query"`
	Error *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"error,omitempty"`
}

func (m *MediaWiki) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if err := lookup.Validate(); err != nil {
		return Packet{}, err
	}
	terms := lookup.Terms()
	queries := make([]string, 0, len(terms))
	for _, term := range terms {
		queries = append(queries, quotedSearchPhrase(term))
	}
	query := strings.Join(queries, " OR ")
	u := *m.endpoint
	values := u.Query()
	values.Set("action", "query")
	values.Set("generator", "search")
	values.Set("gsrsearch", query)
	values.Set("gsrlimit", fmt.Sprint(MaxAdapterResults))
	values.Set("prop", "extracts|info")
	values.Set("explaintext", "1")
	values.Set("exchars", fmt.Sprint(MaxExtractBytes))
	values.Set("inprop", "url")
	values.Set("format", "json")
	values.Set("formatversion", "2")
	values.Set("maxlag", "5")
	u.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Packet{}, fmt.Errorf("build MediaWiki request: %w", err)
	}
	req.Header.Set("User-Agent", m.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return Packet{}, fmt.Errorf("retrieve MediaWiki evidence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Packet{}, fmt.Errorf("retrieve MediaWiki evidence: upstream returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, mediaWikiMaxBody+1))
	if err != nil {
		return Packet{}, fmt.Errorf("read MediaWiki evidence: %w", err)
	}
	if len(body) > mediaWikiMaxBody {
		return Packet{}, fmt.Errorf("retrieve MediaWiki evidence: response exceeds %d bytes", mediaWikiMaxBody)
	}
	var decoded mediaWikiResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Packet{}, fmt.Errorf("decode MediaWiki evidence: %w", err)
	}
	if decoded.Error != nil {
		return Packet{}, fmt.Errorf("retrieve MediaWiki evidence: %s: %s", decoded.Error.Code, decoded.Error.Info)
	}
	packet := Packet{Query: lookup.CanonicalTitle(), Adapter: "mediawiki", AdapterVersion: MediaWikiAdapterVersion,
		RetrievedAt: m.now().UTC()}
	for _, page := range decoded.Query.Pages {
		if len(packet.Citations) == MaxAdapterResults {
			break
		}
		extract := strings.TrimSpace(page.Extract)
		if len(extract) > MaxExtractBytes {
			extract = extract[:MaxExtractBytes]
		}
		citation := Citation{ID: len(packet.Citations) + 1, Title: strings.TrimSpace(page.Title),
			URL: strings.TrimSpace(page.FullURL), Extract: extract}
		pageURL, err := url.Parse(citation.URL)
		if citation.Title == "" || extract == "" || err != nil || pageURL.Scheme != "https" ||
			pageURL.Hostname() != "en.wikipedia.org" || !strings.HasPrefix(pageURL.Path, "/wiki/") {
			continue
		}
		packet.Citations = append(packet.Citations, citation)
	}
	if len(packet.Citations) == 0 {
		return Packet{}, fmt.Errorf("retrieve MediaWiki evidence: no attributable results")
	}
	if err := packet.Validate(); err != nil {
		return Packet{}, err
	}
	return packet, nil
}
