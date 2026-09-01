package notifications

import (
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type WebPushIdentity struct {
	PublicKey  string
	PrivateKey string
}

type WebPushAdapter struct {
	identity  WebPushIdentity
	client    HTTPDoer
	publicURL func() string
}

func NewWebPushAdapter(identity WebPushIdentity, client HTTPDoer, publicURL func() string) *WebPushAdapter {
	if client == nil {
		client = safeWebPushHTTPClient()
	}
	return &WebPushAdapter{identity: identity, client: client, publicURL: publicURL}
}

func (*WebPushAdapter) Means() Means { return MeansWebPush }

func (a *WebPushAdapter) ValidateDestination(_ map[string]string, credentials map[string]string) error {
	if a == nil || a.identity.PublicKey == "" || a.identity.PrivateKey == "" {
		return fmt.Errorf("Browser Push identity is unavailable")
	}
	endpoint, err := url.Parse(credentials["endpoint"])
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return fmt.Errorf("Browser Push requires an HTTPS subscription endpoint")
	}
	if address := net.ParseIP(endpoint.Hostname()); address != nil && !publicWebPushAddress(address) {
		return fmt.Errorf("Browser Push endpoint must use a public Push service")
	}
	publicKey, err := decodeWebPushValue(credentials["p256dh"])
	if err != nil || len(publicKey) != 65 {
		return fmt.Errorf("Browser Push public key is invalid")
	}
	x, _ := elliptic.Unmarshal(elliptic.P256(), publicKey)
	if x == nil {
		return fmt.Errorf("Browser Push public key is invalid")
	}
	auth, err := decodeWebPushValue(credentials["auth"])
	if err != nil || len(auth) < 16 || len(auth) > 64 {
		return fmt.Errorf("Browser Push authentication secret is invalid")
	}
	return nil
}

func safeWebPushHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("resolve Web Push service")
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(addresses) == 0 {
				return nil, fmt.Errorf("resolve Web Push service")
			}
			for _, candidate := range addresses {
				if !publicWebPushAddress(candidate.IP) {
					return nil, fmt.Errorf("Web Push service resolved to a non-public address")
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
		},
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func publicWebPushAddress(address net.IP) bool {
	return address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() &&
		!address.IsLinkLocalUnicast() && !address.IsLinkLocalMulticast()
}

func (a *WebPushAdapter) Deliver(ctx context.Context, delivery Delivery) Result {
	if a == nil || delivery.Destination == nil || delivery.Destination.Means != MeansWebPush ||
		a.ValidateDestination(delivery.Destination.Configuration, delivery.Destination.Credentials) != nil {
		return providerConfigurationFailure()
	}
	message := providerMessage(delivery.Intent, a.publicURL)
	payload, err := json.Marshal(map[string]any{
		"version": 1,
		"title":   "Loomarr",
		"body":    "You have a new Loomarr notification.",
		"url":     webPushPath(message.Link),
		"tag":     "loomarr-" + truncate(message.EventID, 100),
	})
	if err != nil {
		return providerConfigurationFailure()
	}
	credentials := delivery.Destination.Credentials
	response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: credentials["endpoint"],
		Keys:     webpush.Keys{P256dh: credentials["p256dh"], Auth: credentials["auth"]},
	}, &webpush.Options{
		HTTPClient: a.client, Subscriber: webPushSubscriber(a.publicURL), TTL: 300,
		Topic: "loomarr-" + truncate(string(message.EventType), 24), Urgency: webpush.UrgencyNormal,
		VAPIDPublicKey: a.identity.PublicKey, VAPIDPrivateKey: a.identity.PrivateKey,
	})
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Result{Status: StatusFailed, FailureClass: FailureCancelled, OutcomeCode: OutcomeCancelled}
		}
		return providerTransientFailure()
	}
	defer func() { _ = response.Body.Close() }()
	read, err := io.Copy(io.Discard, io.LimitReader(response.Body, providerResponseLimit+1))
	if err != nil || read > providerResponseLimit {
		return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return Result{Status: StatusDelivered}
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeDestinationUnavailable}
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeRecipientRejected}
	}
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooEarly ||
		response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		result := providerTransientFailure()
		result.RetryAfter = providerRetryAfter(response.Header.Get("Retry-After"), time.Now())
		return result
	}
	return providerConfigurationFailure()
}

func decodeWebPushValue(value string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

func webPushSubscriber(publicURL func() string) string {
	if publicURL != nil {
		if parsed, err := url.Parse(strings.TrimSpace(publicURL())); err == nil && parsed.Scheme == "https" && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}
	return "https://github.com/loomarr/loomarr"
}

func webPushPath(link string) string {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Path == "" || (!strings.HasPrefix(parsed.Path, "/queue/") &&
		!strings.HasPrefix(parsed.Path, "/channels/") && parsed.Path != "/") {
		return "/"
	}
	return parsed.EscapedPath()
}
