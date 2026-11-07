package main

import "sort"

// area is one part of the product a newcomer can hold in their head: a plain-words
// description and the packages under internal/ that make it up.
//
// The table below is the single source of truth for which area a package belongs to, and
// TestArchDocs_AreasCoverEveryPackage keeps it exact. It is temporary: once the packages move
// into one directory per area, the directory is the area and this table goes.
type area struct {
	Name        string
	Title       string
	Description string
	// Packages are full paths below internal/, test-only packages included.
	Packages []string
	// MayImport names each other area this one's production code may import, with the reason
	// in plain words. TestArchDocs_AreaImportsAreDeclared fails on an import of an area missing
	// here, and on an entry no package uses any more, so the list is always the real one.
	MayImport map[string]string
	// ImportsAnyArea is for the two areas whose job is to see everything: the composition root
	// and the test support. Their imports are not checked.
	ImportsAnyArea bool
}

var areas = []area{
	{
		Name:  "library",
		Title: "Library",
		Description: "What the household owns and wants. Talks to the media server (Emby or Jellyfin), " +
			"keeps Loomarr's own record of every film and episode, looks titles up on TMDB, and asks " +
			"Sonarr or Radarr to fetch a title the household does not have yet. The metadata providers " +
			"decide which source's facts about a file win.",
		Packages: []string{
			"library", "library/scan", "inventory", "catalog", "tmdb", "moviecollections", "episodeevidence",
			"provision", "provisionbackstop", "requester",
			"metadataprovider", "metadataprovider/arr", "metadataprovider/localart",
			"metadataprovider/nfo", "metadataprovider/precedence", "metadataprovider/providertest",
			"metadataprovider/tmdb",
		},
		MayImport: map[string]string{
			"platform": "the store, settings and jobs behind the provision backstop, the outbound HTTP " +
				"client and metrics every remote service call goes through, and the audience rating ladder " +
				"the metadata providers fold certifications onto",
			"media": "the scanner reads each media file's header to learn its length, streams and " +
				"embedded ids",
			"channels": "the backstop's episode refresh re-reads each scheduled show into the scheduler's " +
				"episode cache, whose entries are `channels/schedule.ResolvedProgram`; the job moves into channels " +
				"with the Wave 6.4 directory moves",
		},
	},
	{
		Name:  "media",
		Title: "Media",
		Description: "Shared tools for looking at and working on media files: reading what a file's " +
			"header says, measuring keyframes and loudness, running ffmpeg and ffprobe safely, picking " +
			"the GPU encoder, and the one pipeline every image goes through. It also holds the checks " +
			"that judge a clip's picture, sound and level, and whether two adverts are the same one.",
		Packages: []string{
			"media", "mediaheader", "mediameasure", "mediatools", "images", "images/rustgen",
			"encode", "bgexec", "proctree", "loudness", "creative", "defects",
		},
		MayImport: map[string]string{
			"platform": "the diagnostics process manager every ffmpeg run goes through, the image jobs and " +
				"their schedules, the outbound HTTP client for fetched artwork, and start-up configuration",
			"library": "the `inventory` stream, keyframe and break values that header reads and measurements " +
				"fill in",
		},
	},
	{
		Name:  "channels",
		Title: "Channels",
		Description: "Channels and what they play. A channel's lineup becomes an ordered schedule, and " +
			"the reconcile engine keeps each channel's programming current for whichever backend plays " +
			"it. Operator edits and approved proposals each have one write path. Self-updating channels " +
			"are re-curated here, Home's channel ideas come from here, and so does who is watching now.",
		Packages: []string{
			"channels", "channels/schedule", "channels/edit", "channels/approval", "channels/recurate",
			"channels/ideas", "channels/recommend", "channels/viewing",
		},
		MayImport: map[string]string{
			"platform": "the store every channel write goes through, the jobs and settings behind " +
				"reconcile and re-curation, the activity feed, the language model for recommendations, and " +
				"the holiday, language and audience-rating vocabularies",
			"library": "the titles a channel plays: `provision` keys and states, `inventory` break " +
				"analysis, and `catalog` candidates for re-curation",
			"proposals": "binding an approved proposal into a channel, and re-curating a self-updating " +
				"channel through the suggester",
			"filler": "asking the filler engine for the pod that fills a break",
			"playout": "handing reconciled programming to the backend that plays it: Loomarr's own " +
				"`playout` engine or `tunarr`",
		},
	},
	{
		Name:  "proposals",
		Title: "Proposals",
		Description: "Turning \"I want a channel that feels like X\" into a lineup a person can approve. " +
			"The suggester checks every title against the real library and catalog, the workflow tracks " +
			"each request through to the household's first channel, and the outlook explains a pending " +
			"proposal before anyone approves it. The quality and planner-reference packages measure how " +
			"good the suggestions are.",
		Packages: []string{
			"proposals", "proposals/workflow", "proposals/outlook", "proposals/suggest",
			"proposals/webreference", "proposals/plannerreference", "proposals/quality",
		},
		MayImport: map[string]string{
			"platform": "the store, the language model, the holiday vocabulary and whole-word matching the " +
				"suggester checks titles with, the audience rating ladder its ceilings are drawn from, and " +
				"the storage governor's size confidence",
			"library": "grounding each suggested title in the real library, catalog, TMDB and collections, " +
				"and the `provision` records for titles the household lacks",
			"channels": "the `channels/schedule` values a proposal describes, and the reconcile engine the outlook " +
				"runs to preview a proposal before approval",
		},
	},
	{
		Name:  "playout",
		Title: "Playout",
		Description: "Getting a channel onto a TV. Loomarr's own streaming engine stitches a channel into " +
			"one continuous stream and burns in the channel's logo; Tunarr is the other backend that can " +
			"play a channel. The Live TV wiring and the backend transition publish channels to the media " +
			"server, LAN discovery lets TV apps find Loomarr, and the bench measures the pipeline on " +
			"real hardware.",
		Packages: []string{
			"playout", "playout/packager", "playoutbench", "watermark", "tunarr", "tunarr/tunarrtest",
			"livetvpublish", "backendtransition", "landiscovery",
		},
		MayImport: map[string]string{
			"platform": "the diagnostics process manager that runs every encoder, the store behind a " +
				"backend transition, start-up configuration, Tunarr's HTTP client and metrics, and the " +
				"address check that keeps LAN discovery answering only the household's network",
			"channels": "the `channels/schedule` programming a backend plays",
			"library": "the media server's Live TV API that channels are published to, and `provision` keys " +
				"naming what a slot plays",
			"media": "the encoder choice, loudness, media facts and process-tree control the stream is built " +
				"from",
		},
	},
	{
		Name:  "filler",
		Title: "Filler",
		Description: "Commercials, bumpers and station IDs. Sources such as YouTube, archive.org and local " +
			"folders are listed and fetched; every clip is checked for safety, structure and defects, " +
			"then admitted or refused; admitted clips are catalogued with their tags and the TV stations " +
			"they name; and the catalog is assembled into the pods that fill a channel's breaks.",
		Packages: []string{
			"filler", "filler/admission", "filler/defectfloor",
			"filler/enrichment", "filler/eval", "filler/research", "filler/screening",
			"filler/records", "filler/structure", "filler/structure/assessment", "filler/structure/cut",
			"filler/structure/window", "filler/structure/window/openrouter", "filler/clips", "filler/fetch",
			"filler/taxonomy", "filler/station", "filler/source", "filler/source/archive",
			"filler/source/youtube", "filler/location",
		},
		MayImport: map[string]string{
			"platform": "the store that records sources, pulls and the catalog, the language models that " +
				"screen and enrich clips, the storage governor, the diagnostics process manager, and the " +
				"OpenRouter clients",
			"media": "ffmpeg, Whisper and the encoder for fetching and cutting clips, and the picture, sound " +
				"and creative-identity checks every clip passes",
			"channels": "the holiday season and filler selection a channel's `channels/schedule` asks for",
		},
	},
	{
		Name:  "people",
		Title: "People",
		Description: "Who may use Loomarr and how they are reached: sign-in sessions, invitations, " +
			"password recovery, contact addresses and notifications, plus the encryption of the secrets " +
			"Loomarr keeps in its database.",
		Packages: []string{
			"people/auth", "people/invitation", "people/recovery", "people/contact",
			"people/secretprotection", "people/notifications",
		},
		MayImport: map[string]string{
			"platform": "the identity store accounts and sessions live in, the outbound HTTP client " +
				"notifications are sent through, and the configured installation secret",
			"library": "signing a member in with their media-server account",
		},
	},
	{
		Name:  "platform",
		Title: "Platform",
		Description: "The ground everything else stands on: the store and its two databases, settings and " +
			"start-up configuration, background jobs and scheduled clean-up, the activity feed and " +
			"diagnostics, metrics, the outbound HTTP client, the event bus, disk-space reservations, the " +
			"language-model and OpenRouter clients, the embedded web app, release tooling, and small " +
			"shared vocabularies such as holidays, language codes, the audience rating ladder and " +
			"whole-word text matching.",
		Packages: []string{
			"store", "store/storedb", "store/catalogstore", "store/identitystore",
			"settings", "config", "jobs", "housekeeping", "activity", "diagnostics", "metrics", "httpx",
			"events", "storagegovernor", "logchange", "releasenotes", "releaseverify", "buildinfo", "web",
			"devbootstrap", "llm", "openroutercatalog", "openroutermedia", "holidayvocab", "langcode",
			"rating", "textmatch",
		},
		MayImport: map[string]string{
			"library": "the store persists inventory, provision and episode-evidence values, and settings " +
				"parses the media-server connection",
			"channels":  "the store persists the `channels/schedule` channel, lineup and slot values",
			"proposals": "the store persists proposals and suggestion-quality runs",
			"filler":    "the store persists the clip catalog, its taxonomy and broadcast stations",
			"media": "the store persists creative fingerprints and defect verdicts, and metrics count image " +
				"generation",
			"people": "the identity store persists accounts, invitations, recovery, contacts, notifications " +
				"and wrapped keys; housekeeping applies each one's retention; dev bootstrap provisions the " +
				"dev admin",
		},
	},
	{
		Name:  "edge",
		Title: "Edge",
		Description: "Where the outside world comes in. The api package is Loomarr's HTTP surface, and app " +
			"is the composition root that builds every other area and hands them to it.",
		Packages:       []string{"api", "app"},
		ImportsAnyArea: true,
	},
	{
		Name:  "test",
		Title: "Test support",
		Description: "Code that exists for tests and never ships in the server: the shared test doubles " +
			"and fixtures, the store's SQLite and Postgres test databases, the invented demo library, the " +
			"end-to-end integration journeys, and the evaluation harness that runs only under its own " +
			"build tag.",
		Packages: []string{
			"testkit", "testkit/execfixture", "testkit/httpfixture", "testkit/operationfixture",
			"testkit/playoutstreamfixture", "testkit/postgresimage", "testkit/recordfixture",
			"testkit/channeljourney", "testkit/libraryfixture", "testkit/outlookfixture",
			"testkit/catalogfixture", "testkit/moviecollectionsfixture", "store/storetest",
			"demolibrary", "integration", "eval",
		},
		ImportsAnyArea: true,
	},
}

