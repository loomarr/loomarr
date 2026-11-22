package agents

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolset is one MCP request's tools: the dispatcher plus the caller every nested call acts as.
// It is built per request (see Handlers.server), so no tool can reach another caller's credential.
type toolset struct {
	d *Dispatcher
	c Caller
}

// get reads path as the caller and decodes the reply into out.
func (t *toolset) get(ctx context.Context, path string, query url.Values, out any) error {
	return t.call(ctx, http.MethodGet, path, query, nil, out)
}

// call sends one request as the caller. out may be nil when the reply's body is not needed.
func (t *toolset) call(ctx context.Context, method, path string, query url.Values, body, out any) error {
	reply, err := t.d.Do(ctx, t.c, method, path, query, body)
	if err != nil {
		return err
	}
	if !reply.OK() {
		return reply.Err()
	}
	if out == nil || len(reply.Body) == 0 {
		return nil
	}
	return reply.Decode(out)
}

// channel is the slice of a channel every tool names it by.
type channel struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Name   string `json:"name"`
	// Status, InAppPlayable and the counts are read by why_not_playing only.
	Status        string `json:"status"`
	InAppPlayable bool   `json:"inAppPlayable"`
	ProgramCount  int    `json:"programCount"`
	PendingCount  int    `json:"pendingCount"`
}

func (c channel) label() string { return fmt.Sprintf("%d %s", c.Number, c.Name) }

// channels lists the channels the caller can see, through GET /v1/channels.
func (t *toolset) channels(ctx context.Context) ([]channel, error) {
	var out struct {
		Channels []channel `json:"channels"`
	}
	if err := t.get(ctx, "/v1/channels", nil, &out); err != nil {
		return nil, err
	}
	return out.Channels, nil
}

// channelNames indexes channels by id, for tools whose route returns ids only.
func channelNames(list []channel) map[string]channel {
	m := make(map[string]channel, len(list))
	for _, c := range list {
		m[c.ID] = c
	}
	return m
}

// resolve finds the one channel the caller means.
func (t *toolset) resolve(ctx context.Context, query string) (channel, error) {
	list, err := t.channels(ctx)
	if err != nil {
		return channel{}, err
	}
	return resolveChannel(list, query)
}

// resolveChannel matches what a person called a channel, trying in order: its id; its number ("7",
// "#7"); its exact name; its number spelled out ("channel 7", "ch 7"); a part of its name. Each
// rule runs only when the stricter ones before it matched nothing, so "Cartoons" picks the channel
// named exactly that even when "Cartoons Classic" also exists, and a channel named "Channel 4"
// still wins over number 4. More than one match is an error that names them, so the model asks
// rather than guesses which channel to change.
func resolveChannel(list []channel, query string) (channel, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return channel{}, fmt.Errorf("say which channel, by number or name")
	}
	lower := strings.ToLower(q)
	byNumber := func(n int) (channel, error) {
		for _, c := range list {
			if c.Number == n {
				return c, nil
			}
		}
		return channel{}, fmt.Errorf("no channel is number %d; %s", n, someChannels(list))
	}
	var byID, exact, partial []channel
	for _, c := range list {
		name := strings.ToLower(c.Name)
		switch {
		case c.ID == q:
			byID = append(byID, c)
		case name == lower:
			exact = append(exact, c)
		case strings.Contains(name, lower):
			partial = append(partial, c)
		}
	}
	if len(byID) == 1 {
		return byID[0], nil
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(q, "#")); err == nil {
		c, err := byNumber(n)
		// No channel has the number, but one may be NAMED it ("24"): the name rules still run.
		if err == nil || (len(exact) == 0 && len(partial) == 0) {
			return c, err
		}
	}
	if c, err := pickOne(q, exact); c != nil || err != nil {
		return deref(c), err
	}
	for _, prefix := range []string{"channel", "ch.", "ch "} {
		if rest, ok := strings.CutPrefix(lower, prefix); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
				return byNumber(n)
			}
		}
	}
	if c, err := pickOne(q, partial); c != nil || err != nil {
		return deref(c), err
	}
	return channel{}, fmt.Errorf("no channel is called %q; %s", q, someChannels(list))
}

