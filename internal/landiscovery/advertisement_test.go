package landiscovery

import (
	"errors"
	"net"
	"testing"
)

type fakeRegistration struct{ stopped bool }

func (r *fakeRegistration) Shutdown() { r.stopped = true }

func TestStartPublishesBoundedLoomarrService(t *testing.T) {
	registration := &fakeRegistration{}
	var got registrationRequest
	started, err := start(&net.TCPAddr{Port: 8080}, " living-room.local ", func(request registrationRequest) (Registration, error) {
		got = request
		return registration, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.instance != "Loomarr on living-room" || got.service != ServiceType || got.domain != "local." || got.port != 8080 {
		t.Fatalf("registration = %#v", got)
	}
	if len(got.text) != 2 || got.text[0] != "protocol=1" || got.text[1] != "scheme=http" {
		t.Fatalf("TXT = %#v", got.text)
	}
	started.Shutdown()
	if !registration.stopped {
		t.Fatal("shutdown did not stop the DNS-SD registration")
	}
}

func TestStartRejectsAnUnusableListenerBeforeRegistration(t *testing.T) {
	want := errors.New("must not register")
	for _, address := range []net.Addr{nil, &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}, &net.TCPAddr{Port: 0}} {
		if _, err := start(address, "host", func(registrationRequest) (Registration, error) { return nil, want }); err == nil || errors.Is(err, want) {
			t.Fatalf("start(%v) = %v, want local validation error", address, err)
		}
	}
}

func TestStartPropagatesRegistrationFailure(t *testing.T) {
	want := errors.New("multicast unavailable")
	if _, err := start(&net.TCPAddr{Port: 8080}, "host", func(registrationRequest) (Registration, error) {
		return nil, want
	}); !errors.Is(err, want) {
		t.Fatalf("start error = %v, want %v", err, want)
	}
}
