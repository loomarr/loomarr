package scan

import (
	"encoding/xml"
	"io"
	"os"
	"strings"
	"sync"
)

// nfoRoots remembers what each NFO it has read says a file is, by the NFO's revision, so a file
// with no mark is read for whether its NFO says it is an episode once, not every pass.
type nfoRoots struct {
	mu   sync.Mutex
	seen map[string]nfoRoot
}

type nfoRoot struct {
	st   Stat
	head nfoHead
}

// nfoHead is what an NFO says a file is: its root element, lower-cased, "" for one that does not
// read; and for an episode's, the show its <showtitle> names, "" for none.
type nfoHead struct {
	root, show string
}

// headOf is what the NFO says its file is.
func (c *nfoRoots) headOf(path string, st Stat) nfoHead {
	c.mu.Lock()
	if got, ok := c.seen[path]; ok && got.st == st {
		c.mu.Unlock()
		return got.head
	}
	c.mu.Unlock()
	head := readNFOHead(path)
	c.mu.Lock()
	if c.seen == nil {
		c.seen = map[string]nfoRoot{}
	}
	c.seen[path] = nfoRoot{st: st, head: head}
	c.mu.Unlock()
	return head
}

// readNFOHead reads an NFO as far as its root element, and an episode's as far as its first
// episode's <showtitle>.
func readNFOHead(path string) nfoHead {
	f, err := os.Open(path) //nolint:gosec // a sidecar the walk found under a library root
	if err != nil {
		return nfoHead{}
	}
	defer func() { _ = f.Close() }()
	d := xml.NewDecoder(io.LimitReader(f, 64<<10))
	d.Strict = false
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var head nfoHead
	for {
		tok, err := d.Token()
		if err != nil {
			return head
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			switch {
			case head.root == "":
				head.root = name
				if name != "episodedetails" {
					return head
				}
			case name == "showtitle":
				var show string
				if d.DecodeElement(&show, &t) == nil {
					head.show = strings.Join(strings.Fields(show), " ")
				}
				return head
			}
		case xml.EndElement:
			// The first episode's details end without a show named.
			if strings.EqualFold(t.Name.Local, "episodedetails") {
				return head
			}
		}
	}
}
