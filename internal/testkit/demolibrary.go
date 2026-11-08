package testkit

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/demolibrary"
)

// DemoLibraryToken is the API key the stand-in demo library accepts.
const DemoLibraryToken = "demo-library-token"

// DemoLibrary runs internal/demolibrary's Emby stand-in over httptest. The stand-in answers
// exactly the queries internal/library sends from one invented catalogue, so a test reads real
// item shapes instead of an authored JSON literal. Every request is recorded so a test can
// assert the query the client sent.
type DemoLibrary struct {
	*httptest.Server
	// Layout is the stand-in's asset directory. Item paths resolve under Layout.Dir; a film's
	// Size appears only once its file exists there.
	Layout demolibrary.Layout

	mu       sync.Mutex
	requests []url.URL
}

// NewDemoLibrary starts the stand-in on a fresh temp layout and closes it with the test.
func NewDemoLibrary(t testing.TB) *DemoLibrary {
	t.Helper()
	d := &DemoLibrary{Layout: demolibrary.Layout{Dir: t.TempDir()}}
	inner := demolibrary.Server{Layout: d.Layout, Library: demolibrary.Invented(), Token: DemoLibraryToken}
	d.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.requests = append(d.requests, *r.URL)
		d.mu.Unlock()
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(d.Close)
	return d
}

// Requests returns every request URL the stand-in received, in arrival order.
func (d *DemoLibrary) Requests() []url.URL {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]url.URL(nil), d.requests...)
}

// LastQuery is the query of the most recent request; it fails the test when there was none.
func (d *DemoLibrary) LastQuery(t testing.TB) url.Values {
	t.Helper()
	reqs := d.Requests()
	if len(reqs) == 0 {
		t.Fatal("the demo library received no request")
	}
	return reqs[len(reqs)-1].Query()
}

// FirstMovie is the catalogue's first film.
func FirstMovie() demolibrary.Title { return firstOfKind(demolibrary.Movie) }

// FirstSeries is the catalogue's first series.
func FirstSeries() demolibrary.Title { return firstOfKind(demolibrary.Series) }

func firstOfKind(kind demolibrary.Kind) demolibrary.Title {
	for _, title := range demolibrary.Catalogue {
		if title.Kind == kind {
			return title
		}
	}
	panic("demolibrary catalogue has no " + string(kind))
}
