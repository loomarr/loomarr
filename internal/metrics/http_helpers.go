package metrics

import (
	"net/http"
	"strconv"
	"strings"
)

func routeLabel(pattern string) string {
	if pattern == "" {
		return "other"
	}
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		pattern = pattern[i+1:]
	}
	if pattern == "" {
		return "other"
	}
	return pattern
}

func statusCode(code int) string {
	if code == 0 {
		code = http.StatusOK
	}
	return strconv.Itoa(code)
}

type statusRecorder struct {
	http.ResponseWriter
	code    int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.code = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(body []byte) (int, error) {
	if !s.written {
		s.written = true
	}
	// This response-observing middleware does not create or transform content; the wrapped
	// application handler remains responsible for its response's content type and escaping.
	// codeql[go/reflected-xss]
	return s.ResponseWriter.Write(body)
}

func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
