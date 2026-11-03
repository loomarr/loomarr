package notifications_test

import (
	"net/http"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/loomarr/loomarr/internal/notifications"
)

// statusLadder is the delivery outcome of every non-2xx class a provider can answer with: a
// rejected credential or a vanished destination never succeeds on retry, a busy or failing
// service does. An Apprise 5xx is ambiguous instead (it may already have fanned out).
var statusLadder = []struct {
	status int
	want   notifications.FailureClass
	code   notifications.OutcomeCode
}{
	{http.StatusUnauthorized, notifications.FailurePermanent, notifications.OutcomeRecipientRejected},
	{http.StatusForbidden, notifications.FailurePermanent, notifications.OutcomeRecipientRejected},
	{http.StatusNotFound, notifications.FailurePermanent, notifications.OutcomeDestinationUnavailable},
	{http.StatusGone, notifications.FailurePermanent, notifications.OutcomeDestinationUnavailable},
	{http.StatusBadRequest, notifications.FailurePermanent, notifications.OutcomeConfigurationInvalid},
	{http.StatusTeapot, notifications.FailurePermanent, notifications.OutcomeConfigurationInvalid},
	{http.StatusFound, notifications.FailurePermanent, notifications.OutcomeConfigurationInvalid},
	{http.StatusRequestTimeout, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
	{http.StatusTooEarly, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
	{http.StatusTooManyRequests, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
	{http.StatusInternalServerError, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
	{http.StatusBadGateway, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
	// A retry cannot make a service implement the method or the HTTP version.
	{http.StatusNotImplemented, notifications.FailurePermanent, notifications.OutcomeConfigurationInvalid},
	{http.StatusHTTPVersionNotSupported, notifications.FailurePermanent, notifications.OutcomeConfigurationInvalid},
	{http.StatusInsufficientStorage, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
	// A CDN in front of a webhook answers 52x for an origin that is briefly down.
	{522, notifications.FailureTransientPreAcceptance, notifications.OutcomeTransportUnavailable},
}

func TestHTTPProviderStatusLadder(t *testing.T) {
	t.Parallel()
	for _, tt := range statusLadder {
		doer := &providerDoer{status: tt.status, response: "details must not escape"}
		adapter := httpAdapter(t, notifications.MeansSlack, doer)
		destination := providerDestination(notifications.MeansSlack, nil, map[string]string{
			"webhookUrl": "https://hooks.slack.com/services/T/B/secret",
		})
		result := adapter.Deliver(t.Context(), providerDelivery(&destination))
		if result.Status != notifications.StatusFailed || result.FailureClass != tt.want || result.OutcomeCode != tt.code {
			t.Errorf("status %d result = %+v, want %v/%v", tt.status, result, tt.want, tt.code)
		}
	}
}

func TestAppriseServerErrorsStayAmbiguousButClientErrorsClassifyAsUsual(t *testing.T) {
	t.Parallel()
	for _, tt := range statusLadder {
		want, code := tt.want, tt.code
		if tt.status >= 500 {
			want, code = notifications.FailureAmbiguous, notifications.OutcomeAcceptanceAmbiguous
		}
		doer := &providerDoer{status: tt.status, response: "details must not escape"}
		adapter := httpAdapter(t, notifications.MeansApprise, doer)
		destination := providerDestination(notifications.MeansApprise,
			map[string]string{"baseUrl": "https://apprise.example.test"},
			map[string]string{"configurationKey": "home"})
		result := adapter.Deliver(t.Context(), providerDelivery(&destination))
		if result.FailureClass != want || result.OutcomeCode != code {
			t.Errorf("apprise status %d result = %+v, want %v/%v", tt.status, result, want, code)
		}
	}
}

func TestWebPushStatusLadder(t *testing.T) {
	t.Parallel()
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range statusLadder {
		doer := &providerDoer{status: tt.status, response: "details must not escape"}
		adapter := notifications.NewWebPushAdapter(notifications.WebPushIdentity{
			PublicKey: publicKey, PrivateKey: privateKey,
		}, doer, nil)
		destination := providerDestination(notifications.MeansWebPush, nil, validWebPushCredentials(t))
		result := adapter.Deliver(t.Context(), providerDelivery(&destination))
		if result.Status != notifications.StatusFailed || result.FailureClass != tt.want || result.OutcomeCode != tt.code {
			t.Errorf("web push status %d result = %+v, want %v/%v", tt.status, result, tt.want, tt.code)
		}
	}
}
