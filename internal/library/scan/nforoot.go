package scan

import (
	"encoding/xml"
	"io"
	"os"
	"strings"
	"sync"
)

// nfoRoots remembers the root element of each NFO it has read, by the NFO's revision, so a file
// with no mark is read for whether its NFO says it is an episode once, not every pass.
type nfoRoots struct {
	mu   sync.Mutex
	seen map[string]nfoRoot
}

type nfoRoot struct {
	st   Stat
	root string
}

// rootOf is the lower-cased name of the NFO's root element, "" for one that does not read.
func (c *nfoRoots) rootOf(path string, st Stat) string {
	c.mu.Lock()
	if got, ok := c.seen[path]; ok && got.st == st {
		c.mu.Unlock()
		return got.root
	}
	c.mu.Unlock()
	root := readNFORoot(path)
	c.mu.Lock()
	if c.seen == nil {
		c.seen = map[string]nfoRoot{}
	}
	c.seen[path] = nfoRoot{st: st, root: root}
	c.mu.Unlock()
	return root
}

// readNFORoot reads an NFO only as far as its root element.
func readNFORoot(path string) string {
	f, err := os.Open(path) //nolint:gosec // a sidecar the walk found under a library root
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	d := xml.NewDecoder(io.LimitReader(f, 64<<10))
	d.Strict = false
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	for {
		tok, err := d.Token()
		if err != nil {
			return ""
		}
		if start, ok := tok.(xml.StartElement); ok {
			return strings.ToLower(start.Name.Local)
		}
	}
}
