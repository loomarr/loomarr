package scan

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/precedence"
)

// said is the title and year a name gives, as the catalog's search reads them.
type said struct {
	title string
	year  int
}

// saidBy is what a name says: its title and year as the catalog's search reads them.
func saidBy(name string) said {
	title, year, _ := searchQuery(strings.TrimSpace(embeddedID.ReplaceAllString(name, "")), 0)
	return said{title: title, year: year}
}

// told is what a title's own names say of it, which no record carries: a movie's file name, its
// folder and its length, or a show's folder. The catalog's search reads the name, and the record
// an id finds is weighed against them all.
type told struct {
	name said
	// folder is a movie's folder when it names one film by its year ("Hollow Pines (2004)"); a
	// folder of films, or a part's own ("CD1"), says nothing of it.
	folder said
	// length is a whole movie file's running time, 0 when no header has read it; nil for a part.
	// It is looked up only when the record's runtime is weighed.
	length func() time.Duration
}

// movieTold is what a movie file's names and length say of it. Its length is the header's as it
// is read, or else as the last read left it.
func (b *builder) movieTold(ctx context.Context, origin inventory.OriginKey, file File) told {
	t := told{name: saidBy(fallbackName(file.Rel, mp.KindMovie, file.Part, file.Sole))}
	if dir := path.Dir(file.Rel); dir != "." {
		if folder := saidBy(path.Base(dir)); folder.year > 0 {
			t.folder = folder
		}
	}
	if file.Part == 0 {
		t.length = func() time.Duration {
			if file.Length > 0 {
				return file.Length
			}
			if read, _ := b.im.lastHeader(ctx, origin); read != nil {
				return time.Duration(read.DurationMillis) * time.Millisecond
			}
			return 0
		}
	}
	return t
}

// namesAnother says whether the catalog record an id found is another title than the files name
// (local, resolved without it, and named). It is when they give a title and a year and neither
// the record's title nor its original title is theirs, its year more than one off: a remake
// shares its title, and a title the catalog knows in another language shares its year, so either
// one stands. A movie's record is also another film's when:
//   - no title its files give shares a word with it, and its runtime is not the file's: a
//     household names a film in its own language, and only the length then tells them apart;
//   - its title is the name's or the folder's with a sequel's number added or taken away
//     ("Hollow Pines 2" for "Hollow Pines"), as a sequel's NFO copied in names it;
//   - its runtime is far from the file's, and the name or folder is not its title word for word,
//     as a film's tag naming the special of the same name does. A long cut of the film named
//     word for word stands.
func namesAnother(local precedence.Resolved, named told, rec mp.Record) bool {
	d := rec.Descriptive
	if d == nil {
		return false
	}
	title, year := local.Title.Value, local.Year.Value
	if title == "" {
		title = named.name.title
	}
	if year <= 0 {
		year = named.name.year
	}
	if title != "" && year > 0 && d.Year > 0 && (d.Year-year > 1 || year-d.Year > 1) &&
		!oneTitle(d.Title, title) && !oneTitle(d.OriginalTitle, title) {
		return true
	}
	if rec.Kind != mp.KindMovie {
		return false
	}
	names := []string{named.name.title, named.folder.title}
	var length time.Duration
	if named.length != nil && d.Runtime > 0 {
		length = named.length()
	}
	if length > 0 && disjoint(append(names, local.Title.Value), d) && !sameLength(d.Runtime, length) {
		return true
	}
	for _, n := range names {
		if sequelOf(d.Title, n) {
			return true
		}
	}
	if length <= 0 || d.Runtime <= 0 {
		return false
	}
	gap := d.Runtime - length
	if gap < 0 {
		gap = -gap
	}
	if gap <= max(15*time.Minute, d.Runtime/4) {
		return false
	}
	for _, n := range names {
		if n != "" && !wordForWord(d.Title, n) && !wordForWord(d.OriginalTitle, n) {
			return true
		}
	}
	return false
}

// disjoint says whether titles a movie's files give, one at least, share no word with its
// record's title or original title. A title with no words of its own tells nothing.
func disjoint(titles []string, d *mp.Descriptive) bool {
	theirs := wordsOf(d.Title)
	for w := range wordsOf(d.OriginalTitle) {
		theirs[w] = true
	}
	some := false
	for _, t := range titles {
		if t == "" {
			continue
		}
		words := wordsOf(t)
		if len(words) == 0 {
			return false
		}
		for w := range words {
			if theirs[w] {
				return false
			}
		}
		some = true
	}
	return some
}

// wordForWord says whether two titles are told by the same words, small ones aside.
func wordForWord(a, b string) bool {
	wa, wb := wordsOf(a), wordsOf(b)
	if len(wa) == 0 || len(wa) != len(wb) {
		return false
	}
	for w := range wa {
		if !wb[w] {
			return false
		}
	}
	return true
}

// sequelOf says whether one title is the other with a sequel's number added or taken away: their
// words, a numeral read as its number, differ by that one number.
func sequelOf(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	number := func(s string) map[string]bool {
		set := map[string]bool{}
		for w := range wordsOf(s) {
			set[sequelNumber(w)] = true
		}
		return set
	}
	wa, wb := number(a), number(b)
	var diff []string
	for w := range wa {
		if !wb[w] {
			diff = append(diff, w)
		}
	}
	for w := range wb {
		if !wa[w] {
			diff = append(diff, w)
		}
	}
	return len(diff) == 1 && len(diff[0]) == 1 && diff[0] >= "2" && diff[0] <= "9"
}

// sequelNumber is a sequel's numeral as its number ("ii" is "2"); any other word as it is.
func sequelNumber(w string) string {
	if n, ok := map[string]string{"ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7", "viii": "8", "ix": "9"}[w]; ok {
		return n
	}
	return w
}
