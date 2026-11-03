// Package nfo provides Kodi NFO sidecars as a metadata provider (#2151 decision 2). An NFO beside
// a media file says which title the file is (its typed uniqueid elements) and carries guide text,
// numbering and a content rating. The scraper that wrote it is named by the default uniqueid, and
// that namespace is the record's catalog.
//
// Reading is bounded and refuses anything but plain XML: a file over MaxSize, a non-regular file,
// a DOCTYPE (so no entity, internal or external, is ever defined) or a malformed document fails.
package nfo

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
)

// MaxSize is the largest NFO read. Real ones are a few kilobytes.
const MaxSize = 1 << 20

// Provider reads NFO sidecars from the local filesystem.
type Provider struct{}

// New returns the NFO provider.
func New() *Provider { return &Provider{} }

func (*Provider) ID() mp.ID { return mp.IDNFO }

// Capabilities: an NFO is a sidecar, resolved by the path of the media it sits beside. Its
// catalog varies by file, so each record names its own. Reads are cold disk seeks, paced like the
// scanner's header reads.
func (*Provider) Capabilities() mp.Capabilities {
	return mp.Capabilities{
		Basis: mp.BasisSidecar,
		Kinds: []mp.Kind{mp.KindMovie, mp.KindSeries, mp.KindEpisode},
		Resolves: []mp.Resolvable{
			{Kind: mp.KindMovie, Namespace: mp.NamespaceFile},
			{Kind: mp.KindSeries, Namespace: mp.NamespaceFile},
			{Kind: mp.KindEpisode, Namespace: mp.NamespaceFile},
		},
		Families: []mp.Family{mp.FamilyIdentity, mp.FamilyDescriptive, mp.FamilyCertification},
		Rate:     mp.RateHint{MaxConcurrent: 4},
	}
}

// Fetch reads the NFO for ref. For a movie or episode, ref names the media file and the NFO is
// the file's name with .nfo (a movie also accepts movie.nfo in its folder). For a series, ref
// names the series folder and the NFO is tvshow.nfo inside it. No NFO is ErrNotFound.
func (p *Provider) Fetch(ctx context.Context, ref mp.Ref) (mp.Record, error) {
	if !p.Capabilities().CanResolve(ref) {
		return mp.Record{}, fmt.Errorf("nfo: %s by %s: %w", ref.Kind, ref.ID.Namespace, mp.ErrUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return mp.Record{}, err
	}
	media := filepath.Clean(ref.ID.Value)
	if !filepath.IsAbs(media) {
		return mp.Record{}, fmt.Errorf("nfo: path %q is not absolute: %w", ref.ID.Value, mp.ErrUnsupported)
	}
	for _, candidate := range candidates(ref.Kind, media) {
		data, err := readBounded(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return mp.Record{}, err
		}
		return Parse(ref.Kind, data)
	}
	return mp.Record{}, fmt.Errorf("nfo: no sidecar for %s: %w", ref.Kind, mp.ErrNotFound)
}

func candidates(kind mp.Kind, media string) []string {
	switch kind {
	case mp.KindSeries:
		return []string{filepath.Join(media, "tvshow.nfo")}
	case mp.KindMovie:
		return []string{sidecar(media), filepath.Join(filepath.Dir(media), "movie.nfo")}
	default:
		return []string{sidecar(media)}
	}
}

func sidecar(media string) string {
	return strings.TrimSuffix(media, filepath.Ext(media)) + ".nfo"
}

// readBounded reads a regular file of at most MaxSize bytes. It does not follow a symlink.
func readBounded(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("nfo: %s is not a regular file", filepath.Base(path))
	}
	if info.Size() > MaxSize {
		return nil, fmt.Errorf("nfo: %s is %d bytes, over the %d-byte limit", filepath.Base(path), info.Size(), MaxSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSize {
		return nil, fmt.Errorf("nfo: %s grew past the %d-byte limit", filepath.Base(path), MaxSize)
	}
	return data, nil
}

// rootElement is each kind's NFO root.
var rootElement = map[mp.Kind]string{
	mp.KindMovie:   "movie",
	mp.KindSeries:  "tvshow",
	mp.KindEpisode: "episodedetails",
}

// Parse reads one NFO document for kind. An episode NFO with several episodedetails (a
// multi-episode file) yields the first episode with EpisodeEnd set to the last.
func Parse(kind mp.Kind, data []byte) (mp.Record, error) {
	root, ok := rootElement[kind]
	if !ok {
		return mp.Record{}, fmt.Errorf("nfo: %s: %w", kind, mp.ErrUnsupported)
	}
	if len(data) > MaxSize {
		return mp.Record{}, fmt.Errorf("nfo: document is over the %d-byte limit", MaxSize)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	var docs []document
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if len(docs) > 0 {
				// Kodi allows a scraper URL after the document; whatever follows the
				// last complete element is not ours to read.
				break
			}
			return mp.Record{}, fmt.Errorf("nfo: %w", err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return mp.Record{}, errors.New("nfo: a DOCTYPE or other directive is not allowed")
		case xml.StartElement:
			if t.Name.Local != root {
				if len(docs) > 0 {
					return recordFrom(kind, docs), nil
				}
				return mp.Record{}, fmt.Errorf("nfo: root element %q is not %q", t.Name.Local, root)
			}
			var doc document
			if err := dec.DecodeElement(&doc, &t); err != nil {
				return mp.Record{}, fmt.Errorf("nfo: %w", err)
			}
			docs = append(docs, doc)
			if kind != mp.KindEpisode {
				return recordFrom(kind, docs), nil
			}
		}
	}
	if len(docs) == 0 {
		return mp.Record{}, fmt.Errorf("nfo: no %s element: %w", root, mp.ErrNotFound)
	}
	return recordFrom(kind, docs), nil
}

// document is the part of a Kodi movie, tvshow or episodedetails element Loomarr reads.
type document struct {
	Title         string     `xml:"title"`
	OriginalTitle string     `xml:"originaltitle"`
	SortTitle     string     `xml:"sorttitle"`
	Plot          string     `xml:"plot"`
	MPAA          string     `xml:"mpaa"`
	Premiered     string     `xml:"premiered"`
	Aired         string     `xml:"aired"`
	Year          string     `xml:"year"`
	Season        string     `xml:"season"`
	Episode       string     `xml:"episode"`
	Runtime       string     `xml:"runtime"`
	Status        string     `xml:"status"`
	Genres        []string   `xml:"genre"`
	Studios       []string   `xml:"studio"`
	UniqueIDs     []uniqueID `xml:"uniqueid"`
}

type uniqueID struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr"`
	Value   string `xml:",chardata"`
}

func recordFrom(kind mp.Kind, docs []document) mp.Record {
	doc := docs[0]
	externalIDs, catalog := identities(doc.UniqueIDs)
	rec := mp.Record{
		Source: mp.Source{Provider: mp.IDNFO, Basis: mp.BasisSidecar, Catalog: catalog},
		Kind:   kind,
		IDs:    externalIDs,
		Descriptive: &mp.Descriptive{
			Title:         clean(doc.Title),
			OriginalTitle: clean(doc.OriginalTitle),
			SortTitle:     clean(doc.SortTitle),
			Overview:      clean(doc.Plot),
			PremiereDate:  firstDate(doc.Premiered, doc.Aired),
			Year:          positive(doc.Year),
			Genres:        cleanAll(doc.Genres),
			Network:       firstNonEmpty(cleanAll(doc.Studios)),
			Status:        clean(doc.Status),
			Runtime:       time.Duration(positive(doc.Runtime)) * time.Minute,
		},
	}
	if rec.Descriptive.Year == 0 && len(rec.Descriptive.PremiereDate) == 10 {
		rec.Descriptive.Year, _ = strconv.Atoi(rec.Descriptive.PremiereDate[:4])
	}
	if rating, country, ok := certification(doc.MPAA); ok {
		rec.Certification = &mp.Certification{Rating: rating, Country: country}
	}
	if kind == mp.KindEpisode {
		rec.Numbering = numbering(docs)
	}
	return rec
}

func numbering(docs []document) *mp.Numbering {
	season, okS := whole(docs[0].Season)
	episode, okE := whole(docs[0].Episode)
	if !okS || !okE {
		return nil
	}
	n := &mp.Numbering{Season: season, Episode: episode}
	for _, doc := range docs[1:] {
		s, okS := whole(doc.Season)
		e, okE := whole(doc.Episode)
		if okS && okE && s == season && e > n.Episode && e > n.EpisodeEnd {
			n.EpisodeEnd = e
		}
	}
	return n
}

var (
	namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	digitsPattern    = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)
	imdbPattern      = regexp.MustCompile(`^tt[0-9]{5,12}$`)
	valuePattern     = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)
)