// flow is one end-to-end path through the areas, told in a few sentences. Package names go
// in backticks; TestArchDocs_FlowsNameListedPackages checks each one is still in the table.
type flow struct {
	Title string
	Text  string
}

var flows = []flow{
	{
		Title: "From a request to Live TV",
		Text: "A member asks for a channel through `api`. `proposals/workflow` turns the request into a " +
			"durable Proposal Job and runs `proposals/suggest`, which grounds each title through `catalog` " +
			"(the library, `tmdb` and the clip catalog) and `proposals/webreference`, using a model from " +
			"`llm`, and returns the values in `proposals`. When a person approves it, `channels/approval` plans the " +
			"channel and `channels/edit` writes it through `store`; any title the household lacks becomes a " +
			"`provision` record that `provisionbackstop` hands to `requester` for Sonarr or Radarr. " +
			"`channels/schedule` turns " +
			"the approved lineup and what is actually available into ordered programming, and the " +
			"`channels` reconcile engine keeps that current for the channel's backend: either Loomarr's " +
			"own `playout` engine, whose `playout/packager` stitches one continuous stream, or `tunarr`. " +
			"`livetvpublish` registers the playlist and guide with the media server's Live TV through `library`, and " +
			"when the household changes backend, `backendtransition` prepares the new one fully before " +
			"publishing it to the media server.",
	},
	{
		Title: "From a filler source to a break",
		Text: "An operator registers a source; `filler/source` lists what it offers, with " +
			"`filler/source/youtube` and `filler/source/archive` as the remote kinds. `filler/fetch` " +
			"downloads each pulled clip into the drop folder, and `filler` drains that folder into the " +
			"clip folder, while `filler/records` records the sources, pulls and acquisition runs. Each clip " +
			"is then checked: `filler/screening` for what it says and shows, and whether it is safe to air, " +
			"`filler/structure/assessment` for long reels that need splitting, and `filler/defectfloor` " +
			"for picture and sound defects; `filler/admission` is the certification tooling that weighs " +
			"that evidence into an admit or refuse decision. A clip that passes its checks enters the catalog, whose " +
			"values are `filler/clips`, whose tags are `filler/taxonomy`, and which `store/catalogstore` " +
			"persists. When `channels/schedule` leaves a break in a channel's programming, the `channels` engine " +
			"asks `filler` for a pod that fits it, and `playout` plays the pod as part of the stream.",
	},
}

