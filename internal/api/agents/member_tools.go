package agents

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The member tools: what any signed-in person can ask. Each result is trimmed to the fields a
// model answers from (names, numbers, local times), not the route's full DTO, because every byte
// of a tool result is context the model pays for on every later turn.

// airing is one programme in a tool result.
type airing struct {
	Title string `json:"title"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type nowNextEntry struct {
	Title   string `json:"title"`
	StartMs int64  `json:"startMs"`
	StopMs  int64  `json:"stopMs"`
	Gap     bool   `json:"gap"`
}

func (k clock) airing(e *nowNextEntry) *airing {
	if e == nil || e.Gap || e.Title == "" {
		return nil
	}
	return &airing{Title: e.Title, Start: k.at(e.StartMs), End: k.at(e.StopMs)}
}

type channelsIn struct{}

type channelRow struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

type channelsOut struct {
	Channels []channelRow `json:"channels"`
}

func (t *toolset) listChannels(ctx context.Context, _ *mcp.CallToolRequest, _ channelsIn) (*mcp.CallToolResult, channelsOut, error) {
	list, err := t.channels(ctx)
	if err != nil {
		return nil, channelsOut{}, err
	}
	out := channelsOut{Channels: make([]channelRow, 0, len(list))}
	for _, c := range list {
		out.Channels = append(out.Channels, channelRow{Number: c.Number, Name: c.Name})
	}
	return nil, out, nil
}

type whatsOnIn struct {
	Channel string `json:"channel,omitempty" jsonschema:"a channel number or name; leave empty for every channel"`
	Count   int    `json:"count,omitempty" jsonschema:"with a channel, how many programmes from now on (default 6, at most 24)"`
}

type whatsOnRow struct {
	Number int      `json:"number"`
	Name   string   `json:"name"`
	Now    *airing  `json:"now,omitempty"`
	Next   *airing  `json:"next,omitempty"`
	Later  []airing `json:"later,omitempty"`
}

type whatsOnOut struct {
	TimeZone string       `json:"timeZone"`
	Channels []whatsOnRow `json:"channels"`
}

func (t *toolset) whatsOn(ctx context.Context, _ *mcp.CallToolRequest, in whatsOnIn) (*mcp.CallToolResult, whatsOnOut, error) {
	k := t.clock(ctx)
	out := whatsOnOut{TimeZone: k.zone()}
	if in.Channel != "" {
		c, err := t.resolve(ctx, in.Channel)
		if err != nil {
			return nil, out, err
		}
		var up struct {
			Upcoming []nowNextEntry `json:"upcoming"`
		}
		q := url.Values{"limit": {strconv.Itoa(clampLimit(in.Count, 6, 24))}}
		if err := t.get(ctx, "/v1/channels/"+url.PathEscape(c.ID)+"/upcoming", q, &up); err != nil {
			return nil, out, err
		}
		row := whatsOnRow{Number: c.Number, Name: c.Name}
		for i := range up.Upcoming {
			a := k.airing(&up.Upcoming[i])
			switch {
			case a == nil:
			case i == 0 && up.Upcoming[i].StartMs <= k.now.UnixMilli():
				row.Now = a
			default:
				row.Later = append(row.Later, *a)
			}
		}
		out.Channels = []whatsOnRow{row}
		return nil, out, nil
	}
	list, err := t.channels(ctx)
	if err != nil {
		return nil, out, err
	}
	var nn struct {
		Channels []struct {
			ChannelID string        `json:"channelId"`
			Now       *nowNextEntry `json:"now"`
			Next      *nowNextEntry `json:"next"`
		} `json:"channels"`
	}
	if err := t.get(ctx, "/v1/channels/now-next", nil, &nn); err != nil {
		return nil, out, err
	}
	names := channelNames(list)
	for _, r := range nn.Channels {
		c, ok := names[r.ChannelID]
		if !ok {
			continue
		}
		out.Channels = append(out.Channels, whatsOnRow{Number: c.Number, Name: c.Name, Now: k.airing(r.Now), Next: k.airing(r.Next)})
	}
	return nil, out, nil
}

type tonightIn struct {
	Hours int `json:"hours,omitempty" jsonschema:"how far ahead to look, in hours (default 6, at most 24)"`
	Count int `json:"count,omitempty" jsonschema:"how many highlights (default 6, at most 20)"`
}

type highlight struct {
	Number int    `json:"number"`
	Name   string `json:"channel"`
	Why    string `json:"why"`
	Title  string `json:"title"`
	Start  string `json:"start"`
	Until  string `json:"until,omitempty"`
}

type tonightOut struct {
	TimeZone   string      `json:"timeZone"`
	Highlights []highlight `json:"highlights"`
}

// highlightReasons turns the route's reason codes into words the model can repeat as they are.
var highlightReasons = map[string]string{
	"series_premiere": "series premiere",
	"season_premiere": "season premiere",
	"marathon":        "marathon",
}

func (t *toolset) tonight(ctx context.Context, _ *mcp.CallToolRequest, in tonightIn) (*mcp.CallToolResult, tonightOut, error) {
	k := t.clock(ctx)
	out := tonightOut{TimeZone: k.zone()}
	from := k.now
	to := from.Add(time.Duration(clampLimit(in.Hours, 6, 24)) * time.Hour)
	q := url.Values{
		"from":  {strconv.FormatInt(from.UnixMilli(), 10)},
		"to":    {strconv.FormatInt(to.UnixMilli(), 10)},
		"limit": {strconv.Itoa(clampLimit(in.Count, 6, 20))},
	}
	var res struct {
		Highlights []struct {
			ChannelName   string `json:"channelName"`
			ChannelNumber int    `json:"channelNumber"`
			Reason        string `json:"reason"`
			UntilMs       int64  `json:"untilMs"`
			Airing        struct {
				Title   string `json:"title"`
				StartMs int64  `json:"startMs"`
			} `json:"airing"`
		} `json:"highlights"`
	}
	if err := t.get(ctx, "/v1/guide/highlights", q, &res); err != nil {
		return nil, out, err
	}
	out.Highlights = make([]highlight, 0, len(res.Highlights))
	for _, h := range res.Highlights {
		why := highlightReasons[h.Reason]
		if why == "" {
			why = strings.ReplaceAll(h.Reason, "_", " ")
		}
		out.Highlights = append(out.Highlights, highlight{
			Number: h.ChannelNumber, Name: h.ChannelName, Why: why,
			Title: h.Airing.Title, Start: k.at(h.Airing.StartMs), Until: k.at(h.UntilMs),
		})
	}
	return nil, out, nil
}

type guideIn struct {
	Channel string `json:"channel,omitempty" jsonschema:"a channel number or name; leave empty for every channel"`
	Hours   int    `json:"hours,omitempty" jsonschema:"how many hours from now (default 4, at most 24)"`
}

type guideRow struct {
	Number  int      `json:"number"`
	Name    string   `json:"name"`
	Airings []airing `json:"airings"`
	Omitted int      `json:"omitted,omitempty"`
}

type guideOut struct {
	TimeZone string     `json:"timeZone"`
	Channels []guideRow `json:"channels"`
}

// guideAiringsPerChannel caps one channel's rows. A 24-hour window of half-hour shows is 48 rows a
// channel; across a whole lineup that is thousands, past what a model reads usefully. The count of
// rows left out is reported, so the model can ask for a single channel or a shorter window.
const guideAiringsPerChannel = 24

func (t *toolset) guide(ctx context.Context, _ *mcp.CallToolRequest, in guideIn) (*mcp.CallToolResult, guideOut, error) {
	k := t.clock(ctx)
	out := guideOut{TimeZone: k.zone()}
	var only string
	if in.Channel != "" {
		c, err := t.resolve(ctx, in.Channel)
		if err != nil {
			return nil, out, err
		}
		only = c.ID
	}
	to := k.now.Add(time.Duration(clampLimit(in.Hours, 4, 24)) * time.Hour)
	q := url.Values{
		"from": {strconv.FormatInt(k.now.UnixMilli(), 10)},
		"to":   {strconv.FormatInt(to.UnixMilli(), 10)},
	}
	var res struct {
		Channels []struct {
			ChannelID string `json:"channelId"`
			Number    int    `json:"number"`
			Name      string `json:"name"`
			Airings   []struct {
				Kind    string `json:"kind"`
				Title   string `json:"title"`
				StartMs int64  `json:"startMs"`
				StopMs  int64  `json:"stopMs"`
			} `json:"airings"`
		} `json:"channels"`
	}
	if err := t.get(ctx, "/v1/guide", q, &res); err != nil {
		return nil, out, err
	}
	for _, c := range res.Channels {
		if only != "" && c.ChannelID != only {
			continue
		}
		row := guideRow{Number: c.Number, Name: c.Name, Airings: []airing{}}
		for _, a := range c.Airings {
			// Programmes only: filler and breaks are not what a person means by "what's on".
			if a.Kind != "program" {
				continue
			}
			if len(row.Airings) == guideAiringsPerChannel {
				row.Omitted++
				continue
			}
			row.Airings = append(row.Airings, airing{Title: a.Title, Start: k.at(a.StartMs), End: k.at(a.StopMs)})
		}
		out.Channels = append(out.Channels, row)
	}
	return nil, out, nil
}

type watchLinkIn struct {
	Channel string `json:"channel" jsonschema:"the channel number or name"`
	Quality string `json:"quality,omitempty" jsonschema:"auto (default), 720 or 480"`
}

type watchLinkOut struct {
	Channel string `json:"channel"`
	URL     string `json:"url"`
	Expires string `json:"expires"`
	Note    string `json:"note"`
}

func (t *toolset) watchLink(ctx context.Context, _ *mcp.CallToolRequest, in watchLinkIn) (*mcp.CallToolResult, watchLinkOut, error) {
	c, err := t.resolve(ctx, in.Channel)
	if err != nil {
		return nil, watchLinkOut{}, err
	}
	q := url.Values{}
	if in.Quality != "" {
		q.Set("quality", in.Quality)
	}
	var res struct {
		URL       string `json:"url"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := t.call(ctx, http.MethodPost, "/v1/channels/"+url.PathEscape(c.ID)+"/play-url", q, nil, &res); err != nil {
		return nil, watchLinkOut{}, err
	}
	if res.URL == "" {
		// The route answers 200 with an empty absolute URL when the server's public address is
		// unset; a blank link reported as success would be a link that silently does nothing.
		return nil, watchLinkOut{}, errors.New("this server has no public address set (Settings → server.public_url), so it cannot hand out a stream link")
	}
	k := t.clock(ctx)
	return nil, watchLinkOut{
		Channel: c.label(), URL: res.URL, Expires: k.stamp(res.ExpiresAt) + " " + k.zone(),
		Note: "An HLS stream for a player such as VLC. It stops working when it expires; ask again for a new one.",
	}, nil
}

