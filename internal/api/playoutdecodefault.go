package api

import (
	"net/url"
	"sync"
)

// maxDecodeFaultSources bounds the memory. A household has a few hundred episodes at most; when the
// cap is hit the oldest entry is forgotten, which only costs that source one more hardware attempt.
const maxDecodeFaultSources = 512

// decodeFaultSet remembers sources whose GPU decode failed (#1401), for the life of the server.
// The zero value is ready to use.
type decodeFaultSet struct {
	mu    sync.Mutex
	known map[string]struct{}
	order []string
}

func (d *decodeFaultSet) has(source string) bool {
	if source == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.known[source]
	return ok
}

// add reports whether source was newly remembered.
func (d *decodeFaultSet) add(source string) bool {
	if source == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.known[source]; ok {
		return false
	}
	if d.known == nil {
		d.known = make(map[string]struct{})
	}
	if len(d.order) >= maxDecodeFaultSources {
		delete(d.known, d.order[0])
		d.order = d.order[1:]
	}
	d.known[source] = struct{}{}
	d.order = append(d.order, source)
	return true
}

// decodeSourceKey identifies a media source independent of per-request credentials: the query
// string of a media-server URL carries tokens that can change between requests for the same item.
func decodeSourceKey(input string) string {
	u, err := url.Parse(input)
	if err != nil || u.Scheme == "" {
		return input
	}
	u.RawQuery, u.Fragment, u.User = "", "", nil
	return u.String()
}
