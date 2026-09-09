package playoutcertfixture

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

// GatedListener is a lifecycle adapter whose Close call remains in flight
// until Release is closed, then returns Err. It records the exact call count.
type GatedListener struct {
	Release <-chan struct{}
	Err     error
	Started chan struct{}
	Calls   atomic.Int32

	startedOnce sync.Once
}

func NewGatedListener(release <-chan struct{}, err error) *GatedListener {
	return &GatedListener{Release: release, Err: err, Started: make(chan struct{})}
}

func (l *GatedListener) Accept() (net.Conn, error) {
	return nil, errors.New("gated lifecycle listener does not accept connections")
}

func (l *GatedListener) Close() error {
	l.Calls.Add(1)
	l.startedOnce.Do(func() { close(l.Started) })
	if l.Release != nil {
		<-l.Release
	}
	return l.Err
}

func (l *GatedListener) Addr() net.Addr { return &net.TCPAddr{} }

// GatedHandler holds requests before forwarding them to the wrapped handler.
// Closing Release admits every held and subsequent request.
type GatedHandler struct {
	Handler http.Handler
	Release <-chan struct{}
	Started chan struct{}
	Calls   atomic.Int32

	startedOnce sync.Once
}

func NewGatedHandler(handler http.Handler, release <-chan struct{}) *GatedHandler {
	return &GatedHandler{Handler: handler, Release: release, Started: make(chan struct{})}
}

func (h *GatedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Calls.Add(1)
	h.startedOnce.Do(func() { close(h.Started) })
	if h.Release != nil {
		select {
		case <-h.Release:
		case <-r.Context().Done():
			http.Error(w, r.Context().Err().Error(), http.StatusRequestTimeout)
			return
		}
	}
	h.Handler.ServeHTTP(w, r)
}