type myChannelsIn struct{}

type myChannelsOut struct {
	Starred []channelRow `json:"starred"`
	Recent  []channelRow `json:"recent"`
}

func (t *toolset) myChannels(ctx context.Context, _ *mcp.CallToolRequest, _ myChannelsIn) (*mcp.CallToolResult, myChannelsOut, error) {
	list, err := t.channels(ctx)
	if err != nil {
		return nil, myChannelsOut{}, err
	}
	var res struct {
		Favourites []struct {
			ChannelID string `json:"channelId"`
		} `json:"favourites"`
		Recent []struct {
			ChannelID string `json:"channelId"`
		} `json:"recent"`
	}
	if err := t.get(ctx, "/v1/me/channels", nil, &res); err != nil {
		return nil, myChannelsOut{}, err
	}
	names := channelNames(list)
	out := myChannelsOut{Starred: []channelRow{}, Recent: []channelRow{}}
	for _, f := range res.Favourites {
		if c, ok := names[f.ChannelID]; ok {
			out.Starred = append(out.Starred, channelRow{Number: c.Number, Name: c.Name})
		}
	}
	for _, r := range res.Recent {
		if c, ok := names[r.ChannelID]; ok {
			out.Recent = append(out.Recent, channelRow{Number: c.Number, Name: c.Name})
		}
	}
	return nil, out, nil
}

