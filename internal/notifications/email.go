package notifications

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"net/mail"
	"net/url"
	"strings"
	texttemplate "text/template"
	"time"
)

type EmailSecurity string

const (
	EmailSecuritySTARTTLS EmailSecurity = "starttls"
	EmailSecurityTLS      EmailSecurity = "tls"
	EmailSecurityNone     EmailSecurity = "none"
)

type EmailConfig struct {
	Enabled     bool
	Host        string
	Port        int
	Security    EmailSecurity
	Username    string
	Password    string
	FromAddress string
	FromName    string
}

func (c EmailConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("email delivery requires an SMTP host")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("email delivery requires an SMTP port from 1 through 65535")
	}
	switch c.Security {
	case EmailSecuritySTARTTLS, EmailSecurityTLS, EmailSecurityNone:
	default:
		return fmt.Errorf("email delivery has an invalid SMTP security policy")
	}
	if c.Username == "" && c.Password != "" {
		return fmt.Errorf("an SMTP password requires a username")
	}
	if err := validateBareMailbox(c.FromAddress); err != nil {
		return fmt.Errorf("email delivery requires a valid sender address: %w", err)
	}
	if err := safeText("sender name", c.FromName, 200); err != nil {
		return err
	}
	return nil
}

type EmailMessage struct {
	ToAddress string
	ToName    string
	Subject   string
	TextBody  string
	HTMLBody  string
}

type EmailContent struct {
	Subject  string
	TextBody string
	HTMLBody string
}

func RenderAccountEmail(intent Intent, actionURL string, expiresAt time.Time) (EmailContent, error) {
	parsed, err := url.Parse(actionURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return EmailContent{}, fmt.Errorf("account email requires an absolute HTTP or HTTPS action URL")
	}
	if expiresAt.IsZero() {
		return EmailContent{}, fmt.Errorf("account email requires an expiry")
	}
	var subject, heading, description, action string
	switch intent.Topic {
	case TopicAccountInvitation:
		subject = "You're invited to Loomarr"
		heading = "You're invited"
		description = "An administrator invited you to join Loomarr."
		action = "Open your invitation"
	case TopicLocalPasswordRecovery:
		subject = "Reset your Loomarr password"
		heading = "Reset your password"
		description = "A password reset was requested for your Loomarr account."
		action = "Reset your password"
	default:
		return EmailContent{}, fmt.Errorf("unsupported account notification topic %q", intent.Topic)
	}
	data := accountEmailData{
		Name: intent.Template.RecipientName, Heading: heading, Description: description,
		ActionText: action, URL: actionURL,
		Expires: expiresAt.UTC().Format("January 2, 2006 at 3:04 PM MST"),
	}
	textBody, err := executeTextTemplate(accountEmailText, data)
	if err != nil {
		return EmailContent{}, fmt.Errorf("render account email text: %w", err)
	}
	htmlBody, err := executeHTMLTemplate(accountEmailHTML, data)
	if err != nil {
		return EmailContent{}, fmt.Errorf("render account email HTML: %w", err)
	}
	return EmailContent{Subject: subject, TextBody: textBody, HTMLBody: htmlBody}, nil
}

type accountEmailData struct {
	Name        string
	Heading     string
	Description string
	ActionText  string
	URL         string
	Expires     string
}

const accountEmailText = `{{if .Name}}Hello {{.Name}},

{{end}}{{.Description}}

{{.ActionText}}:
{{.URL}}

This link expires on {{.Expires}}.

If you weren't expecting this message, you can safely ignore it.
`

const accountEmailHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>{{.Heading}}</title></head>
<body>
<main>
{{if .Name}}<p>Hello {{.Name}},</p>{{end}}
<h1>{{.Heading}}</h1>
<p>{{.Description}}</p>
<p><a href="{{.URL}}">{{.ActionText}}</a></p>
<p>This link expires on {{.Expires}}.</p>
<p>If you weren't expecting this message, you can safely ignore it.</p>
</main>
</body>
</html>
`

func executeTextTemplate(source string, data accountEmailData) (string, error) {
	tpl, err := texttemplate.New("account-email").Parse(source)
	if err != nil {
		return "", err
	}
	var rendered bytes.Buffer
	if err := tpl.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func executeHTMLTemplate(source string, data accountEmailData) (string, error) {
	tpl, err := htmltemplate.New("account-email").Parse(source)
	if err != nil {
		return "", err
	}
	var rendered bytes.Buffer
	if err := tpl.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func (m EmailMessage) Validate() error {
	if err := validateBareMailbox(m.ToAddress); err != nil {
		return fmt.Errorf("invalid email destination: %w", err)
	}
	if err := safeText("recipient name", m.ToName, 200); err != nil {
		return err
	}
	if err := safeText("email subject", m.Subject, 200); err != nil {
		return err
	}
	if m.Subject == "" || strings.TrimSpace(m.TextBody) == "" || strings.TrimSpace(m.HTMLBody) == "" {
		return fmt.Errorf("email requires subject, plain-text body, and HTML body")
	}
	return nil
}

func validateBareMailbox(raw string) error {
	raw = strings.TrimSpace(raw)
	address, err := mail.ParseAddress(raw)
	if err != nil || raw == "" || address.Name != "" || address.Address != raw {
		return fmt.Errorf("want one mailbox without a display name")
	}
	return nil
}

var ErrEmailDestinationUnavailable = errors.New("notifications: email destination unavailable")

type EmailMaterializer interface {
	Materialize(context.Context, Delivery) (MaterializedEmail, error)
}

// MaterializedEmail exists only for one claimed attempt. Invalidate revokes any bearer grant
// minted while rendering when SMTP proves the message was not accepted. Neither the message nor
// the callback crosses the durable notification boundary.
type MaterializedEmail struct {
	Message    EmailMessage
	Invalidate func(context.Context) error
}

type EmailTransmissionState string

const (
	EmailAccepted               EmailTransmissionState = "accepted"
	EmailTransientPreAcceptance EmailTransmissionState = "transient_pre_acceptance"
	EmailRecipientRejected      EmailTransmissionState = "recipient_rejected"
	EmailConfigurationRejected  EmailTransmissionState = "configuration_rejected"
	EmailAcceptanceAmbiguous    EmailTransmissionState = "acceptance_ambiguous"
	EmailCancelled              EmailTransmissionState = "cancelled"
)

type EmailTransmission struct {
	State             EmailTransmissionState
	ProviderMessageID string
}

type EmailSender interface {
	Send(context.Context, EmailConfig, EmailMessage) EmailTransmission
}

type EmailAdapter struct {
	config       func() EmailConfig
	materializer EmailMaterializer
	sender       EmailSender
}

func NewEmailAdapter(config func() EmailConfig, materializer EmailMaterializer, sender EmailSender) *EmailAdapter {
	return &EmailAdapter{config: config, materializer: materializer, sender: sender}
}

func (*EmailAdapter) Means() Means { return MeansEmail }

func (a *EmailAdapter) Deliver(ctx context.Context, delivery Delivery) Result {
	if a == nil || a.config == nil {
		return emailConfigurationFailure()
	}
	config := a.config()
	if !config.Enabled {
		return Result{Status: StatusSuppressed, OutcomeCode: OutcomeDeliveryDisabled}
	}
	if err := config.Validate(); err != nil || a.materializer == nil || a.sender == nil {
		return emailConfigurationFailure()
	}
	materialized, err := a.materializer.Materialize(ctx, delivery)
	if errors.Is(err, ErrEmailDestinationUnavailable) {
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeDestinationUnavailable}
	}
	if err != nil {
		return emailConfigurationFailure()
	}
	if materialized.Message.Validate() != nil {
		if materialized.Invalidate != nil {
			if err := materialized.Invalidate(ctx); err != nil {
				return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
			}
		}
		return emailConfigurationFailure()
	}
	transmission := a.sender.Send(ctx, config, materialized.Message)
	if emailDefinitelyNotAccepted(transmission.State) && materialized.Invalidate != nil {
		if err := materialized.Invalidate(ctx); err != nil {
			return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
		}
	}
	return resultForEmailTransmission(transmission)
}

func emailDefinitelyNotAccepted(state EmailTransmissionState) bool {
	return state == EmailTransientPreAcceptance || state == EmailRecipientRejected ||
		state == EmailConfigurationRejected
}

func (a *EmailAdapter) SendTest(ctx context.Context, destination string) Result {
	if a == nil || a.config == nil {
		return emailConfigurationFailure()
	}
	config := a.config()
	if !config.Enabled {
		return Result{Status: StatusSuppressed, OutcomeCode: OutcomeDeliveryDisabled}
	}
	if err := config.Validate(); err != nil || a.sender == nil {
		return emailConfigurationFailure()
	}
	message := EmailMessage{
		ToAddress: destination,
		Subject:   "Loomarr test email",
		TextBody:  "Loomarr email delivery is configured correctly.\n",
		HTMLBody:  `<!doctype html><html lang="en"><body><main><p>Loomarr email delivery is configured correctly.</p></main></body></html>`,
	}
	if err := message.Validate(); err != nil {
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeDestinationUnavailable}
	}
	return resultForEmailTransmission(a.sender.Send(ctx, config, message))
}

func resultForEmailTransmission(transmission EmailTransmission) Result {
	switch transmission.State {
	case EmailAccepted:
		return Result{Status: StatusDelivered, ProviderMessageID: transmission.ProviderMessageID}
	case EmailTransientPreAcceptance:
		return Result{Status: StatusFailed, FailureClass: FailureTransientPreAcceptance, OutcomeCode: OutcomeTransportUnavailable}
	case EmailRecipientRejected:
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeRecipientRejected}
	case EmailConfigurationRejected:
		return emailConfigurationFailure()
	case EmailAcceptanceAmbiguous:
		return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
	case EmailCancelled:
		return Result{Status: StatusFailed, FailureClass: FailureCancelled, OutcomeCode: OutcomeCancelled}
	default:
		return emailConfigurationFailure()
	}
}

func emailConfigurationFailure() Result {
	return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeConfigurationInvalid}
}
