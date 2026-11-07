package arrhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/httpx"
)

// Name is the connection Loomarr keeps in each arr's Settings → Connect.
const Name = "Loomarr"

// Username is the basic-auth user an arr sends with each event; the password is the secret.
const Username = "loomarr"

// events are the arr notifications that change files. Each arr offers the ones that apply to it
// (a "supportsOnX" flag); every other notification is switched off.
var events = map[string]bool{
	"onDownload": true, "onUpgrade": true, "onImportComplete": true, "onRename": true,
	"onSeriesDelete": true, "onEpisodeFileDelete": true, "onEpisodeFileDeleteForUpgrade": true,
	"onMovieDelete": true, "onMovieFileDelete": true, "onMovieFileDeleteForUpgrade": true,
}

// ErrNotConfigured is an arr with no address.
var ErrNotConfigured = errors.New("arrhook: not configured")

// Conn is an arr's address and API key, read per call.
type Conn func() (baseURL, apiKey string)

// Client registers Loomarr's webhook with one arr.
type Client struct {
	Conn Conn
	HTTP *http.Client
}

// masked is how an arr returns a stored password.
const masked = "********"

// timeout bounds one call when the caller supplies no client. Saving a connection waits for the
// arr's own test delivery to Loomarr, so it is longer than a read.
const timeout = 30 * time.Second

// Register makes sure the arr has one webhook connection named Name that posts the file events to
// url with the secret, creating or updating it as needed. The arr tests a connection before it
// saves one, so an error here also covers an arr that cannot reach url. changed reports a write.
func (c Client) Register(ctx context.Context, url, secret string) (changed bool, err error) {
	existing, err := c.find(ctx)
	if err != nil {
		return false, err
	}
	conn := existing
	if conn == nil {
		if conn, err = c.template(ctx); err != nil {
			return false, err
		}
	}
	want := desired(conn, url, secret)
	if existing != nil && same(existing, want) {
		return false, nil
	}
	if existing == nil {
		return true, c.send(ctx, http.MethodPost, "/api/v3/notification", want, nil)
	}
	id, ok := existing["id"].(float64)
	if !ok {
		return false, fmt.Errorf("arrhook: the %s connection has no id", Name)
	}
	return true, c.send(ctx, http.MethodPut, "/api/v3/notification/"+strconv.FormatFloat(id, 'f', -1, 64), want, nil)
}

// Test asks the arr to send its test event to Loomarr's registered connection.
func (c Client) Test(ctx context.Context) error {
	existing, err := c.find(ctx)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("arrhook: no %s connection", Name)
	}
	return c.send(ctx, http.MethodPost, "/api/v3/notification/test", existing, nil)
}

// find is the arr's connection named Name, nil when there is none.
func (c Client) find(ctx context.Context) (map[string]any, error) {
	var all []map[string]any
	if err := c.send(ctx, http.MethodGet, "/api/v3/notification", nil, &all); err != nil {
		return nil, err
	}
	for _, n := range all {
		if n["name"] == Name {
			return n, nil
		}
	}
	return nil, nil
}

// template is the arr's blank webhook connection.
func (c Client) template(ctx context.Context) (map[string]any, error) {
	var schema []map[string]any
	if err := c.send(ctx, http.MethodGet, "/api/v3/notification/schema", nil, &schema); err != nil {
		return nil, err
	}
	for _, n := range schema {
		if n["implementation"] == "Webhook" {
			return n, nil
		}
	}
	return nil, errors.New("arrhook: the arr offers no webhook connection")
}

// desired is conn named and pointed at url, with only the file events on.
func desired(conn map[string]any, url, secret string) map[string]any {
	out := make(map[string]any, len(conn))
	for k, v := range conn {
		out[k] = v
	}
	out["name"] = Name
	for k, v := range conn {
		if _, isBool := v.(bool); isBool && strings.HasPrefix(k, "on") {
			out[k] = events[k] && conn["supportsO"+k[1:]] == true
		}
	}
	values := map[string]any{"url": url, "method": 1, "username": Username, "password": secret}
	var fields []any
	raw, _ := conn["fields"].([]any)
	for _, f := range raw {
		field, ok := f.(map[string]any)
		if !ok {
			fields = append(fields, f)
			continue
		}
		copied := make(map[string]any, len(field))
		for k, v := range field {
			copied[k] = v
		}
		if v, ok := values[fmt.Sprint(field["name"])]; ok {
			copied["value"] = v
		}
		fields = append(fields, copied)
	}
	out["fields"] = fields
	return out
}

// same reports whether the arr's connection already matches want. A password the arr masks is
// taken as matching: Loomarr's secret never changes, so only a hand edit could make it differ.
func same(have, want map[string]any) bool {
	for k, v := range want {
		if k == "fields" {
			continue
		}
		if b, ok := v.(bool); ok && strings.HasPrefix(k, "on") && have[k] != b {
			return false
		}
	}
	if have["name"] != want["name"] {
		return false
	}
	haveFields := fieldValues(have)
	for name, v := range fieldValues(want) {
		h := haveFields[name]
		if name == "password" && h == masked {
			continue
		}
		if fmt.Sprint(h) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

func fieldValues(conn map[string]any) map[string]any {
	out := map[string]any{}
	raw, _ := conn["fields"].([]any)
	for _, f := range raw {
		if field, ok := f.(map[string]any); ok {
			switch name := fmt.Sprint(field["name"]); name {
			case "url", "method", "username", "password":
				out[name] = field["value"]
			}
		}
	}
	return out
}

// send makes one call; a body is JSON, and out (when given) decodes the answer.
func (c Client) send(ctx context.Context, method, path string, body, out any) error {
	if c.Conn == nil {
		return ErrNotConfigured
	}
	base, key := c.Conn()
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ErrNotConfigured
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// A copy that refuses every redirect: Go forwards X-Api-Key across hosts, and an arr's API
	// never redirects.
	client := http.Client{Timeout: timeout}
	if c.HTTP != nil {
		client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := httpx.CheckStatus(resp); err != nil {
		return err
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}