type starIn struct {
	Channel string `json:"channel" jsonschema:"the channel number or name"`
}

type doneOut struct {
	Done string `json:"done"`
}

func (t *toolset) star(ctx context.Context, _ *mcp.CallToolRequest, in starIn) (*mcp.CallToolResult, doneOut, error) {
	c, err := t.resolve(ctx, in.Channel)
	if err != nil {
		return nil, doneOut{}, err
	}
	if err := t.call(ctx, http.MethodPut, "/v1/me/favourites/"+url.PathEscape(c.ID), nil, nil, nil); err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Done: "Starred " + c.label() + "."}, nil
}

func (t *toolset) unstar(ctx context.Context, _ *mcp.CallToolRequest, in starIn) (*mcp.CallToolResult, doneOut, error) {
	c, err := t.resolve(ctx, in.Channel)
	if err != nil {
		return nil, doneOut{}, err
	}
	if err := t.call(ctx, http.MethodDelete, "/v1/me/favourites/"+url.PathEscape(c.ID), nil, nil, nil); err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Done: "Unstarred " + c.label() + "."}, nil
}

type ideasIn struct {
	Count int `json:"count,omitempty" jsonschema:"how many ideas (default 6, at most 20)"`
}

type idea struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Pitch     string `json:"pitch"`
	InLibrary int    `json:"titlesInLibrary"`
	Requested bool   `json:"alreadyRequested,omitempty"`
}

