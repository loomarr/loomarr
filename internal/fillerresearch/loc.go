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
	LOCAdapterVersion = "loc-v1"
	locEndpoint       = "https://www.loc.gov/film-and-videos/"
)

type LOCConfig struct {
	Client               *http.Client
	UserAgent            string
	Endpoint             string
	AllowInsecureTestURL bool
	Now                  func() time.Time
}

// LOC searches the official moving-image catalog and cites item records only. It does not fetch
// item pages or media, and catalog metadata never becomes rights or admission authority.
type LOC struct {
	client    *http.Client
	endpoint  *url.URL
	userAgent string
	now       func() time.Time
}

func (*LOC) Identity() (string, string) { return "loc", LOCAdapterVersion }

func NewLOC(config LOCConfig) (*LOC, error) {
	raw := strings.TrimSpace(config.Endpoint)
	if raw == "" {
		raw = locEndpoint
	}
	endpoint, err := url.Parse(raw)
	loopback := config.AllowInsecureTestURL && endpoint != nil &&
		(endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "localhost" || endpoint.Hostname() == "::1")
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" ||
		(!loopback && (endpoint.Scheme != "https" || endpoint.Hostname() != "www.loc.gov" || endpoint.Path != "/film-and-videos/")) {
		return nil, fmt.Errorf("%w: Library of Congress requires the canonical moving-image API endpoint", ErrInvalid)
	}
	if strings.TrimSpace(config.UserAgent) == "" {
		return nil, fmt.Errorf("%w: Library of Congress requires an identifying User-Agent", ErrInvalid)
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: 12 * time.Second}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &LOC{client: sameOriginClient(config.Client, endpoint), endpoint: endpoint,
		userAgent: strings.TrimSpace(config.UserAgent), now: config.Now}, nil
}

func (l *LOC) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if err := lookup.Validate(); err != nil {
		return Packet{}, err
	}
	u := *l.endpoint
	values := u.Query()
	values.Set("fo", "json")
	values.Set("at", "results")
	values.Set("c", fmt.Sprint(MaxAdapterResults))
	values.Set("sp", "1")
	values.Set("q", lookup.Subject())
	u.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Packet{}, fmt.Errorf("build Library of Congress request: %w", err)
	}
	req.Header.Set("User-Agent", l.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return Packet{}, fmt.Errorf("retrieve Library of Congress evidence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Packet{}, fmt.Errorf("retrieve Library of Congress evidence: upstream returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, structuredMaxBody+1))
	if err != nil || len(body) > structuredMaxBody {
		return Packet{}, fmt.Errorf("retrieve Library of Congress evidence: bounded response read failed")
	}
	var decoded struct {
		Results []struct {
			ID          string          `json:"id"`
			Title       string          `json:"title"`
			Date        string          `json:"date"`
			Description json.RawMessage `json:"description"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Packet{}, fmt.Errorf("decode Library of Congress evidence: %w", err)
	}
	packet := Packet{Query: lookup.CanonicalTitle(), Adapter: "loc", AdapterVersion: LOCAdapterVersion,
		RetrievedAt: l.now().UTC()}
	for _, result := range decoded.Results {
		itemURL, ok := canonicalLOCItemURL(result.ID)
		title := strings.TrimSpace(result.Title)
		if !ok || title == "" {
			continue
		}
		parts := []string{title}
		if date := strings.TrimSpace(result.Date); date != "" {
			parts = append(parts, date)
		}
		parts = append(parts, boundedJSONStrings(result.Description)...)
		extract := strings.Join(parts, " · ")
		if len(extract) > MaxExtractBytes {
			extract = extract[:MaxExtractBytes]
		}
		packet.Citations = append(packet.Citations, Citation{ID: len(packet.Citations) + 1,
			Title: title, URL: itemURL, Extract: extract})
		if len(packet.Citations) == MaxAdapterResults {
			break
		}
	}
	if len(packet.Citations) == 0 {
		return Packet{}, fmt.Errorf("retrieve Library of Congress evidence: no attributable results")
	}
	return packet, packet.Validate()
}

func canonicalLOCItemURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User != nil || !strings.EqualFold(u.Hostname(), "www.loc.gov") {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 2 || parts[0] != "item" || parts[1] == "" {
		return "", false
	}
	return "https://www.loc.gov/item/" + parts[1] + "/", true
}

func boundedJSONStrings(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		out := make([]string, 0, min(2, len(many)))
		for _, value := range many {
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
				if len(out) == 2 {
					break
				}
			}
		}
		return out
	}
	var one string
	if json.Unmarshal(raw, &one) == nil && strings.TrimSpace(one) != "" {
		return []string{strings.TrimSpace(one)}
	}
	return nil
}
