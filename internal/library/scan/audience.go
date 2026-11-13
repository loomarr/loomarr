package scan

import "path"

// Audience shelves: a household's "Kids/" or "Family/", which holds films and shows alike, so a
// folder on one is a film's or a show's by what its own videos say.

// audienceNames are the catch-alls a household sorts by who watches, which hold films and shows
// alike: a folder in one is a film's or a show's by what its videos say.
var audienceNames = map[string]bool{"kids": true, "children": true, "family": true}

// unshelved says whether a folder, relative to the root, has nothing above it but mere catch-alls
// ("Media/Library/Bumbleton/"): nothing says what its videos are but themselves.
func unshelved(rel string) bool {
	for d := path.Dir(rel); d != "."; d = path.Dir(d) {
		if key := folderKey(path.Base(d)); !catchAllNames[key] || audienceNames[key] || contentShelves[key] {
			return false
		}
	}
	return true
}

// contentShelves are the shelves a household keeps videos on that are neither films nor shows:
// each video on one is its own.
var contentShelves = map[string]bool{
	"home videos": true, "home movies": true, "family videos": true, "concerts": true, "music videos": true,
	"sports": true, "workouts": true, "fitness": true, "stand-up": true, "standup": true, "comedy specials": true,
	"youtube": true, "clips": true, "camera": true, "phone": true, "vacation": true, "holidays": true,
}

// onAudienceShelf says whether a folder, relative to the root, is on an audience's shelf: the
// first folder above it that is no mere catch-all ("Media", "Library") is an audience's.
func onAudienceShelf(rel string) bool {
	for d := path.Dir(rel); d != "."; d = path.Dir(d) {
		key := folderKey(path.Base(d))
		if audienceNames[key] {
			return true
		}
		if !catchAllNames[key] {
			return false
		}
	}
	return false
}