// identities returns the well-formed uniqueids, one per namespace (the first wins), and the
// catalog: the default uniqueid's namespace, or the only one's.
func identities(raw []uniqueID) ([]mp.ExternalID, mp.Namespace) {
	var out []mp.ExternalID
	var catalog mp.Namespace
	seen := map[mp.Namespace]bool{}
	for _, u := range raw {
		ns := mp.Namespace(strings.ToLower(strings.TrimSpace(u.Type)))
		value := strings.TrimSpace(u.Value)
		if !validID(ns, value) || seen[ns] {
			continue
		}
		seen[ns] = true
		out = append(out, mp.ExternalID{Namespace: ns, Value: value})
		if strings.EqualFold(strings.TrimSpace(u.Default), "true") && catalog == "" {
			catalog = ns
		}
	}
	if catalog == "" && len(out) == 1 {
		catalog = out[0].Namespace
	}
	return out, catalog
}

func validID(ns mp.Namespace, value string) bool {
	switch ns {
	case mp.NamespaceTMDB, mp.NamespaceTVDB:
		return digitsPattern.MatchString(value)
	case mp.NamespaceIMDB:
		return imdbPattern.MatchString(value)
	case mp.NamespaceFile:
		return false
	default:
		return namespacePattern.MatchString(string(ns)) && valuePattern.MatchString(value)
	}
}

var countryPrefix = regexp.MustCompile(`^([A-Za-z]{2})\s*:\s*(.+)$`)

// certification reads Kodi's mpaa: "PG-13", "Rated PG-13", or "US:PG-13". A country prefix is
// kept so a rating in another system is never read against the US ladder.
func certification(raw string) (rating, country string, ok bool) {
	s := clean(raw)
	if m := countryPrefix.FindStringSubmatch(s); m != nil {
		country, s = strings.ToUpper(m[1]), strings.TrimSpace(m[2])
	}
	if s == "" {
		return "", "", false
	}
	return s, country, true
}

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

func cleanAll(values []string) []string {
	var out []string
	for _, v := range values {
		if v = clean(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func firstNonEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstDate(candidates ...string) string {
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if len(c) >= 10 {
			if _, err := time.Parse(time.DateOnly, c[:10]); err == nil {
				return c[:10]
			}
		}
	}
	return ""
}

func positive(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// whole reads a non-negative integer; season 0 holds specials.
func whole(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil && n >= 0
}
