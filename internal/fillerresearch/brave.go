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
	BraveAdapterVersion = "brave-v1"
	braveEndpoint       = "https://api.search.brave.com/res/v1/web/search"
	webSearchMaxBody    = 64 << 10
)

type BraveConfig struct {
	Client               *http.Client
	APIKey               string
	Endpoint             string
	AllowInsecureTestURL bool
	Now                  func() time.Time
}

type Brave struct {
	client   *http.Client
	apiKey   string
	endpoint *url.URL
	now      func() time.Time
}

func NewBrave(config BraveConfig) (*Brave, error) {
	raw := strings.TrimSpace(config.Endpoint)
	if raw == "" {
		raw = braveEndpoint
	}
	endpoint, err := url.Parse(raw)
	loopback := config.AllowInsecureTestURL && endpoint != nil &&
		(endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "localhost" || endpoint.Hostname() == "::1")
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" ||
		(!loopback && (endpoint.Scheme != "https" || endpoint.Hostname() != "api.search.brave.com" || endpoint.Path != "/res/v1/web/search")) {
		return nil, fmt.Errorf("%w: Brave Search requires the canonical API endpoint", ErrInvalid)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("%w: Brave Search API key is required", ErrWebSearchUnavailable)
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Brave{client: sameOriginClient(config.Client, endpoint), apiKey: strings.TrimSpace(config.APIKey),
		endpoint: endpoint, now: config.Now}, nil
}

func (*Brave) Identity() (string, string) { return "brave", BraveAdapterVersion }

func (b *Brave) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if err := lookup.Validate(); err != nil {
		return Packet{}, err
	}
	u := *b.endpoint
	q := u.Query()
	q.Set("q", strings.Join(lookup.Terms(), " "))
	q.Set("count", fmt.Sprint(MaxAdapterResults))
	q.Set("safesearch", "moderate")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Packet{}, fmt.Errorf("build Brave Search request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)
	req.Header.Set("User-Agent", "Loomarr/1.0 (https://github.com/loomarr/loomarr)")
	resp, err := b.client.Do(req)
	if err != nil {
		return Packet{}, fmt.Errorf("retrieve Brave Search evidence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Packet{}, fmt.Errorf("retrieve Brave Search evidence: upstream returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, webSearchMaxBody+1))
	if err != nil || len(body) > webSearchMaxBody {
		return Packet{}, fmt.Errorf("retrieve Brave Search evidence: bounded response read failed")
	}
	var decoded struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Packet{}, fmt.Errorf("decode Brave Search evidence: %w", err)
	}
	packet := Packet{Query: lookup.CanonicalTitle(), Adapter: "brave", AdapterVersion: BraveAdapterVersion,
		RetrievedAt: b.now().UTC()}
	for _, result := range decoded.Web.Results {
		if citation, ok := webCitation(len(packet.Citations)+1, result.Title, result.URL, result.Description); ok {
			packet.Citations = append(packet.Citations, citation)
		}
		if len(packet.Citations) == MaxAdapterResults {
			break
		}
	}
	if len(packet.Citations) == 0 {
		return Packet{}, fmt.Errorf("retrieve Brave Search evidence: no attributable results")
	}
	return packet, packet.Validate()
}

func webCitation(id int, title, rawURL, snippet string) (Citation, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return Citation{}, false
	}
	title = strings.TrimSpace(title)
	snippet = strings.TrimSpace(snippet)
	if title == "" || snippet == "" {
		return Citation{}, false
	}
	if len(snippet) > MaxExtractBytes {
		snippet = snippet[:MaxExtractBytes]
	}
	return Citation{ID: id, Title: title, URL: u.String(), Extract: snippet}, true
}
