package main

import (
	"sort"
	"time"
)

// overlapAllowed is how far a programme may run into the next row's start and both still have
// aired: a runtime a little longer than its slot. A re-dealt row starts minutes after the row it
// replaced, so it overlaps by nearly the whole programme.
const overlapAllowed = 5 * time.Minute

// aired keeps one row per slot of the scheduled ledger in [from, to). The ledger records a row at
// a programme's start for every deal a reconcile commits (#4140 step 1), so a window re-dealt on
// air leaves several rows for one slot, and recorded_at equals aired_at, so the ledger cannot say
// which deal ran. aired lays each channel's rows end to end in start order: a row that starts
// before the last kept programme has ended (less overlapAllowed) is a replaced deal. One programme
// per slot is what the repeat measures need; which of the overlapping deals it was is unknown.
//
// redealt maps each channel to the local dates on which at least one of its rows was replaced.
func aired(scheduled []ProgramAiring, items map[string]Item, from, to time.Time, loc *time.Location) (kept []ProgramAiring, redealt map[string]map[string]bool) {
	byChannel := map[string][]ProgramAiring{}
	for _, a := range scheduled {
		if inSpan(a.AiredAt, from, to) {
			byChannel[a.ChannelID] = append(byChannel[a.ChannelID], a)
		}
	}
	redealt = map[string]map[string]bool{}
	for channel, rows := range byChannel {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].AiredAt.Before(rows[j].AiredAt) })
		var ends time.Time
		for _, a := range rows {
			if a.AiredAt.Before(ends.Add(-overlapAllowed)) {
				if redealt[channel] == nil {
					redealt[channel] = map[string]bool{}
				}
				redealt[channel][day(a.AiredAt, loc)] = true
				continue
			}
			kept = append(kept, a)
			ends = a.AiredAt.Add(time.Duration(items[a.ItemID].RuntimeMs) * time.Millisecond)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].AiredAt.Before(kept[j].AiredAt) })
	return kept, redealt
}
