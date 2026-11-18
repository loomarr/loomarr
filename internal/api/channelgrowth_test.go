package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

type fakeGrowth struct {
	status growth.Status
	asked  string
}

func (f *fakeGrowth) Status(_ context.Context, ch channelstore.Channel) (growth.Status, error) {
	f.asked = ch.ID
	return f.status, nil
}

func newGrowthServer(t *testing.T, g api.ChannelsOptions) *httptest.Server {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/g.db")
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.SaveChannel(context.Background(), channelstore.Channel{
		Channel: schedule.Channel{ID: "ch-1", Name: "Sitcoms", Number: 7, Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st, Auth: testAuthorizer{}, Log: slog.New(slog.DiscardHandler), Channels: g,
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A member reads what a channel takes in by itself: the line in plain words, how short it is, what
// joined (each undone by taking it off) and what came back off with the model's reason.
func TestChannelGrowth_ShowsTheLineTheShortfallAndWhatJoined(t *testing.T) {
	judged := time.Date(2026, 11, 18, 5, 10, 0, 0, time.UTC)
	g := &fakeGrowth{status: growth.Status{
		Words:   "series from 1986 to 1995, rated up to TV-PG, comedy",
		Line:    growth.Line{YearFrom: 1986, YearTo: 1995, Genres: []string{"comedy"}},
		Carried: growth.Carried{Episodes: 300, Shows: 4},
		Target:  growth.Target{Episodes: 420, Shows: 6},
		Short:   growth.Target{Episodes: 120, Shows: 2},
		Waiting: 3,
		Joined:  []growth.Joined{{Key: "show:a", Name: "A Show", Batch: "growth-2026-11-18"}},
		Off:     []growth.TakenOff{{Key: "show:b", Name: "B Show", Reason: "a drama, not a comedy", At: judged}},
	}}
	srv := newGrowthServer(t, api.ChannelsOptions{Growth: g})

	res := do(t, srv, http.MethodGet, "/v1/channels/ch-1/growth", memberToken, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("growth → %d, want 200", res.StatusCode)
	}
	var body struct {
		Still string `json:"still"`
		Line  struct {
			Words  string   `json:"words"`
			Genres []string `json:"genres"`
			Odd    []string `json:"odd"`
		} `json:"line"`
		Short   struct{ Episodes, Shows, Films int } `json:"short"`
		Waiting int                                  `json:"waiting"`
		Joined  []struct{ Key, Name, Batch string }  `json:"joined"`
		Off     []struct {
			Key, Name, Reason string
			At                *time.Time
		} `json:"off"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if g.asked != "ch-1" {
		t.Errorf("status read for %q, want ch-1", g.asked)
	}
	if body.Still != "" || body.Line.Words != g.status.Words || body.Waiting != 3 {
		t.Errorf("still %q, words %q, waiting %d", body.Still, body.Line.Words, body.Waiting)
	}
	if body.Line.Odd == nil {
		t.Error("odd is null; the wire never carries null for a list")
	}
	if body.Short.Episodes != 120 || body.Short.Shows != 2 {
		t.Errorf("short %+v, want 120 episodes and 2 shows", body.Short)
	}
	if len(body.Joined) != 1 || body.Joined[0].Batch != "growth-2026-11-18" {
		t.Errorf("joined %+v", body.Joined)
	}
	if len(body.Off) != 1 || body.Off[0].Reason == "" || body.Off[0].At == nil || !body.Off[0].At.Equal(judged) {
		t.Errorf("off %+v, want the model's reason and when", body.Off)
	}
}

func TestChannelGrowth_UnknownChannelAndUnwiredInstall(t *testing.T) {
	srv := newGrowthServer(t, api.ChannelsOptions{Growth: &fakeGrowth{}})
	if res := do(t, srv, http.MethodGet, "/v1/channels/nope/growth", memberToken, ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown channel → %d, want 404", res.StatusCode)
	}
	bare := newGrowthServer(t, api.ChannelsOptions{})
	if res := do(t, bare, http.MethodGet, "/v1/channels/ch-1/growth", memberToken, ""); res.StatusCode != http.StatusNotImplemented {
		t.Errorf("no growth wired → %d, want 501", res.StatusCode)
	}
}