// pickOne is the single channel in found, nil when there is none, or an error naming them all.
func pickOne(q string, found []channel) (*channel, error) {
	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		return &found[0], nil
	default:
		return nil, fmt.Errorf("%q matches %d channels (%s); say which", q, len(found), labels(found, 8))
	}
}

func deref(c *channel) channel {
	if c == nil {
		return channel{}
	}
	return *c
}

func someChannels(list []channel) string {
	if len(list) == 0 {
		return "there are no channels yet"
	}
	return "the channels are " + labels(list, 12)
}

func labels(list []channel, limit int) string {
	parts := make([]string, 0, min(len(list), limit)+1)
	for i, c := range list {
		if i == limit {
			parts = append(parts, fmt.Sprintf("and %d more", len(list)-limit))
			break
		}
		parts = append(parts, c.label())
	}
	return strings.Join(parts, ", ")
}

// clock formats times in the household's time zone, the zone the guide and every screen use.
// A model told "19:30" answers "what's on tonight" correctly; one told an epoch does arithmetic.
type clock struct {
	loc *time.Location
	now time.Time
}

// clock reads the household's zone (GET /v1/household). An unknown zone falls back to UTC rather
// than failing the tool: a time in the wrong zone, labelled with its zone, is still usable.
func (t *toolset) clock(ctx context.Context) clock {
	var out struct {
		TimeZone string `json:"timeZone"`
	}
	loc := time.UTC
	if t.get(ctx, "/v1/household", nil, &out) == nil {
		if l, err := time.LoadLocation(out.TimeZone); err == nil {
			loc = l
		}
	}
	return clock{loc: loc, now: time.Now().In(loc)}
}

// at renders an epoch-ms time: "19:30" today, "Tue 19:30" within the week either side, and
// "Tue 3 Nov 19:30" further out, where a weekday alone would name the wrong week.
func (k clock) at(ms int64) string {
	if ms <= 0 {
		return ""
	}
	t := time.UnixMilli(ms).In(k.loc)
	if y, m, d := t.Date(); y == k.now.Year() && m == k.now.Month() && d == k.now.Day() {
		return t.Format("15:04")
	}
	if gap := t.Sub(k.now); gap > -6*24*time.Hour && gap < 6*24*time.Hour {
		return t.Format("Mon 15:04")
	}
	return t.Format("Mon 2 Jan 15:04")
}

// stamp renders an RFC 3339 time the same way, or "" when it does not parse or is the zero time
// (a Go time.Time with omitempty still serialises, as 0001-01-01, for a job that never ran).
func (k clock) stamp(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil || t.Year() <= 1 {
		return ""
	}
	return k.at(t.UnixMilli())
}

// segment is one path segment taken from a model's argument. url.PathEscape leaves "." and ".."
// as they are, and either would rewrite the route the call reaches.
func segment(arg, value, from string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" || v == "." || v == ".." {
		return "", errMissing(arg, from)
	}
	return url.PathEscape(v), nil
}

// untrusted caps free text that someone other than the caller wrote (a member's channel request, an
// event message) before it reaches the model. It is reported, never obeyed — the server's
// instructions say so — and the cap keeps one long text from being most of the context.
func untrusted(s string) string {
	const limit = 300
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "…"
}

// zone names the household's zone, sent once per result so the times above are unambiguous.
func (k clock) zone() string { return k.loc.String() }

// readOnly marks a tool that changes nothing, so a client can run it without asking.
func readOnly() *mcp.ToolAnnotations { return &mcp.ToolAnnotations{ReadOnlyHint: true} }

// changes marks a tool that changes something; destructive says whether it can't simply be undone.
func changes(destructive bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: &destructive}
}

// errMissing is a required argument the model left empty, with where to find a value for it.
func errMissing(arg, from string) error {
	if from == "" {
		return fmt.Errorf("%s is required", arg)
	}
	return fmt.Errorf("%s is required; get one from %s", arg, from)
}

// clampLimit applies a tool's default and ceiling to a model-supplied count.
func clampLimit(n, def, ceiling int) int {
	if n <= 0 {
		return def
	}
	return min(n, ceiling)
}
