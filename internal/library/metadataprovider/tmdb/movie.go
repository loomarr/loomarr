package tmdb

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// ---- movie ----

type wireMovie struct {
	ID            int              `json:"id"`
	IMDB          string           `json:"imdb_id"`
	Title         string           `json:"title"`
	OriginalTitle string           `json:"original_title"`
	Overview      string           `json:"overview"`
	ReleaseDate   string           `json:"release_date"`
	Runtime       int              `json:"runtime"`
	Tagline       string           `json:"tagline"`
	VoteAverage   float64          `json:"vote_average"`
	VoteCount     int              `json:"vote_count"`
	Genres        []idName         `json:"genres"`
	Companies     []idName         `json:"production_companies"`
	Countries     []idName         `json:"production_countries"`
	Videos        wireVideos       `json:"videos"`
	Credits       wireCredits      `json:"credits"`
	PosterPath    string           `json:"poster_path"`
	BackdropPath  string           `json:"backdrop_path"`
	Collection    *wireCollection  `json:"belongs_to_collection"`
	ExternalIDs   wireExternalIDs  `json:"external_ids"`
	Images        wireImages       `json:"images"`
	Translations  wireTranslations `json:"translations"`
	Keywords      struct {
		Keywords []idName `json:"keywords"`
	} `json:"keywords"`
	ReleaseDatesWrap struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Dates   []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
}

type wireCollection struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (p *Provider) movie(ctx context.Context, loc locale, id int) (mp.Record, error) {
	var m wireMovie
	path := "/movie/" + strconv.Itoa(id) + "?" + loc.detailQuery("external_ids,release_dates,keywords,images,videos,credits")
	if err := p.get(ctx, path, &m); err != nil {
		return mp.Record{}, err
	}
	if m.ID != id {
		return mp.Record{}, fmt.Errorf("tmdb movie %d: answer names movie %d: %w", id, m.ID, mp.ErrNotFound)
	}
	rec := mp.Record{
		Source: source,
		Kind:   mp.KindMovie,
		IDs: ids(
			mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(m.ID)},
			mp.ExternalID{Namespace: mp.NamespaceIMDB, Value: firstNonEmpty(m.ExternalIDs.IMDB, m.IMDB)},
		),
		Descriptive: &mp.Descriptive{
			Title:           m.Title,
			OriginalTitle:   m.OriginalTitle,
			Overview:        m.Overview,
			PremiereDate:    m.ReleaseDate,
			Year:            yearOf(m.ReleaseDate),
			Genres:          names(m.Genres),
			Keywords:        keywords(m.Keywords.Keywords),
			CommunityRating: communityRating(m.VoteAverage, m.VoteCount),
			Studios:         names(m.Companies),
			Runtime:         time.Duration(m.Runtime) * time.Minute,
			Tagline:         strings.TrimSpace(m.Tagline),
			Countries:       names(m.Countries),
			Trailer:         trailer(m.Videos),
			People:          people(m.Credits, nil),
		},
		Artwork: artwork(loc, m.Images, m.PosterPath, m.BackdropPath),
	}
	if m.Collection != nil && m.Collection.ID > 0 {
		rec.Descriptive.Collection = &mp.CollectionRef{
			ID:   mp.ExternalID{Namespace: mp.NamespaceTMDBCollection, Value: strconv.Itoa(m.Collection.ID)},
			Name: m.Collection.Name,
		}
	}
	fillFromEnglish(rec.Descriptive, m.Translations)
	var ratings []string
	for _, r := range m.ReleaseDatesWrap.Results {
		if strings.EqualFold(r.Country, usCountry) {
			for _, d := range r.Dates {
				ratings = append(ratings, d.Certification)
			}
		}
	}
	rec.Certification = strictest(ratings)
	return rec, nil
}

// ---- person ----

// maxFilmography bounds the titles kept per person, newest first.
const maxFilmography = 300

// wireFilmCredit is one row of a person's combined credits: a movie (title, release_date) or a
// series (name, first_air_date).
type wireFilmCredit struct {
	ID           int    `json:"id"`
	MediaType    string `json:"media_type"`
	Title        string `json:"title"`
	Name         string `json:"name"`
	ReleaseDate  string `json:"release_date"`
	FirstAirDate string `json:"first_air_date"`
	Character    string `json:"character"`
	Job          string `json:"job"`
}

// selfParts are the characters TMDB gives a person appearing as themselves: talk shows and
// documentaries, not roles.
var selfParts = map[string]bool{"self": true, "himself": true, "herself": true, "themselves": true}

