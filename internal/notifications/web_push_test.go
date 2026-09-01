package notifications_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/loomarr/loomarr/internal/notifications"
)

func TestWebPushAdapterEncryptsLowDetailPayloadAndUsesVAPID(t *testing.T) {
	t.Parallel()
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	credentials := validWebPushCredentials(t)
	doer := &providerDoer{status: http.StatusCreated, response: "provider response is discarded"}
	adapter := notifications.NewWebPushAdapter(notifications.WebPushIdentity{
		PublicKey: publicKey, PrivateKey: privateKey,
	}, doer, func() string { return "https://loomarr.example.test" })
	destination := providerDestination(notifications.MeansWebPush, nil, credentials)
	destination.Scope = notifications.ScopePerson
	destination.OwnerID = "user-1"
	destination.Audience = notifications.RecipientPerson
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.Status != notifications.StatusDelivered {
		t.Fatalf("result = %+v", result)
	}
	if doer.request.url != credentials["endpoint"] || doer.request.method != http.MethodPost ||
		doer.request.header.Get("Content-Encoding") != "aes128gcm" ||
		!strings.HasPrefix(doer.request.header.Get("Authorization"), "vapid ") {
		t.Fatalf("request = %+v", doer.request)
	}
	if strings.Contains(doer.request.body, "Channel") || strings.Contains(doer.request.body, "attention") ||
		strings.Contains(doer.request.body, credentials["auth"]) {
		t.Fatalf("encrypted request exposed preview or subscription secret: %q", doer.request.body)
	}
}

func TestWebPushAdapterRetiresGoneSubscriptionsWithoutRetry(t *testing.T) {
	t.Parallel()
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	doer := &providerDoer{status: http.StatusGone, response: "gone"}
	adapter := notifications.NewWebPushAdapter(notifications.WebPushIdentity{
		PublicKey: publicKey, PrivateKey: privateKey,
	}, doer, nil)
	destination := providerDestination(notifications.MeansWebPush, nil, validWebPushCredentials(t))
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.FailureClass != notifications.FailurePermanent ||
		result.OutcomeCode != notifications.OutcomeDestinationUnavailable {
		t.Fatalf("result = %+v", result)
	}
}

func TestWebPushAdapterBoundsResponsesAndRateLimitHints(t *testing.T) {
	t.Parallel()
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	doer := &providerDoer{
		status: http.StatusTooManyRequests, response: strings.Repeat("x", 65<<10),
		header: http.Header{"Retry-After": []string{"30"}},
	}
	adapter := notifications.NewWebPushAdapter(notifications.WebPushIdentity{
		PublicKey: publicKey, PrivateKey: privateKey,
	}, doer, nil)
	destination := providerDestination(notifications.MeansWebPush, nil, validWebPushCredentials(t))
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.FailureClass != notifications.FailureAmbiguous || result.RetryAfter != 0 {
		t.Fatalf("oversized response result = %+v", result)
	}

	doer.response = "slow down"
	result = adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.FailureClass != notifications.FailureTransientPreAcceptance || result.RetryAfter != 30*time.Second {
		t.Fatalf("rate-limit result = %+v", result)
	}
}

func TestWebPushValidationRejectsNonHTTPSOrInvalidSubscriptionKeys(t *testing.T) {
	t.Parallel()
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	adapter := notifications.NewWebPushAdapter(notifications.WebPushIdentity{
		PublicKey: publicKey, PrivateKey: privateKey,
	}, &providerDoer{}, nil)
	credentials := validWebPushCredentials(t)
	credentials["endpoint"] = "http://push.example.test/subscription"
	if err := adapter.ValidateDestination(nil, credentials); err == nil {
		t.Fatal("accepted a non-HTTPS Push endpoint")
	}
	credentials = validWebPushCredentials(t)
	credentials["endpoint"] = "https://127.0.0.1/subscription"
	if err := adapter.ValidateDestination(nil, credentials); err == nil {
		t.Fatal("accepted a private Push endpoint")
	}
	credentials = validWebPushCredentials(t)
	credentials["p256dh"] = base64.RawURLEncoding.EncodeToString([]byte("not-a-P256-point"))
	if err := adapter.ValidateDestination(nil, credentials); err == nil {
		t.Fatal("accepted an invalid browser public key")
	}
}

func validWebPushCredentials(t *testing.T) map[string]string {
	t.Helper()
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, auth); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"endpoint": "https://push.example.test/subscription/secret",
		"p256dh":   base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()),
		"auth":     base64.RawURLEncoding.EncodeToString(auth),
	}
}
