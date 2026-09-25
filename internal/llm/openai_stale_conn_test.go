package llm_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
)

// staleConnServer is a raw TCP HTTP/1.1 server whose behaviour is scripted per request, so a test
// can do what a llama.cpp-style server does and net/http/httptest cannot do deterministically:
// answer once over a keep-alive connection, then drop that connection while the client has it
// pooled.
type staleConnServer struct {
	ln       net.Listener
	requests atomic.Int32
	keys     sync.Map // Idempotency-Key values seen, to prove each request carries its own
	// script decides, for the nth request (1-based) on a connection, what to do.
	script func(n int, conn net.Conn, req *http.Request) (keepOpen bool)
}

func newStaleConnServer(t *testing.T, script func(n int, conn net.Conn, req *http.Request) bool) *staleConnServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &staleConnServer{ln: ln, script: script}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *staleConnServer) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	rd := bufio.NewReader(conn)
	for {
		req, err := http.ReadRequest(rd)
		if err != nil {
			return
		}
		_, _ = rd.Discard(int(req.ContentLength))
		n := int(s.requests.Add(1))
		if key := req.Header.Get("Idempotency-Key"); key != "" {
			s.keys.Store(key, struct{}{})
		}
		if !s.script(n, conn, req) {
			return
		}
	}
}

func (s *staleConnServer) url() string { return "http://" + s.ln.Addr().String() + "/v1" }

const okChatBody = `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`

func writeOK(conn net.Conn, req *http.Request) {
	resp := &http.Response{StatusCode: 200, ProtoMajor: 1, ProtoMinor: 1, Header: http.Header{"Content-Type": {"application/json"}},
		ContentLength: int64(len(okChatBody)), Body: http.NoBody}
	dump, _ := httputil.DumpResponse(resp, false)
	_, _ = conn.Write(append(dump, okChatBody...))
	_ = req
}

func chatOnce(o *llm.OpenAI, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := o.Chat(ctx, []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{})
	return err
}

// #1493: llama.cpp closes an idle keep-alive connection after ~5 s. Go had it pooled, wrote the
// next call onto it and got "http: server closed idle connection" back in 1 ms, because a chat
// POST is not replayable. The call must instead go out again on a fresh connection.
func TestOpenAI_ReplaysOnKeepAliveConnectionTheServerClosed(t *testing.T) {
	srv := newStaleConnServer(t, func(n int, conn net.Conn, req *http.Request) bool {
		switch n {
		case 1:
			writeOK(conn, req)
			return true // keep-alive: the client pools this connection
		case 2:
			return false // server drops the pooled connection without answering
		default:
			writeOK(conn, req)
			return true
		}
	})
	o := llm.NewOpenAI(srv.url(), "m", "")

	if err := chatOnce(o, 5*time.Second); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := chatOnce(o, 5*time.Second); err != nil {
		t.Fatalf("second call over a connection the server closed must succeed: %v", err)
	}
}

// Each chat POST carries its own Idempotency-Key: what makes Go's transport willing to replay it.
func TestOpenAI_EachPostCarriesAFreshIdempotencyKey(t *testing.T) {
	srv := newStaleConnServer(t, func(_ int, conn net.Conn, req *http.Request) bool {
		writeOK(conn, req)
		return true
	})
	o := llm.NewOpenAI(srv.url(), "m", "")
	for range 3 {
		if err := chatOnce(o, 5*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	keys := 0
	srv.keys.Range(func(_, _ any) bool { keys++; return true })
	if keys != 3 {
		t.Fatalf("want 3 distinct Idempotency-Keys for 3 calls, got %d", keys)
	}
}

// The replay is strictly for "the write never reached a live server". None of these may be retried.
func TestOpenAI_DoesNotRetryWhatMayHaveReachedTheServer(t *testing.T) {
	t.Run("500 is a real answer", func(t *testing.T) {
		srv := newStaleConnServer(t, func(_ int, conn net.Conn, _ *http.Request) bool {
			_, _ = conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\nContent-Length: 0\r\n\r\n"))
			return true
		})
		if err := chatOnce(llm.NewOpenAI(srv.url(), "m", ""), 5*time.Second); err == nil {
			t.Fatal("a 500 must surface as an error")
		}
		if got := srv.requests.Load(); got != 1 {
			t.Fatalf("500 was retried: server saw %d requests, want 1", got)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := newStaleConnServer(t, func(_ int, _ net.Conn, _ *http.Request) bool {
			time.Sleep(600 * time.Millisecond) // never answers within the caller's budget
			return false
		})
		if err := chatOnce(llm.NewOpenAI(srv.url(), "m", ""), 200*time.Millisecond); err == nil {
			t.Fatal("a timeout must surface as an error")
		}
		if got := srv.requests.Load(); got != 1 {
			t.Fatalf("timeout was retried: server saw %d requests, want 1", got)
		}
	})

	t.Run("fresh connection dropped after the request was read", func(t *testing.T) {
		srv := newStaleConnServer(t, func(_ int, _ net.Conn, _ *http.Request) bool {
			return false // the server read the request, then hung up: it may have started generating
		})
		if err := chatOnce(llm.NewOpenAI(srv.url(), "m", ""), 5*time.Second); err == nil {
			t.Fatal("a dropped fresh connection must surface as an error")
		}
		if got := srv.requests.Load(); got != 1 {
			t.Fatalf("request on a fresh connection was replayed: server saw %d, want 1", got)
		}
	})
}
