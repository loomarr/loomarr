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
	var refused error
	for _, candidate := range candidates(ref.Kind, media) {
		data, err := readBounded(candidate)
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue
		case errors.Is(err, ErrNotRegularFile), errors.Is(err, ErrTooLarge):
			// A refused sidecar is skipped for the next candidate, and reported if none
			// is read.
			refused = errors.Join(refused, err)
			continue
		case err != nil:
			return mp.Record{}, err
		}
		return Parse(ref.Kind, data)
	}
	if refused != nil {
		return mp.Record{}, refused
	}
	return mp.Record{}, fmt.Errorf("nfo: no sidecar for %s: %w", ref.Kind, mp.ErrNotFound)
}

var (
	// ErrTooLarge is a sidecar over MaxSize.
	ErrTooLarge = errors.New("nfo: sidecar over the size limit")
	// ErrNotRegularFile is a sidecar that is a symlink, a device or a directory, or that was
	// swapped between being checked and opened.
	ErrNotRegularFile = errors.New("nfo: sidecar is not a regular file")
	// ErrDirective is a document with a DOCTYPE or other directive, which could define entities.
	ErrDirective = errors.New("nfo: directives are not allowed")
	// ErrMalformed is a document that is not well-formed XML of the expected kind.
	ErrMalformed = errors.New("nfo: malformed document")
)

func candidates(kind mp.Kind, media string) []string {
	switch kind {
	case mp.KindSeries:
		return []string{filepath.Join(media, "tvshow.nfo")}
	case mp.KindMovie:
		if soleVideo(media) {
			return []string{sidecar(media), filepath.Join(filepath.Dir(media), "movie.nfo")}
		}
		return []string{sidecar(media)}
	default:
		return []string{sidecar(media)}
	}
}

// videoExtensions are the containers a movie folder's video file may use.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".ts": true, ".m2ts": true,
	".mov": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".webm": true,
}

// soleVideo reports whether media is the only video in its folder. Kodi's movie.nfo describes a
// folder, so in a folder of several movies it describes none of them.
func soleVideo(media string) bool {
	entries, err := os.ReadDir(filepath.Dir(media))
	if err != nil {
		return false
	}
	videos := 0
	for _, e := range entries {
		if !e.IsDir() && videoExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
			videos++
		}
	}
	return videos <= 1
}

func sidecar(media string) string {
	return strings.TrimSuffix(media, filepath.Ext(media)) + ".nfo"
}

// readBounded reads a regular file of at most MaxSize bytes. It does not follow a symlink: the
// opened file must be the one Lstat saw, so a link swapped in between is refused too.
func readBounded(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegularFile, filepath.Base(path))
	}
	if info.Size() > MaxSize {
		return nil, fmt.Errorf("%w: %s is %d bytes", ErrTooLarge, filepath.Base(path), info.Size())
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, fmt.Errorf("%w: %s changed while it was opened", ErrNotRegularFile, filepath.Base(path))
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSize {
		return nil, fmt.Errorf("%w: %s grew while it was read", ErrTooLarge, filepath.Base(path))
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
// multi-episode file) yields one record per episode, each with its own ids: the first, with the
// rest in More. HTML's named entities (&eacute; …) are understood; no other entity can be defined.
func Parse(kind mp.Kind, data []byte) (mp.Record, error) {
	root, ok := rootElement[kind]
	if !ok {
		return mp.Record{}, fmt.Errorf("nfo: %s: %w", kind, mp.ErrUnsupported)
	}
	if len(data) > MaxSize {
		return mp.Record{}, ErrTooLarge
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = charsetReader
	var docs []document
	var end int64
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Kodi allows a scraper URL after the document, which is not ours to read. Markup
			// after the last complete element is a broken document, not a URL.
			if len(docs) > 0 && !bytes.Contains(data[end:], []byte("<")) {
				break
			}
			return mp.Record{}, fmt.Errorf("%w: %w", ErrMalformed, err)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return mp.Record{}, ErrDirective
		case xml.StartElement:
			if t.Name.Local != root {
				if len(docs) > 0 {
					return records(kind, docs), nil
				}
				return mp.Record{}, fmt.Errorf("%w: root element %q is not %q", ErrMalformed, t.Name.Local, root)
			}
			var doc document
			if err := dec.DecodeElement(&doc, &t); err != nil {
				return mp.Record{}, fmt.Errorf("%w: %w", ErrMalformed, err)
			}
			docs = append(docs, doc)
			end = dec.InputOffset()
			if kind != mp.KindEpisode {
				return records(kind, docs), nil
			}
		}
	}
	if len(docs) == 0 {
		return mp.Record{}, fmt.Errorf("nfo: no %s element: %w", root, mp.ErrNotFound)
	}
	return records(kind, docs), nil
}

// records is the first document's record with the rest in More.
func records(kind mp.Kind, docs []document) mp.Record {
	rec := recordFrom(kind, docs[0])
	for _, doc := range docs[1:] {
		rec.More = append(rec.More, recordFrom(kind, doc))
	}
	return rec
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

func recordFrom(kind mp.Kind, doc document) mp.Record {
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
			Runtime:       time.Duration(leadingMinutes(doc.Runtime)) * time.Minute,
		},
	}
	if rec.Descriptive.Year == 0 && len(rec.Descriptive.PremiereDate) == 10 {
		rec.Descriptive.Year, _ = strconv.Atoi(rec.Descriptive.PremiereDate[:4])
	}
	if rating, country, ok := certification(doc.MPAA); ok {
		rec.Certification = &mp.Certification{Rating: rating, Country: country}
	}
	if kind == mp.KindEpisode {
		season, okS := whole(doc.Season)
		episode, okE := whole(doc.Episode)
		if okS && okE {
			rec.Numbering = &mp.Numbering{Season: season, Episode: episode}
		}
	}
	return rec
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

// leadingMinutes reads Kodi's runtime, which is minutes, sometimes followed by a unit ("112 min").
// Any other form is a gap.
func leadingMinutes(s string) int {
	s = strings.TrimSpace(s)
	digits := strings.TrimRightFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if rest := strings.TrimSpace(s[len(digits):]); rest != "" && !strings.EqualFold(rest, "min") && !strings.EqualFold(rest, "mins") && !strings.EqualFold(rest, "minutes") {
		return 0
	}
	return positive(digits)
}

// charsetReader decodes the legacy single-byte encodings old scrapers wrote. UTF-8 needs none.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	var high *[32]rune
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1":
	case "windows-1252", "cp1252":
		high = &cp1252
	default:
		return nil, fmt.Errorf("nfo: unsupported encoding %q", label)
	}
	raw, err := io.ReadAll(io.LimitReader(input, MaxSize))
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	out.Grow(len(raw))
	for _, b := range raw {
		r := rune(b)
		if high != nil && b >= 0x80 && b < 0xA0 && high[b-0x80] != 0 {
			r = high[b-0x80]
		}
		out.WriteRune(r)
	}
	return strings.NewReader(out.String()), nil
}

// cp1252 is windows-1252's 0x80–0x9F block; zero entries are undefined and pass through.
var cp1252 = [32]rune{
	'€', 0, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0, 'Ž', 0,
	0, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0, 'ž', 'Ÿ',
}

// whole reads a non-negative integer; season 0 holds specials.
func whole(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil && n >= 0
}
