// Package httpfixture provides shared no-network HTTP test seams without
// importing any application adapter.
package httpfixture

import (
	"fmt"
	"io"
	"net/http"
	"sync"
)

// RoundTripperFunc adapts a function to http.RoundTripper.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Step is one result returned by a ScriptedTransport.
type Step struct {
	Response *http.Response
	Err      error
}

// Request records the stable parts of one attempt, including the payload before
// the underlying transport closes it.
type Request struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

// ScriptedTransport returns a fixed sequence of HTTP results without opening a
// socket. It is safe to share across concurrent requests.
type ScriptedTransport struct {
	mu       sync.Mutex
	steps    []Step
	requests []Request
}

func NewScriptedTransport(steps ...Step) *ScriptedTransport {
	return &ScriptedTransport{steps: append([]Step(nil), steps...)}
}

func (s *ScriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot, err := requestSnapshot(req)
	if err != nil {
		return nil, err
	}
	s.requests = append(s.requests, snapshot)
	index := len(s.requests) - 1
	if index >= len(s.steps) {
		return nil, fmt.Errorf("httpfixture: unexpected round trip %d", index+1)
	}
	step := s.steps[index]
	if step.Response != nil {
		if step.Response.Body == nil {
			step.Response.Body = http.NoBody
		}
		if step.Response.Request == nil {
			step.Response.Request = req
		}
	}
	return step.Response, step.Err
}

func (s *ScriptedTransport) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	requests := make([]Request, len(s.requests))
	copy(requests, s.requests)
	for i := range requests {
		requests[i].Header = requests[i].Header.Clone()
		requests[i].Body = append([]byte(nil), requests[i].Body...)
	}
	return requests
}

func (s *ScriptedTransport) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func requestSnapshot(req *http.Request) (Request, error) {
	snapshot := Request{Method: req.Method, URL: req.URL.String(), Header: req.Header.Clone()}
	if req.Body == nil || req.Body == http.NoBody {
		return snapshot, nil
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return Request{}, fmt.Errorf("httpfixture: read request body: %w", err)
	}
	snapshot.Body = body
	return snapshot, nil
}
