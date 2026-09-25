package filler

import "time"

// logRepeatInterval is how long an UNCHANGED per-item warning stays quiet before it is logged
// again. Passes run every couple of minutes, so without this a condition that persists (a
// quarantined artifact, a clip that keeps failing the same way) fills the log with identical lines
// and rotates useful evidence away; a day keeps a still-broken item visible without the flood.
const logRepeatInterval = 24 * time.Hour
