package playoutcertfixture

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

// PostEventCorruptClient preserves the held raw streams through initial media
// validation, then replaces their bytes after the excess raw request is
// rejected. Selection is by the explicitly supplied held stream identities,
// rather than raw request order: unrelated phases can open raw responses before
// or around this cohort. Wrapped bodies stay valid until the gate closes.
func PostEventCorruptClient(base http.RoundTripper, heldStreamIDs []string) *http.Client {
	gate := make(chan struct{})
	var gateOnce sync.Once
	held := make(map[string]struct{}, len(heldStreamIDs))
	for _, streamID := range heldStreamIDs {
		held[streamID] = struct{}{}
	}
	return &http.Client{Transport: httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		response, err := base.RoundTrip(request)
		if err != nil || !strings.HasPrefix(request.URL.Path, "/v1/playout/stream/") {
			return response, err
		}
		streamID, unescapeErr := url.PathUnescape(strings.TrimPrefix(request.URL.Path, "/v1/playout/stream/"))
		if response.StatusCode == http.StatusOK && unescapeErr == nil {
			_, isHeld := held[streamID]
			if isHeld {
				response.Body = newPostEventCorruptBody(response.Body, gate)
			}
		}
		if response.StatusCode == http.StatusServiceUnavailable {
			gateOnce.Do(func() { close(gate) })
		}
		return response, nil
	})}
}

type postEventCorruptBody struct {
	source io.ReadCloser
	gate   <-chan struct{}
	closed chan struct{}
	once   sync.Once
}

func newPostEventCorruptBody(source io.ReadCloser, gate <-chan struct{}) *postEventCorruptBody {
	return &postEventCorruptBody{source: source, gate: gate, closed: make(chan struct{})}
}

func (b *postEventCorruptBody) Read(buffer []byte) (int, error) {
	select {
	case <-b.gate:
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-timer.C:
		case <-b.closed:
			if !timer.Stop() {
				<-timer.C
			}
			return 0, io.ErrClosedPipe
		}
		n := min(len(buffer), 188)
		for index := range n {
			buffer[index] = 0x7f
		}
		return n, nil
	default:
		return b.source.Read(buffer)
	}
}

func (b *postEventCorruptBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return b.source.Close()
}
