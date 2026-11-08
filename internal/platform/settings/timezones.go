package settings

import "strings"

//go:generate go run timezones_generate.go

// zoneAreas are the IANA regions a place's zone is filed under. Go's tz data also carries the
// legacy and rule names (`EST5EDT`, `GB-Eire`, `Factory`); a household picks a city, so those stay
// out of the picker. Nothing is rejected for it: a pinned or stored legacy name still saves.
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