// areaInventory returns every package the table lists, and any package listed more than once.
func areaInventory(table []area) (listed map[string]bool, duplicates []string) {
	listed = map[string]bool{}
	for _, a := range table {
		for _, p := range a.Packages {
			if listed[p] {
				duplicates = append(duplicates, p)
			}
			listed[p] = true
		}
	}
	sort.Strings(duplicates)
	return listed, duplicates
}

// areaEdge is one import of a package in another area: the importing package, and the
// package it imports.
type areaEdge struct{ From, To string }

// areaImportViolations compares the imports the scan sees against each area's MayImport.
// undeclared holds, per "from → to" area pair, the package edges that no entry allows; unused
// holds the "from → to" entries no package edge needs.
func areaImportViolations(table []area, pkgs []Package) (undeclared map[string][]areaEdge, unused []string) {
	areaOf := map[string]string{}
	for _, a := range table {
		for _, p := range a.Packages {
			areaOf[p] = a.Name
		}
	}
	byArea := map[string]area{}
	for _, a := range table {
		byArea[a.Name] = a
	}
	used := map[string]bool{}
	undeclared = map[string][]areaEdge{}
	for _, p := range pkgs {
		from := byArea[areaOf[p.Name]]
		if from.ImportsAnyArea {
			continue
		}
		for _, dep := range p.Imports {
			to, ok := areaOf[dep]
			if !ok || to == from.Name {
				continue
			}
			pair := from.Name + " → " + to
			if _, allowed := from.MayImport[to]; allowed {
				used[pair] = true
				continue
			}
			undeclared[pair] = append(undeclared[pair], areaEdge{From: p.Name, To: dep})
		}
	}
	for _, a := range table {
		for to := range a.MayImport {
			if pair := a.Name + " → " + to; !used[pair] {
				unused = append(unused, pair)
			}
		}
	}
	sort.Strings(unused)
	return undeclared, unused
}

// areaCount is how many of an area's package imports land in one other area.
type areaCount struct {
	Area  string
	Count int
}

// crossAreaImports counts a's imports of packages in other areas, busiest area first.
func crossAreaImports(a area, byName map[string]Package, areaOf map[string]string, table []area) []areaCount {
	counts := map[string]int{}
	for _, name := range a.Packages {
		for _, dep := range byName[name].Imports {
			if other, ok := areaOf[dep]; ok && other != a.Name {
				counts[other]++
			}
		}
	}
	order := map[string]int{}
	for i, t := range table {
		order[t.Name] = i
	}
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return order[names[i]] < order[names[j]]
	})
	out := make([]areaCount, len(names))
	for i, n := range names {
		out[i] = areaCount{Area: n, Count: counts[n]}
	}
	return out
}
