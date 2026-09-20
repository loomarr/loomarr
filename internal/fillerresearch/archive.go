package fillerresearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ArchiveAdapterVersion = "archive-search-v1"
	archiveEndpoint       = "https://archive.org/advancedsearch.php"
	archiveTimeout        = 12 * time.Second
	archiveMaxBody        = 64 << 10
)

type ArchiveConfig struct {
	Client               *http.Client
	UserAgent            string
	Endpoint             string
	AllowInsecureTestURL bool
	Now                  func() time.Time
}

// Archive searches Archive.org's public item metadata. It never downloads an item or follows a
// result URL; the returned canonical details links are evidence for the interpreting model and UI.
type Archive struct {
	client    *http.Client
	endpoint  *url.URL
	userAgent string
	now       func() time.Time
}

func NewArchive(config ArchiveConfig) (*Archive, error) {
	raw := strings.TrimSpace(config.Endpoint)
	if raw == "" {
		raw = archiveEndpoint
	}
	endpoint, err := url.Parse(raw)
	loopback := config.AllowInsecureTestURL && endpoint != nil &&
		(endpoint.Hostname() == "127.0.0.1" || endpoint.Hostname() == "localhost" || endpoint.Hostname() == "::1")
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" ||
		(!loopback && (endpoint.Scheme != "https" || endpoint.Hostname() != "archive.org" || endpoint.Path != "/advancedsearch.php")) {
		return nil, fmt.Errorf("%w: Archive.org requires the canonical advanced-search endpoint", ErrInvalid)
	}
	userAgent := strings.TrimSpace(config.UserAgent)
	if userAgent == "" {
		return nil, fmt.Errorf("%w: Archive.org requires an identifying User-Agent", ErrInvalid)
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: archiveTimeout}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Archive{client: sameOriginClient(client, endpoint), endpoint: endpoint, userAgent: userAgent, now: now}, nil
}

func (*Archive) Identity() (string, string) { return "archive-search", ArchiveAdapterVersion }

type archiveResponse struct {
	Response struct {
		Docs []struct {
			Identifier  json.RawMessage `json:"identifier"`
			Title       json.RawMessage `json:"title"`
			Description json.RawMessage `json:"description"`
			Year        json.RawMessage `json:"year"`
			Date        json.RawMessage `json:"date"`
			Creator     json.RawMessage `json:"creator"`
			Subject     json.RawMessage `json:"subject"`
		} `json:"docs"`
	} `json:"response"`
}

func (a *Archive) Retrieve(ctx context.Context, lookup Lookup) (Packet, error) {
	if err := lookup.Validate(); err != nil {
		return Packet{}, err
	}
	u := *a.endpoint
	values := u.Query()
	clauses := make([]string, 0, len(lookup.Terms())*2)
	for _, term := range lookup.Terms() {
		phrase := quotedSearchPhrase(term)
		clauses = append(clauses, "title:"+phrase, "description:"+phrase)
	}
	values.Set("q", "mediatype:movies AND ("+strings.Join(clauses, " OR ")+")")
	for _, field := range []string{"identifier", "title", "description", "year", "date", "creator", "subject"} {
		values.Add("fl[]", field)
	}
	values.Set("rows", strconv.Itoa(MaxAdapterResults))
	values.Set("page", "1")
	values.Set("output", "json")
	u.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Packet{}, fmt.Errorf("build Archive.org request: %w", err)
	}
	req.Header.Set("User-Agent", a.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return Packet{}, fmt.Errorf("retrieve Archive.org evidence: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Packet{}, fmt.Errorf("retrieve Archive.org evidence: upstream returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, archiveMaxBody+1))
	if err != nil {
		return Packet{}, fmt.Errorf("read Archive.org evidence: %w", err)
	}
	if len(body) > archiveMaxBody {
		return Packet{}, fmt.Errorf("retrieve Archive.org evidence: response exceeds %d bytes", archiveMaxBody)
	}
	var decoded archiveResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Packet{}, fmt.Errorf("decode Archive.org evidence: %w", err)
	}
	packet := Packet{Query: lookup.CanonicalTitle(), Adapter: "archive-search",
		AdapterVersion: ArchiveAdapterVersion, RetrievedAt: a.now().UTC()}
	for _, doc := range decoded.Response.Docs {
		identifier := archiveText(doc.Identifier)
		title := archiveText(doc.Title)
		if identifier == "" || strings.Contains(identifier, "/") || strings.ContainsAny(identifier, "\r\n\t") {
			continue
		}
		if title == "" {
			title = identifier
		}
		extract := archiveExtract(doc.Description, doc.Year, doc.Date, doc.Creator, doc.Subject)
		if extract == "" {
			continue
		}
		packet.Citations = append(packet.Citations, Citation{ID: len(packet.Citations) + 1, Title: title,
			URL: "https://archive.org/details/" + url.PathEscape(identifier), Extract: extract})
		if len(packet.Citations) == MaxAdapterResults {
			break
		}
	}
	if len(packet.Citations) == 0 {
		return Packet{}, fmt.Errorf("retrieve Archive.org evidence: no attributable results")
	}
	if err := packet.Validate(); err != nil {
		return Packet{}, err
	}
	return packet, nil
}

func quotedSearchPhrase(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

func archiveText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.Join(strings.Fields(text), " ")
	}
	var texts []string
	if json.Unmarshal(raw, &texts) == nil {
		for i := range texts {
			texts[i] = strings.Join(strings.Fields(texts[i]), " ")
		}
		return strings.Join(texts, "; ")
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

func archiveExtract(description, year, date, creator, subject json.RawMessage) string {
	parts := make([]string, 0, 5)
	for _, field := range []struct {
		label string
		value json.RawMessage
	}{
		{"Description", description}, {"Year", year}, {"Date", date},
		{"Creator", creator}, {"Subject", subject},
	} {
		if value := archiveText(field.value); value != "" {
			parts = append(parts, field.label+": "+value)
		}
	}
	extract := strings.Join(parts, "\n")
	if len(extract) > MaxExtractBytes {
		extract = extract[:MaxExtractBytes]
	}
	return extract
}