// Filmography is a person's movies and series by their TMDB person id (#2151 decision 50): every
// part they played, other than appearing as themselves, and every title they created, directed
// or wrote, newest first, at most maxFilmography.
func (p *Provider) Filmography(ctx context.Context, person mp.ExternalID) ([]mp.FilmCredit, error) {
	if person.Namespace != mp.NamespaceTMDB {
		return nil, fmt.Errorf("tmdb: person by %s: %w", person.Namespace, mp.ErrUnsupported)
	}
	id, err := strconv.Atoi(strings.TrimSpace(person.Value))
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("tmdb: person id %q is not a positive integer: %w", person.Value, mp.ErrUnsupported)
	}
	loc, err := p.resolveLocale()
	if err != nil {
		return nil, err
	}
	var credits struct {
		Cast []wireFilmCredit `json:"cast"`
		Crew []wireFilmCredit `json:"crew"`
	}
	q := url.Values{}
	q.Set("language", loc.language)
	if err := p.get(ctx, "/person/"+strconv.Itoa(id)+"/combined_credits?"+q.Encode(), &credits); err != nil {
		return nil, err
	}
	var out []mp.FilmCredit
	seen := map[string]bool{}
	add := func(c wireFilmCredit, role mp.PersonRole, character string) {
		kind := map[string]mp.Kind{"movie": mp.KindMovie, "tv": mp.KindSeries}[c.MediaType]
		title := strings.TrimSpace(firstNonEmpty(c.Title, c.Name))
		key := string(kind) + "\x00" + strconv.Itoa(c.ID) + "\x00" + string(role)
		if kind == "" || c.ID <= 0 || title == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, mp.FilmCredit{Kind: kind, ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(c.ID)}, Title: title,
			Year: yearOf(firstNonEmpty(c.ReleaseDate, c.FirstAirDate)), Role: role, Character: character})
	}
	for _, c := range credits.Cast {
		character := strings.TrimSpace(c.Character)
		if !selfParts[strings.ToLower(character)] {
			add(c, mp.PersonActor, character)
		}
	}
	for _, c := range credits.Crew {
		if role, ok := crewRoles[c.Job]; ok {
			add(c, role, "")
		}
	}
	slices.SortStableFunc(out, func(a, b mp.FilmCredit) int {
		return cmp.Or(cmp.Compare(b.Year, a.Year), strings.Compare(a.Title, b.Title))
	})
	if len(out) > maxFilmography {
		out = out[:maxFilmography]
	}
	return out, nil
}

// ---- collection ----

type wireCollectionDetail struct {
	ID           int              `json:"id"`
	Name         string           `json:"name"`
	Overview     string           `json:"overview"`
	PosterPath   string           `json:"poster_path"`
	BackdropPath string           `json:"backdrop_path"`
	Images       wireImages       `json:"images"`
	Translations wireTranslations `json:"translations"`
	Parts        []struct {
		ID          int    `json:"id"`
		MediaType   string `json:"media_type"`
		Title       string `json:"title"`
		ReleaseDate string `json:"release_date"`
	} `json:"parts"`
}

// collection is a TMDB collection: its name, its artwork, and its movies in release order (an
// unreleased one last), each as a record with its TMDB id, title and year.
func (p *Provider) collection(ctx context.Context, loc locale, id int) (mp.Record, error) {
	var c wireCollectionDetail
	path := "/collection/" + strconv.Itoa(id) + "?" + loc.detailQuery("images")
	if err := p.get(ctx, path, &c); err != nil {
		return mp.Record{}, err
	}
	if c.ID != id {
		return mp.Record{}, fmt.Errorf("tmdb collection %d: answer names collection %d: %w", id, c.ID, mp.ErrNotFound)
	}
	parts := c.Parts[:0:0]
	for _, part := range c.Parts {
		if part.ID > 0 && strings.TrimSpace(part.Title) != "" && (part.MediaType == "" || part.MediaType == "movie") {
			parts = append(parts, part)
		}
	}
	sort.SliceStable(parts, func(i, j int) bool {
		a, b := parts[i].ReleaseDate, parts[j].ReleaseDate
		if (a == "") != (b == "") {
			return b == ""
		}
		return a < b
	})
	rec := mp.Record{
		Source:      source,
		Kind:        mp.KindCollection,
		IDs:         []mp.ExternalID{{Namespace: mp.NamespaceTMDBCollection, Value: strconv.Itoa(c.ID)}},
		Descriptive: &mp.Descriptive{Title: c.Name, Overview: c.Overview},
		Artwork:     artwork(loc, c.Images, c.PosterPath, c.BackdropPath),
	}
	fillFromEnglish(rec.Descriptive, c.Translations)
	for _, part := range parts {
		rec.Children = append(rec.Children, mp.Record{
			Source: source,
			Kind:   mp.KindMovie,
			IDs:    []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: strconv.Itoa(part.ID)}},
			Descriptive: &mp.Descriptive{
				Title: strings.TrimSpace(part.Title), PremiereDate: part.ReleaseDate, Year: yearOf(part.ReleaseDate),
			},
		})
	}
	return rec, nil
}
