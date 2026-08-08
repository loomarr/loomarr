package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// authorizeWith runs authorizePlayout against a synthetic request carrying `query` and a path
// value of `id`, returning whether it authorized. Exercises the real branch the HLS routes use
// without standing up a full router.
func authorizeWith(s *Server, id, query string) bool {
	req := httptest.NewRequest(http.MethodGet, "/v1/playout/hls/"+id+"/master.m3u8"+query, nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	return s.authorizePlayout(rec, req)
}

// serverWithToken builds a bare Server whose only wired dependency is the playout token — enough
// to exercise the pure sign/verify path without a store or a router.
func serverWithToken(token string) *Server {
	return &Server{playoutSecret: func() string { return token }}
}

const testPlayoutTok = "sign-test-token-abcdefghijklmnop"

func TestSignPlayout_VerifiesRoundTrip(t *testing.T) {
	s := serverWithToken(testPlayoutTok)
	now := time.Now()
	sig := s.signPlayout("ch_abc", now.Add(time.Hour))
	if sig == "" {
		t.Fatal("signPlayout returned empty with a token set")
	}
	if !s.verifyPlayoutSignature(sig, "ch_abc", now) {
		t.Fatal("a freshly signed URL failed to verify for its own channel")
	}
}

func TestVerifyPlayoutSignature_RejectsWrongChannel(t *testing.T) {
	s := serverWithToken(testPlayoutTok)
	now := time.Now()
	sig := s.signPlayout("ch_abc", now.Add(time.Hour))
	// A URL minted for ch_abc must not pull ch_xyz — the channel is inside the signed material.
	if s.verifyPlayoutSignature(sig, "ch_xyz", now) {
		t.Fatal("a signature for ch_abc verified against ch_xyz")
	}
}

func TestVerifyPlayoutSignature_RejectsExpired(t *testing.T) {
	s := serverWithToken(testPlayoutTok)
	base := time.Now()
	sig := s.signPlayout("ch_abc", base.Add(time.Minute))
	// One second past the deadline must fail, even for the right channel and an untampered sig.
	if s.verifyPlayoutSignature(sig, "ch_abc", base.Add(time.Minute+time.Second)) {
		t.Fatal("an expired signature verified")
	}
}

func TestVerifyPlayoutSignature_RejectsTamper(t *testing.T) {
	s := serverWithToken(testPlayoutTok)
	now := time.Now()
	sig := s.signPlayout("ch_abc", now.Add(time.Hour))

	// Flip the last character of the MAC. Any single-character change must fail the constant-time
	// compare.
	tampered := sig[:len(sig)-1]
	if strings.HasSuffix(sig, "A") {
		tampered += "B"
	} else {
		tampered += "A"
	}
	if s.verifyPlayoutSignature(tampered, "ch_abc", now) {
		t.Fatal("a tampered signature verified")
	}

	// A shifted expiry (re-encoding a later deadline without re-signing) must also fail: the
	// expiry is signed, so it cannot be edited in the URL to extend the capability.
	parts := strings.Split(sig, ":")
	if len(parts) >= 3 {
		parts[len(parts)-2] = "99999999999" // far-future exp, original MAC
		forged := strings.Join(parts, ":")
		if s.verifyPlayoutSignature(forged, "ch_abc", now) {
			t.Fatal("an extended-expiry forgery verified — the expiry is not covered by the MAC")
		}
	}
}

func TestVerifyPlayoutSignature_RejectsWhenNoToken(t *testing.T) {
	// With no token configured there is no key to verify against — everything fails closed, the
	// same posture the device-token path takes.
	signed := serverWithToken(testPlayoutTok).signPlayout("ch_abc", time.Now().Add(time.Hour))
	noKey := serverWithToken("")
	if noKey.verifyPlayoutSignature(signed, "ch_abc", time.Now()) {
		t.Fatal("verified a signature with no token configured")
	}
	if noKey.signPlayout("ch_abc", time.Now().Add(time.Hour)) != "" {
		t.Fatal("signed with no token configured")
	}
}

func TestVerifyPlayoutSignature_RejectsMalformed(t *testing.T) {
	s := serverWithToken(testPlayoutTok)
	now := time.Now()
	for _, raw := range []string{"", "onlyonepart", "two:parts", "ch:notanumber:sig"} {
		if s.verifyPlayoutSignature(raw, "ch", now) {
			t.Fatalf("malformed capability %q verified", raw)
		}
	}
}

// The HLS routes authorize on a signed URL for THE PATH'S channel — and reject a signature minted
// for a different channel, so a valid capability for ch1 cannot be replayed against ch2.
func TestAuthorizePlayout_AcceptsSignedURLForThatChannel(t *testing.T) {
	s := serverWithToken(testPlayoutTok)
	sig := s.signPlayout("ch1", time.Now().Add(time.Hour))

	if !authorizeWith(s, "ch1", "?"+signQueryParam+"="+sig) {
		t.Fatal("a valid signed URL for ch1 was rejected on ch1")
	}
	if authorizeWith(s, "ch2", "?"+signQueryParam+"="+sig) {
		t.Fatal("a signed URL for ch1 authorized ch2 — cross-channel replay")
	}
	if authorizeWith(s, "ch1", "?"+signQueryParam+"=garbage") {
		t.Fatal("a garbage signature authorized")
	}
}

// The device token still works on these routes (a media server uses the same URLs), alongside the
// signed-URL path.
func TestAuthorizePlayout_AcceptsDeviceToken(t *testing.T) {
	s := serverWithToken(testPlayoutTok)
	if !authorizeWith(s, "ch1", "?"+playoutTokenParam+"="+testPlayoutTok) {
		t.Fatal("the device token was rejected on an HLS route")
	}
	if authorizeWith(s, "ch1", "?"+playoutTokenParam+"=wrong") {
		t.Fatal("a wrong device token authorized")
	}
}

func TestPlayoutHLSURL_CarriesSigAndQuality(t *testing.T) {
	s := &Server{
		playoutSecret: func() string { return testPlayoutTok },
		liveConfig:    func(k string) string { return map[string]string{"server.public_url": "http://loomarr.local:8080"}[k] },
	}
	u := s.playoutHLSURL("ch_abc", "720", time.Now().Add(time.Hour))
	if u == "" {
		t.Fatal("empty HLS URL with public_url and token set")
	}
	if !strings.Contains(u, "/v1/playout/hls/ch_abc/master.m3u8") {
		t.Fatalf("URL does not point at the master playlist: %s", u)
	}
	if !strings.Contains(u, signQueryParam+"=") {
		t.Fatalf("URL carries no signature: %s", u)
	}
	if !strings.Contains(u, "quality=720") {
		t.Fatalf("URL dropped the quality cap: %s", u)
	}

	// No public URL ⇒ no ABSOLUTE URL, matching the other playout builders' "not configured"
	// signal. But the RELATIVE URL must still be built — the web player uses it and does not need
	// public_url — which is the whole point of returning both.
	noBase := &Server{playoutSecret: func() string { return testPlayoutTok }, liveConfig: func(string) string { return "" }}
	if noBase.playoutHLSURL("ch_abc", "", time.Now().Add(time.Hour)) != "" {
		t.Fatal("built an ABSOLUTE HLS URL with no public_url")
	}
	rel := noBase.playoutHLSPathURL("ch_abc", "720", time.Now().Add(time.Hour))
	if rel == "" {
		t.Fatal("relative HLS URL should be built even with no public_url")
	}
	if !strings.HasPrefix(rel, "/v1/playout/hls/ch_abc/master.m3u8?") {
		t.Fatalf("relative URL is not same-origin: %s", rel)
	}
	if strings.Contains(rel, "://") {
		t.Fatalf("relative URL must carry no scheme/host: %s", rel)
	}
	if !strings.Contains(rel, signQueryParam+"=") || !strings.Contains(rel, "quality=720") {
		t.Fatalf("relative URL missing sig or quality: %s", rel)
	}
}