type ideasOut struct {
	Ideas []idea `json:"ideas"`
	More  int    `json:"more,omitempty"`
}

func (t *toolset) channelIdeas(ctx context.Context, _ *mcp.CallToolRequest, in ideasIn) (*mcp.CallToolResult, ideasOut, error) {
	var res struct {
		Ideas []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Pitch     string `json:"pitch"`
			InLibrary int    `json:"inLibrary"`
			Requested bool   `json:"requested"`
		} `json:"ideas"`
	}
	if err := t.get(ctx, "/v1/discovery/ideas", nil, &res); err != nil {
		return nil, ideasOut{}, err
	}
	n := min(clampLimit(in.Count, 6, 20), len(res.Ideas))
	out := ideasOut{Ideas: make([]idea, 0, n), More: len(res.Ideas) - n}
	for _, i := range res.Ideas[:n] {
		out.Ideas = append(out.Ideas, idea{ID: i.ID, Name: i.Name, Pitch: i.Pitch, InLibrary: i.InLibrary, Requested: i.Requested})
	}
	return nil, out, nil
}

type requestIdeaIn struct {
	IdeaID string `json:"ideaId" jsonschema:"the id of a channel idea, from channel_ideas"`
}

func (t *toolset) requestIdea(ctx context.Context, _ *mcp.CallToolRequest, in requestIdeaIn) (*mcp.CallToolResult, doneOut, error) {
	id, err := segment("ideaId", in.IdeaID, "channel_ideas")
	if err != nil {
		return nil, doneOut{}, err
	}
	if err := t.call(ctx, http.MethodPost, "/v1/discovery/ideas/"+id+"/request", nil, nil, nil); err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Done: "Requested. An admin approves it before the channel is made."}, nil
}

type proposeIn struct {
	Description string `json:"description" jsonschema:"the channel in the person's own words, e.g. 'Saturday-morning cartoons from the 90s'"`
	Name        string `json:"name,omitempty" jsonschema:"a name for the channel, if the person gave one"`
}

func (t *toolset) propose(ctx context.Context, _ *mcp.CallToolRequest, in proposeIn) (*mcp.CallToolResult, doneOut, error) {
	if strings.TrimSpace(in.Description) == "" {
		return nil, doneOut{}, errMissing("description", "")
	}
	body := map[string]string{"description": in.Description}
	if in.Name != "" {
		body["channelName"] = in.Name
	}
	if err := t.call(ctx, http.MethodPost, "/v1/proposals", nil, body, nil); err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Done: "Sent. Loomarr drafts the channel from the description, and an admin approves it before it is made."}, nil
}

type watchingIn struct{}

type viewer struct {
	Name    string `json:"name"`
	Device  string `json:"device"`
	Channel string `json:"channel"`
	Since   string `json:"since"`
	You     bool   `json:"you,omitempty"`
}

