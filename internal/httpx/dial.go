package httpx

import (
	"context"
	"net"
	"net/http"
	"time"
)

// dialTimeout bounds connect for every httpx transport (and TLS for the ones that
// do TLS themselves), so a black-holed host fails in seconds rather than after the
// operating system's multi-minute connect timeout. A variable only so tests can
// shorten it.
var dialTimeout = 10 * time.Second

// baseDial is the unbounded connect underneath boundedDial; tests replace it.
var baseDial DialContextFunc = (&net.Dialer{KeepAlive: 30 * time.Second}).DialContext

// boundedDial is the one dial function every httpx transport uses: baseDial under
// a dialTimeout deadline. It composes with PublicDialContext, which wraps it.
func boundedDial(ctx context.Context, network, addr string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	return baseDial(ctx, network, addr)
}

// baseTransport is the pooled transport every httpx client starts from. It honours
// the environment proxy (HTTP_PROXY, HTTPS_PROXY, NO_PROXY) and bounds dial and TLS
// setup. Homelab, LAN and tailnet targets are reached directly when the operator
// lists them in NO_PROXY; loopback is never proxied.
func baseTransport() *http.Transport {
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         boundedDial,
		TLSHandshakeTimeout: dialTimeout,
		// Conservative pooled defaults; a homelab has few upstreams.
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
}
