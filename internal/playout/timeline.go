package playout

import (
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

// "What is airing right now?" — the question the ffconcat loop asks (§9.1).
//
// The mechanism (prior-art §1, Tunarr's): one long-lived `-c copy` ffmpeg reads a two-line
// HTTP ffconcat playlist whose entries BOTH point at a "what's on now" endpoint. Each time
// the concat demuxer opens it, that endpoint answers for the current wall-clock, spawns a
// child encode for that one item, and streams finite MPEG-TS until it ends. The demuxer
// advances, loops, asks again — and gets the next thing.
//
// So the program boundary is the concat demuxer's EOF-and-advance. There is no splicing
// code, and this file is the whole sequencing layer.
//
// Deliberately NOT a new scheduler. `schedule.ComputeDesiredAt` already answers "what does
// this channel air at instant T", honouring curation rules, seasonality, ordering,
// separation and the relaxation ladder — it is what reconcile pushes to Tunarr and what the
// cycle preview shows. Playout asks the SAME function, so what plays cannot drift from what
// the preview promised. Building a second scheduler for playout would be the §10
// shared-assembler mistake in a new place.

// Airing is what a channel should be playing at a given instant.
type Airing struct {
	// Kind mirrors the scheduler's slot kind, so a caller can distinguish "play this
	// program" from "play a filler clip" from "there is nothing".
	Kind schedule.SlotKind
	// LibraryItemID is the media-server item to stream, for a program slot. Empty for
	// filler (which resolves to a clip) and for the nothing-to-play case.
	LibraryItemID string
	// Source is a direct ffmpeg input for an item that is NOT a library title — currently a
	// commercial clip resolved to a local file under FILLER_DIR (§10).
	//
	// Separate from LibraryItemID rather than overloading it: the two are resolved by different
	// code (one via the media server's stream endpoint, one via a path join with a containment
	// check) and conflating them would let a filler path reach the library resolver, or a
	// library id reach the filesystem.
	Source string
	// Title is for logs and the guide, never for identity.
	Title string
	// Offset is how far INTO the item playout should start.
	//
	// This is what makes a mid-program tune-in land at the right place rather than
	// restarting the show for whoever joins. A channel is a wall clock, not a playlist
	// that begins when someone watches.
	Offset time.Duration
	// Remaining is how much of the item is left. The child encode is bounded by it, so the
	// process exits at the item boundary and the concat demuxer advances — that EOF is the
	// sequencing signal.
	Remaining time.Duration
}

// Playable reports whether there is something to encode.
//
// Two ways to be playable, because playout has two kinds of input (§9.1, §10):
//
//   - A library PROGRAM, identified by LibraryItemID, which the resolver turns into a media
//     server stream URL.
//   - A commercial CLIP, a local file under FILLER_DIR, which has no library id at all — the
//     resolver returns its path directly.
//
// Source is what distinguishes them: it is set for a resolved filler clip and empty for a
// library program (whose input is derived from LibraryItemID instead). An earlier version
// required LibraryItemID unconditionally, which made every resolved commercial fall through to
// the offline card — the ad was picked correctly and then silently never played.
func (a Airing) Playable() bool {
	if a.Kind != schedule.SlotProgram {
		return false
	}
	return a.LibraryItemID != "" || a.Source != ""
}

// AiringAt walks a computed lineup against the wall clock and returns what is on.
//
// The cycle repeats: a channel with 90 minutes of programming plays it again, which is what
// makes a channel continuous without an infinitely long lineup. `epoch` anchors the cycle so
// the answer is STABLE — two callers asking at the same instant get the same item at the same
// offset, which is required for the shared-encoder model (one encode, N viewers) to be
// coherent.
//
// Slots with unknown duration are skipped rather than guessed at: a pending acquisition has
// DurationMs 0, and treating that as instantaneous would make the cycle drift, while treating
// it as some default would air silence for a made-up length.
func AiringAt(slots []schedule.Slot, epoch, now time.Time) Airing {
	total := cycleDuration(slots)
	if total <= 0 {
		// Nothing with a known duration — the honest answer is "nothing", which the caller
		// renders as the offline card rather than as dead air.
		return Airing{Kind: schedule.SlotFlex}
	}

	// Where in the cycle are we? Modulo keeps this correct for any elapsed time, including
	// a channel that has been running for weeks, and handles a clock that moved backwards
	// (NTP, DST) by wrapping rather than going negative.
	elapsed := now.Sub(epoch)
	if elapsed < 0 {
		elapsed = 0
	}
	into := elapsed % total

	for _, s := range slots {
		d := slotDuration(s)
		if d <= 0 {
			continue // unknown duration — not airable, see above
		}
		if into < d {
			return Airing{
				Kind:          s.Kind,
				LibraryItemID: s.LibraryItemID,
				Title:         s.Title,
				Offset:        into,
				Remaining:     d - into,
			}
		}
		into -= d
	}
	// Unreachable while total > 0 and the loop uses the same durations, but returning flex
	// beats an index panic if those ever diverge.
	return Airing{Kind: schedule.SlotFlex}
}

// cycleDuration is the summed length of everything airable in the lineup.
func cycleDuration(slots []schedule.Slot) time.Duration {
	var total time.Duration
	for _, s := range slots {
		total += slotDuration(s)
	}
	return total
}

// fillerSlotDuration is how long a break gap lasts when the scheduler did not say.
//
// The scheduler emits break gaps as elastic flex for Tunarr to fill from a filler-list —
// Tunarr decides the length. Internal playout has no such negotiator, so a gap with no
// duration needs one, and it must be the SAME every time or the cycle length changes between
// calls and two viewers of one channel see different programs.
const fillerSlotDuration = 30 * time.Second

// slotDuration returns how long a slot occupies the timeline.
func slotDuration(s schedule.Slot) time.Duration {
	if s.DurationMs > 0 {
		return time.Duration(s.DurationMs) * time.Millisecond
	}
	// A filler/flex gap with no stated duration gets the fixed fallback; anything else
	// (notably a pending acquisition) has genuinely unknown length and is not airable.
	if s.Kind == schedule.SlotFiller || s.Kind == schedule.SlotFlex {
		return fillerSlotDuration
	}
	return 0
}

// Broadcast is one programme on the timeline, with the wall-clock times it occupies.
//
// Distinct from Airing, which answers "what is on RIGHT NOW" for the encoder. A guide needs the
// opposite shape: a sequence with absolute start/stop times, spanning hours. Same arithmetic
// underneath, so the guide cannot advertise something different from what plays.
type Broadcast struct {
	Kind          schedule.SlotKind
	Title         string
	LibraryItemID string
	Season        int
	Episode       int
	// Start and Stop are absolute wall-clock. Stop is exclusive.
	Start, Stop time.Time
}

// Duration is how long this programme occupies the schedule.
func (b Broadcast) Duration() time.Duration { return b.Stop.Sub(b.Start) }

// BroadcastsBetween walks the cycle and returns every programme overlapping [from, to).
//
// The guide's counterpart to AiringAt, and deliberately built on the SAME cycle arithmetic: a
// guide that computed its own timeline would eventually disagree with the encoder, and the
// symptom — "it says Heat but Predator is playing" — is the kind of bug nobody can reproduce on
// demand.
//
// Programmes are CLIPPED to the window, so a film already in progress at `from` reports its real
// start time rather than being truncated or dropped. That matters for an EPG: a media server
// draws the currently-airing programme from its actual start, and a clipped start renders as a
// programme that appears to begin the moment you opened the guide.
//
// Filler and flex are INCLUDED here — the caller decides whether to advertise them. §10 and
// decision #12 say the XMLTV guide must not, but the same walk serves Loomarr's own time-grid
// (V13b), which shows breaks explicitly. Filtering here would make this function useless to one
// of its two callers.
func BroadcastsBetween(slots []schedule.Slot, epoch, from, to time.Time) []Broadcast {
	total := cycleDuration(slots)
	if total <= 0 || !to.After(from) {
		return nil
	}

	// Rewind to the start of the programme covering `from`, so a film already in progress
	// reports its true start. Without this the first entry would begin at `from`.
	elapsed := from.Sub(epoch)
	if elapsed < 0 {
		elapsed = 0
	}
	into := elapsed % total
	cursor := from.Add(-into) // wall-clock time at which the current cycle pass began

	// Advance to the slot containing `from`, keeping cursor aligned to slot boundaries.
	idx := 0
	for _, s := range slots {
		d := slotDuration(s)
		if d <= 0 {
			idx++
			continue
		}
		if into < d {
			break
		}
		into -= d
		cursor = cursor.Add(d)
		idx++
	}

	var out []Broadcast
	// A hard cap on iterations, not on output: a lineup of very short items over a long window
	// is legitimate, but an unbounded loop over a corrupt cycle would hang the request. 10k
	// programmes is far past any real guide (Tunarr's own 14-hour output is 98).
	const maxIterations = 10000
	for i := 0; i < maxIterations && cursor.Before(to); i++ {
		if idx >= len(slots) {
			idx = 0 // the cycle repeats — that is what makes a channel continuous
		}
		s := slots[idx]
		d := slotDuration(s)
		if d <= 0 {
			idx++
			continue // unairable (a pending acquisition has no known duration)
		}
		stop := cursor.Add(d)
		if stop.After(from) {
			out = append(out, Broadcast{
				Kind: s.Kind, Title: s.Title, LibraryItemID: s.LibraryItemID,
				Season: s.Season, Episode: s.Episode,
				Start: cursor, Stop: stop,
			})
		}
		cursor = stop
		idx++
	}
	return out
}
