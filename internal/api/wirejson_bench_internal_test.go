package api

import (
	"fmt"
	"io"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// guideBenchBody is the Guide's response body at fleet scale: 100 channels × 200 airings, half
// the airings with no genres (nil, omitempty) and every tenth a filler block with a pod. It
// has nothing the wire encoder must change, so the encoder's whole cost is the walk.
func guideBenchBody() any {
	var out guideOutput
	out.Body.FromMs, out.Body.ToMs = 1_760_000_000_000, 1_760_014_400_000
	out.Body.Channels = make([]GuideChannelTimeline, 100)
	for c := range out.Body.Channels {
		airings := make([]GuideAiring, 200)
		for a := range airings {
			start := out.Body.FromMs + int64(a)*1_800_000
			airings[a] = GuideAiring{
				Kind: "program", ScheduleBlockID: fmt.Sprintf("blk-%d-%d", c, a),
				Title: "The Episode", Series: "The Show", Season: 2, Episode: a,
				StartMs: start, StopMs: start + 1_800_000, Description: "A plot summary of moderate length.",
				Year: 1994, Rating: "TV-PG", ItemID: "item", RuntimeMs: 1_320_000, Provenance: "in library",
			}
			if a%2 == 0 {
				airings[a].Genres = []string{"Comedy", "Family"}
			}
			if a%10 == 0 {
				airings[a].Kind = "filler"
				airings[a].Pod = &PodPoolDTO{MatchLevel: "exact", TotalMs: 60_000, Entries: []PodEntryDTO{
					{Hash: "h1", Name: "Cereal", Kind: "commercial", DurationMs: 30_000},
					{Hash: "h2", Name: "Cars", Kind: "commercial", DurationMs: 30_000},
				}}
			}
		}
		out.Body.Channels[c] = GuideChannelTimeline{
			ChannelID: fmt.Sprintf("ch-%d", c), Name: "Channel", Number: c + 1, Status: "live", Airings: airings,
		}
	}
	return out.Body
}

// The baseline: Huma's default encoder, which writes a nil slice as null.
func BenchmarkWireJSON_GuideDefault(b *testing.B) {
	benchmarkGuide(b, huma.DefaultJSONFormat)
}

// The served encoder. Its overhead over the baseline is the cost of the no-null guarantee on
// the API's largest body; keep it within 10%.
func BenchmarkWireJSON_Guide(b *testing.B) {
	benchmarkGuide(b, wireJSONFormat)
}

func benchmarkGuide(b *testing.B, format huma.Format) {
	body := guideBenchBody()
	b.ReportAllocs()
	for b.Loop() {
		if err := format.Marshal(io.Discard, body); err != nil {
			b.Fatal(err)
		}
	}
}
