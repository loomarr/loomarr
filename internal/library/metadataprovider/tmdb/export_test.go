package tmdb

import "time"

// SetClock replaces the provider's clock so a test can place "now" inside or outside the change
// feed's window.
func SetClock(p *Provider, now func() time.Time) { p.now = now }
