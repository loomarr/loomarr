package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// Provenance answers "why is this here, and is it real yet?" in one line, for the guide's
// hover card (§12, v2 mock: "in library · 76 episodes", "acquiring · Sonarr grabbed it",
// "requested · 48h deadline").
//
// Assembled on the SERVER because it is one sentence drawn from several places —
// provisioning state, the arr's download progress, a deadline measured against now. A client
// reassembling it would be re-implementing a decision, and every client would drift.
//
// The phrasing follows the states' real meanings (§4), not a generic "loading":
//
//	available   → it is here and will play
//	downloading → a release was grabbed and is coming in, with progress when the arr reports it
//	requested   → accepted downstream, waiting on a release, with time left before we give up
//	wanted      → submitted, not yet accepted — the state that means "nothing is happening yet"
//	unavailable → we gave up, and the slot plays filler instead
func provenanceOf(rec provision.Record, now time.Time) string {
	switch rec.State {
	case provision.Available:
		return "in library"

	case provision.Downloading:
		// Progress is the honest thing to show once a release is actually moving. The arr's
		// own ETA string is preferred over computing one — it knows the queue, we do not.
		parts := []string{"acquiring"}
		if rec.Progress > 0 {
			parts = append(parts, fmt.Sprintf("%.0f%%", rec.Progress*100))
		}
		if eta := strings.TrimSpace(rec.ETAText); eta != "" {
			parts = append(parts, eta+" left")
		}
		if len(parts) == 1 && rec.DownloadStatus != "" {
			// No progress yet, but the queue says something — "stalled" and "warning" are
			// exactly the cases a user wants surfaced rather than a silent spinner.
			parts = append(parts, rec.DownloadStatus)
		}
		return strings.Join(parts, " · ")

	case provision.Requested:
		if d := timeLeft(rec.Deadline, now); d != "" {
			return "requested · " + d
		}
		return "requested"

	case provision.Wanted:
		return "requested" // submitted but not yet accepted downstream; same story to a viewer

	case provision.Unavailable:
		// Says WHY the slot is filler rather than leaving a mystery gap. LastError is an
		// internal string, so it is deliberately not surfaced here.
		return "unavailable — playing filler instead"
	}
	return ""
}

// timeLeft renders how long until a deadline, coarsely.
//
// Coarse ON PURPOSE: a 48-hour acquisition deadline counted to the minute implies a precision
// the process does not have, and a guide is glanced at, not studied. A passed deadline says so
// rather than rendering a negative.
func timeLeft(deadline, now time.Time) string {
	if deadline.IsZero() {
		return ""
	}
	d := deadline.Sub(now)
	switch {
	case d <= 0:
		return "deadline passed"
	case d < time.Hour:
		return fmt.Sprintf("%dm left", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh left", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd left", int(d.Hours()/24))
	}
}