type watchingOut struct {
	// Scope says whose viewing this is: "household" for an admin, "self" for a member, who sees
	// only their own devices. The model needs it to avoid saying "nobody is watching" to a member.
	Scope    string   `json:"scope"`
	Watching int      `json:"watching"`
	Viewers  []viewer `json:"viewers"`
}

func (t *toolset) whoIsWatching(ctx context.Context, _ *mcp.CallToolRequest, _ watchingIn) (*mcp.CallToolResult, watchingOut, error) {
	list, err := t.channels(ctx)
	if err != nil {
		return nil, watchingOut{}, err
	}
	var res struct {
		Scope    string `json:"scope"`
		Watching int    `json:"watching"`
		Viewers  []struct {
			Name      string `json:"name"`
			Device    string `json:"device"`
			ChannelID string `json:"channelId"`
			Since     string `json:"since"`
			You       bool   `json:"you"`
		} `json:"viewers"`
	}
	if err := t.get(ctx, "/v1/household/viewing", nil, &res); err != nil {
		return nil, watchingOut{}, err
	}
	k := t.clock(ctx)
	names := channelNames(list)
	out := watchingOut{Scope: res.Scope, Watching: res.Watching, Viewers: make([]viewer, 0, len(res.Viewers))}
	for _, v := range res.Viewers {
		name := v.ChannelID
		if c, ok := names[v.ChannelID]; ok {
			name = c.label()
		}
		out.Viewers = append(out.Viewers, viewer{Name: v.Name, Device: v.Device, Channel: name, Since: k.stamp(v.Since), You: v.You})
	}
	return nil, out, nil
}

// addMemberTools registers what every signed-in person may use. memberOnly adds the tools whose
// route refuses admins (requesting a channel idea is a member's ask; an admin makes the channel).
func addMemberTools(s *mcp.Server, t *toolset, memberOnly bool) {
	mcp.AddTool(s, &mcp.Tool{Name: "list_channels", Annotations: readOnly(),
		Description: "List the channels by number and name."}, t.listChannels)
	mcp.AddTool(s, &mcp.Tool{Name: "whats_on", Annotations: readOnly(),
		Description: "What is on now and next on every channel, or now and the next few programmes on one channel."}, t.whatsOn)
	mcp.AddTool(s, &mcp.Tool{Name: "tonight", Annotations: readOnly(),
		Description: "Programmes worth calling out in the next hours: series and season premieres and marathons."}, t.tonight)
	mcp.AddTool(s, &mcp.Tool{Name: "guide", Annotations: readOnly(),
		Description: "The programme guide from now on, for every channel or one."}, t.guide)
	mcp.AddTool(s, &mcp.Tool{Name: "watch_link", Annotations: changes(false),
		Description: "A short-lived stream link for a channel, to open in a video player."}, t.watchLink)
	mcp.AddTool(s, &mcp.Tool{Name: "my_channels", Annotations: readOnly(),
		Description: "The channels this person has starred and watched recently."}, t.myChannels)
	mcp.AddTool(s, &mcp.Tool{Name: "star_channel", Annotations: changes(false),
		Description: "Star a channel, so it is listed first in the apps."}, t.star)
	mcp.AddTool(s, &mcp.Tool{Name: "unstar_channel", Annotations: changes(false),
		Description: "Remove a channel's star."}, t.unstar)
	mcp.AddTool(s, &mcp.Tool{Name: "channel_ideas", Annotations: readOnly(),
		Description: "Ideas for new channels drawn from the library."}, t.channelIdeas)
	if memberOnly {
		mcp.AddTool(s, &mcp.Tool{Name: "request_channel_idea", Annotations: changes(false),
			Description: "Ask for a channel from channel_ideas. An admin approves it before it is made."}, t.requestIdea)
	}
	mcp.AddTool(s, &mcp.Tool{Name: "propose_channel", Annotations: changes(false),
		Description: "Ask for a new channel described in words. Loomarr drafts it and an admin approves it."}, t.propose)
	mcp.AddTool(s, &mcp.Tool{Name: "who_is_watching", Annotations: readOnly(),
		Description: "Who is watching which channel right now. A member sees only their own devices."}, t.whoIsWatching)
}
