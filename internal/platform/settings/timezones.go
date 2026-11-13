package settings

import (
	"strings"
	_ "time/tzdata" // every zone on the list loads on a host with no tz database, or an older one
)

//go:generate go run ../../../cmd/settings-timezones

// zoneAreas are the IANA regions a place's zone is filed under. Go's tz data also carries the
// legacy and rule names (`EST5EDT`, `GB-Eire`, `Factory`); a household picks a city, so those stay
// out of the picker, and guide.timezone refuses them: it accepts exactly what the picker offers (see
// ianaZone), so a value validates the same on every host.
var zoneAreas = map[string]bool{
	"Africa": true, "America": true, "Antarctica": true, "Arctic": true, "Asia": true,
	"Atlantic": true, "Australia": true, "Europe": true, "Indian": true, "Pacific": true,
}

// timezonePresets is guide.timezone's choices: the viewer's own device first, then UTC, then every
// city zone in Go's tz data. Labels read the zone as its region and place ("America/New York").
func timezonePresets() []Preset {
	out := []Preset{preset("", "Each viewer's device"), preset("UTC", "UTC")}
	for _, name := range goZoneNames {
		area, _, ok := strings.Cut(name, "/")
		if !ok || !zoneAreas[area] {
			continue
		}
		out = append(out, preset(name, strings.ReplaceAll(name, "_", " ")))
	}
	return out
}
