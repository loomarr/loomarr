package channels

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// GrowthStatus reads what a channel grew by (growth.StatusReader).
type GrowthStatus interface {
	Status(ctx context.Context, ch channelstore.Channel) (growth.Status, error)
}

func (s *Handlers) registerGrowth(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "channel-growth", Method: http.MethodGet, Path: "/v1/channels/{id}/growth",
		Summary: "What a channel takes in by itself",
		Description: "The line the channel's own picks draw, in plain words; what it carries against its target and what it " +
			"lacks; the titles that joined by themselves, each undone by taking it off the lineup; and the titles taken back " +
			"off, with the model's reason where the model took one off. Read only: the keep switch is `policy.growth.keep` " +
			"on the channel PATCH, and a title taken off is added back like any other. 501 when growth isn't wired.",
		Tags: []string{"channels"},
	}, httpcore.RoleMember), s.channelGrowth)
}

type growthCountsDTO struct {
	Episodes int `json:"episodes"`
	Shows    int `json:"shows"`
	Films    int `json:"films"`
}

type growthLineDTO struct {
	Words     string   `json:"words" doc:"The line in plain words, e.g. \"series from 1986 to 1995, rated up to TV-PG, comedy and family\""`
	Fixed     bool     `json:"fixed" doc:"Nothing joins: one seed, or nothing airable to draw a line from"`
	YearFrom  int      `json:"yearFrom,omitempty"`
	YearTo    int      `json:"yearTo,omitempty"`
	MaxRating string   `json:"maxRating,omitempty"`
	Genres    []string `json:"genres" maxItems:"32"`
	Odd       []string `json:"odd" maxItems:"200" doc:"Keys of the channel's own titles that sit outside what most share; they keep airing and do not stretch the line"`
}

type growthJoinedDTO struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Batch string `json:"batch" doc:"The night it joined, e.g. growth-2026-11-16"`
}

type growthOffDTO struct {
	Key    string     `json:"key"`
	Name   string     `json:"name"`
	Reason string     `json:"reason" doc:"The model's reason when the model took it off; empty when a person did"`
	At     *time.Time `json:"at,omitempty" doc:"When the model judged it; absent when a person took it off"`
}

type growthOutput struct {
	Body struct {
		Still   string            `json:"still" enum:",kept,by_hand,fixed,not_active" doc:"Why the channel does not grow; empty when it does. kept = switched to keep as it is; by_hand = made by hand; fixed = its line admits nothing; not_active = not live"`
		Line    growthLineDTO     `json:"line"`
		Carried growthCountsDTO   `json:"carried" doc:"What the channel airs now"`
		Target  growthCountsDTO   `json:"target" doc:"Enough for a week without a repeat and no show dominating a day; zero on a fixed line"`
		Short   growthCountsDTO   `json:"short" doc:"What it lacks of the target; zero when it has enough"`
		Waiting int               `json:"waiting" doc:"Unclear fits waiting for the model's verdict before they may join"`
		Joined  []growthJoinedDTO `json:"joined" maxItems:"2000"`
		Off     []growthOffDTO    `json:"off" maxItems:"1000"`
	}
}

func (s *Handlers) channelGrowth(ctx context.Context, in *httpcore.ChannelIDInput) (*growthOutput, error) {
	if s.growth == nil {
		return nil, huma.Error501NotImplemented("Growth isn't available on this install.")
	}
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	}
	if err != nil {
		return nil, err
	}
	st, err := s.growth.Status(ctx, ch)
	if err != nil {
		return nil, err
	}
	out := &growthOutput{}
	b := &out.Body
	b.Still, b.Waiting = st.Still, st.Waiting
	b.Line = growthLineDTO{Words: st.Words, Fixed: st.Line.Fixed, YearFrom: st.Line.YearFrom, YearTo: st.Line.YearTo,
		MaxRating: string(st.Line.MaxRating), Genres: append([]string{}, st.Line.Genres...), Odd: []string{}}
	for _, o := range st.Line.Odd {
		b.Line.Odd = append(b.Line.Odd, string(o.Key))
	}
	b.Carried = growthCountsDTO(st.Carried)
	b.Target = growthCountsDTO(st.Target)
	b.Short = growthCountsDTO(st.Short)
	b.Joined, b.Off = []growthJoinedDTO{}, []growthOffDTO{}
	for _, j := range st.Joined {
		b.Joined = append(b.Joined, growthJoinedDTO{Key: string(j.Key), Name: j.Name, Batch: j.Batch})
	}
	for _, o := range st.Off {
		dto := growthOffDTO{Key: string(o.Key), Name: o.Name, Reason: o.Reason}
		if !o.At.IsZero() {
			at := o.At
			dto.At = &at
		}
		b.Off = append(b.Off, dto)
	}
	return out, nil
}
