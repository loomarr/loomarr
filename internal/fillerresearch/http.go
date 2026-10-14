package fillerresearch

import (
	"fmt"
	"net/http"
	"net/url"
)

const maxProviderRedirects = 3

// sameOriginClient prevents an adapter-owned API request from turning into an arbitrary fetch.
// Clone the client so a shared observed transport keeps its metrics while this adapter owns only
// its redirect policy.
func sameOriginClient(client *http.Client, endpoint *url.URL) *http.Client {
	cloned := *client
	prior := client.CheckRedirect
	cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxProviderRedirects {
			return fmt.Errorf("provider redirect limit reached")
		}
		if req.URL.Scheme != endpoint.Scheme || req.URL.Host != endpoint.Host {
			return fmt.Errorf("provider redirect left the configured origin")
		}
		if prior != nil {
			return prior(req, via)
		}
		return nil
	}
	return &cloned
}
