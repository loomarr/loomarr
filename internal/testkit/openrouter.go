package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

// OpenRouterRequest is the request subset certification tests inspect at the
// actual OpenAI-compatible HTTP boundary.
type OpenRouterRequest struct {
	Model    string          `json:"model"`
	Provider OpenRouterRoute `json:"provider"`
}

type OpenRouterRoute struct {
	Order             []string `json:"order"`
	AllowFallbacks    *bool    `json:"allow_fallbacks"`
	RequireParameters *bool    `json:"require_parameters"`
	DataCollection    string   `json:"data_collection"`
	ZDR               *bool    `json:"zdr"`
}

// OpenRouter is a loopback test server for strict request-body and response
// attribution tests. It never reaches a real provider.
type OpenRouter struct {
	URL       string
	server    *httptest.Server
	mu        sync.Mutex
	responses []string
	requests  []OpenRouterRequest
}

func NewOpenRouter(responses ...string) *OpenRouter {
	o := &OpenRouter{responses: append([]string(nil), responses...)}
	o.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request OpenRouterRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		o.mu.Lock()
		o.requests = append(o.requests, request)
		response := "{}"
		if len(o.responses) > 0 {
			response = o.responses[0]
			o.responses = o.responses[1:]
		}
		o.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	o.URL = o.server.URL
	return o
}

func (o *OpenRouter) Close() { o.server.Close() }

func (o *OpenRouter) Requests() []OpenRouterRequest {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]OpenRouterRequest, len(o.requests))
	copy(out, o.requests)
	return out
}
