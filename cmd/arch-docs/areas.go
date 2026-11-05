package main

import (
	"fmt"
	"sort"
	"strings"
)

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
			"library", "inventory", "catalog", "tmdb", "moviecollections", "episodeevidence",
			"provision", "provisionbackstop", "requester",
			"metadataprovider", "metadataprovider/arr", "metadataprovider/localart",
			"metadataprovider/nfo", "metadataprovider/precedence", "metadataprovider/providertest",
			"metadataprovider/tmdb",
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
	},
	{
		Name:  "channels",
		Title: "Channels",
		Description: "Channels and what they play. A channel's lineup becomes an ordered schedule, and " +
			"the reconcile engine keeps each channel's programming current for whichever backend plays " +
			"it. Operator edits and approved proposals each have one write path. Self-updating channels " +
			"are re-curated here, Home's channel ideas come from here, and so does who is watching now.",
		Packages: []string{
			"schedule", "channels", "channeledit", "binder", "recurate", "ideas", "recommend", "viewing",
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
			"proposal", "proposalworkflow", "proposaloutlook", "suggest", "reference",
			"plannerreference", "quality",
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
	},
	{
		Name:  "filler",
		Title: "Filler",
		Description: "Commercials, bumpers and station IDs. Sources such as YouTube, archive.org and local " +
			"folders are listed and fetched; every clip is checked for safety, structure and defects, " +
			"then admitted or refused; admitted clips are catalogued with their tags and the TV stations " +
			"they name; and the catalog is assembled into the pods that fill a channel's breaks.",
		Packages: []string{
			"filler", "filleradmission", "fillerairworthiness", "fillerdecision", "fillerdefectfloor",
			"fillerenrichment", "fillereval", "fillerresearch", "fillersafety", "fillerscreening",
			"fillerstore", "fillerstructure", "fillerstructureassessment", "fillerstructuremedia",
			"fillerstructurewindow", "fillerstructurewindowopenrouter", "clipcatalog", "clipfetch",
			"taxonomy", "broadcaststation", "sourceprovider", "sourceprovider/archive",
			"sourceprovider/youtube", "installationlocation",
		},
	},
	{
		Name:  "people",
		Title: "People",
		Description: "Who may use Loomarr and how they are reached: sign-in sessions, invitations, " +
			"password recovery, contact addresses and notifications, plus the encryption of the secrets " +
			"Loomarr keeps in its database.",
		Packages: []string{
			"auth", "invitation", "recovery", "contact", "secretprotection", "notifications",
		},
	},
	{
		Name:  "platform",
		Title: "Platform",
		Description: "The ground everything else stands on: the store and its two databases, settings and " +
			"start-up configuration, background jobs and scheduled clean-up, the activity feed and " +
			"diagnostics, metrics, the outbound HTTP client, the event bus, disk-space reservations, the " +
			"language-model and OpenRouter clients, the embedded web app, release tooling, and small " +
			"shared vocabularies such as holidays, language codes and whole-word text matching.",
		Packages: []string{
			"store", "store/storedb", "store/storetest", "store/catalogstore", "store/identitystore",
			"settings", "config", "jobs", "housekeeping", "activity", "diagnostics", "metrics", "httpx",
			"events", "storagegovernor", "logchange", "releasenotes", "releaseverify", "buildinfo", "web",
			"devbootstrap", "llm", "openroutercatalog", "openroutermedia", "holidayvocab", "langcode",
			"textmatch",
		},
	},
	{
		Name:  "edge",
		Title: "Edge",
		Description: "Where the outside world comes in. The api package is Loomarr's HTTP surface, and app " +
			"is the composition root that builds every other area and hands them to it.",
		Packages: []string{"api", "app"},
	},
	{
		Name:  "test",
		Title: "Test support",
		Description: "Code that exists for tests and never ships in the server: the shared test doubles " +
			"and fixtures, the invented demo library, the end-to-end integration journeys, and the " +
			"evaluation harness that runs only under its own build tag.",
		Packages: []string{
			"testkit", "testkit/execfixture", "testkit/httpfixture", "testkit/operationfixture",
			"testkit/playoutstreamfixture", "testkit/postgresimage", "testkit/recordfixture",
			"testkit/channeljourney", "testkit/libraryfixture", "testkit/outlookfixture",
			"testkit/catalogfixture", "testkit/moviecollectionsfixture", "demolibrary", "integration",
			"eval",
		},
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
		Text: "A member asks for a channel through `api`. `proposalworkflow` turns the request into a " +
			"durable Proposal Job and runs `suggest`, which grounds each title through `catalog` " +
			"(the library, `tmdb` and the clip catalog) and `reference`, using a model from `llm`, and " +
			"returns the values in `proposal`. When a person approves it, `binder` plans the channel and " +
			"`channeledit` writes it through `store`; any title the household lacks becomes a `provision` " +
			"record that `provisionbackstop` hands to `requester` for Sonarr or Radarr. `schedule` turns " +
			"the approved lineup and what is actually available into ordered programming, and the " +
			"`channels` reconcile engine keeps that current for the channel's backend: either Loomarr's " +
			"own `playout` engine, whose `playout/packager` stitches one continuous stream, or `tunarr`. " +
			"`livetvpublish` registers the playlist and guide with the media server's Live TV through `library`, and " +
			"when the household changes backend, `backendtransition` prepares the new one fully before " +
			"publishing it to the media server.",
	},
	{
		Title: "From a filler source to a break",
		Text: "An operator registers a source; `sourceprovider` lists what it offers, with " +
			"`sourceprovider/youtube` and `sourceprovider/archive` as the remote kinds. `clipfetch` " +
			"downloads each pulled clip into the drop folder, and `filler` drains that folder into the " +
			"clip folder, while `fillerstore` records the sources, pulls and acquisition runs. Each clip " +
			"is then checked: `fillersafety` and `fillerscreening` for what it says and shows, " +
			"`fillerstructureassessment` for long reels that need splitting, and `fillerdefectfloor` " +
			"for picture and sound defects. `filleradmission` turns that evidence into an admit or " +
			"refuse decision, which `fillerdecision` records. Admitted clips enter the catalog, whose " +
			"values are `clipcatalog`, whose tags are `taxonomy`, and which `store/catalogstore` " +
			"persists. When `schedule` leaves a break in a channel's programming, the `channels` engine " +
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

// renderStartHere renders the orientation section: each area with its description, the
// other areas it imports from today, and its packages; then the two traced flows.
//
// Package rows are a table rather than the "- **`name`**" list the layer listing uses,
// because the coverage gate parses that list as the production-package inventory.
func renderStartHere(table []area, pkgs []Package) string {
	areaOf := map[string]string{}
	for _, a := range table {
		for _, p := range a.Packages {
			areaOf[p] = a.Name
		}
	}
	byName := make(map[string]Package, len(pkgs))
	for _, p := range pkgs {
		byName[p.Name] = p
	}

	var b strings.Builder
	b.WriteString("## Start here\n\n")
	b.WriteString("The packages under `internal/` group into the areas below. Each area says what it " +
		"is for, which other areas it imports from today (counted as package-to-package imports), " +
		"and which packages make it up. The area table lives in `cmd/arch-docs/areas.go`; a new " +
		"package must be added to one area before it merges.\n\n")

	for _, a := range table {
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", a.Title, a.Description)
		fmt.Fprintf(&b, "**Imports from other areas:** %s\n\n", crossAreaImports(a, byName, areaOf, table))
		b.WriteString("| Package | What it is |\n| --- | --- |\n")
		names := append([]string(nil), a.Packages...)
		sort.Strings(names)
		for _, n := range names {
			synopsis := "Tests only."
			if p, ok := byName[n]; ok {
				synopsis = p.Synopsis
				if synopsis == "" {
					synopsis = "—"
				}
			}
			fmt.Fprintf(&b, "| `%s` | %s |\n", n, strings.ReplaceAll(synopsis, "|", `\|`))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Two flows, end to end\n\n")
	for _, f := range flows {
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", f.Title, f.Text)
	}
	return b.String()
}

// crossAreaImports summarises a's imports of packages in other areas, busiest area first.
func crossAreaImports(a area, byName map[string]Package, areaOf map[string]string, table []area) string {
	counts := map[string]int{}
	for _, name := range a.Packages {
		for _, dep := range byName[name].Imports {
			if other, ok := areaOf[dep]; ok && other != a.Name {
				counts[other]++
			}
		}
	}
	if len(counts) == 0 {
		return "none"
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
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf("%s (%d)", n, counts[n])
	}
	return strings.Join(parts, ", ")
}
