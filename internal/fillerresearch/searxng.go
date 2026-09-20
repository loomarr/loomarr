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

const SearXNGAdapterVersion = "searxng-v1"

type SearXNGConfig struct {
	Client               *http.Client
	Endpoint             string
	AllowInsecureTestURL bool
	Now                  func() time.Time
}

type SearXNG struct {
	client   *http.Client
	endpoint *url.URL
	now      func() time.Time
}

func NewSearXNG(config SearXNGConfig) (*SearXNG, error) {
	endpoint, err := url.Parse(strings.TrimSpace(config.Endpoint))
	loopback := config.AllowInsecureTestURL && endpoint != nil &&
		(endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "localhost" || endpoint.Hostname() == "::1")
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" ||
		(!loopback && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("%w: SearXNG requires a credential-free HTTPS endpoint", ErrInvalid)
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &SearXNG{client: sameOriginClient(config.Client, endpoint), endpoint: endpoint, now: config.Now}, nil
}

func (*SearXNG) Identity() (string, string) { return "searxng", SearXNGAdapterVersion }

func (s *SearXNG) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if err := lookup.Validate(); err != nil {
		return Packet{}, err
	}
	u := *s.endpoint
	u.Path = strings.TrimRight(u.Path, "/") + "/search"
	q := u.Query()
	q.Set("q", strings.Join(lookup.Terms(), " "))
	q.Set("format", "json")
	q.Set("safesearch", "1")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Packet{}, fmt.Errorf("build SearXNG request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Loomarr/1.0 (https://github.com/loomarr/loomarr)")
	resp, err := s.client.Do(req)
	if err != nil {
		return Packet{}, fmt.Errorf("retrieve SearXNG evidence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Packet{}, fmt.Errorf("retrieve SearXNG evidence: upstream returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, webSearchMaxBody+1))
	if err != nil || len(body) > webSearchMaxBody {
		return Packet{}, fmt.Errorf("retrieve SearXNG evidence: bounded response read failed")
	}
	var decoded struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Packet{}, fmt.Errorf("decode SearXNG evidence: %w", err)
	}
	packet := Packet{Query: lookup.CanonicalTitle(), Adapter: "searxng", AdapterVersion: SearXNGAdapterVersion,
		RetrievedAt: s.now().UTC()}
	for _, result := range decoded.Results {
		if citation, ok := webCitation(len(packet.Citations)+1, result.Title, result.URL, result.Content); ok {
			packet.Citations = append(packet.Citations, citation)
		}
		if len(packet.Citations) == MaxAdapterResults {
			break
		}
	}
	if len(packet.Citations) == 0 {
		return Packet{}, fmt.Errorf("retrieve SearXNG evidence: no attributable results")
	}
	return packet, packet.Validate()
}
