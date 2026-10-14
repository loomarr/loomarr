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
	WikidataAdapterVersion = "wikidata-v1"
	wikidataEndpoint       = "https://www.wikidata.org/w/api.php"
	structuredMaxBody      = 64 << 10
)

type WikidataConfig struct {
	Client               *http.Client
	UserAgent            string
	Endpoint             string
	AllowInsecureTestURL bool
	Now                  func() time.Time
}

// Wikidata searches the official entity API and cites canonical entity pages. It never follows a
// result URL or lets provider/model output choose a request target.
type Wikidata struct {
	client    *http.Client
	endpoint  *url.URL
	userAgent string
	now       func() time.Time
}

func (*Wikidata) Identity() (string, string) { return "wikidata", WikidataAdapterVersion }

func NewWikidata(config WikidataConfig) (*Wikidata, error) {
	raw := strings.TrimSpace(config.Endpoint)
	if raw == "" {
		raw = wikidataEndpoint
	}
	endpoint, err := url.Parse(raw)
	loopback := config.AllowInsecureTestURL && endpoint != nil &&
		(endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "localhost" || endpoint.Hostname() == "::1")
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" ||
		(!loopback && (endpoint.Scheme != "https" || endpoint.Hostname() != "www.wikidata.org" || endpoint.Path != "/w/api.php")) {
		return nil, fmt.Errorf("%w: Wikidata requires the canonical API endpoint", ErrInvalid)
	}
	if strings.TrimSpace(config.UserAgent) == "" {
		return nil, fmt.Errorf("%w: Wikidata requires an identifying User-Agent", ErrInvalid)
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: 12 * time.Second}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Wikidata{client: sameOriginClient(config.Client, endpoint), endpoint: endpoint,
		userAgent: strings.TrimSpace(config.UserAgent), now: config.Now}, nil
}

func (w *Wikidata) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if err := lookup.Validate(); err != nil {
		return Packet{}, err
	}
	u := *w.endpoint
	values := u.Query()
	values.Set("action", "wbsearchentities")
	values.Set("search", lookup.Subject())
	values.Set("language", "en")
	values.Set("uselang", "en")
	values.Set("type", "item")
	values.Set("limit", fmt.Sprint(MaxAdapterResults))
	values.Set("format", "json")
	values.Set("formatversion", "2")
	u.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Packet{}, fmt.Errorf("build Wikidata request: %w", err)
	}
	req.Header.Set("User-Agent", w.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return Packet{}, fmt.Errorf("retrieve Wikidata evidence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Packet{}, fmt.Errorf("retrieve Wikidata evidence: upstream returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, structuredMaxBody+1))
	if err != nil || len(body) > structuredMaxBody {
		return Packet{}, fmt.Errorf("retrieve Wikidata evidence: bounded response read failed")
	}
	var decoded struct {
		Search []struct {
			ID          string `json:"id"`
			Label       string `json:"label"`
			Description string `json:"description"`
		} `json:"search"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Packet{}, fmt.Errorf("decode Wikidata evidence: %w", err)
	}
	packet := Packet{Query: lookup.CanonicalTitle(), Adapter: "wikidata", AdapterVersion: WikidataAdapterVersion,
		RetrievedAt: w.now().UTC()}
	for _, result := range decoded.Search {
		id, label := strings.TrimSpace(result.ID), strings.TrimSpace(result.Label)
		if !validWikidataItemID(id) || label == "" {
			continue
		}
		extract := label
		if description := strings.TrimSpace(result.Description); description != "" {
			extract += ": " + description
		}
		packet.Citations = append(packet.Citations, Citation{ID: len(packet.Citations) + 1, Title: label,
			URL: "https://www.wikidata.org/wiki/" + id, Extract: extract})
		if len(packet.Citations) == MaxAdapterResults {
			break
		}
	}
	if len(packet.Citations) == 0 {
		return Packet{}, fmt.Errorf("retrieve Wikidata evidence: no attributable results")
	}
	return packet, packet.Validate()
}

func validWikidataItemID(id string) bool {
	if len(id) < 2 || id[0] != 'Q' || id[1] == '0' {
		return false
	}
	for _, digit := range id[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
