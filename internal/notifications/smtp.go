package notifications

import (
	"context"
	"errors"
	"net/textproto"
	"time"

	mail "github.com/wneessen/go-mail"
)

type SMTPSender struct {
	timeout time.Duration
}

func NewSMTPSender(timeout time.Duration) *SMTPSender {
	if timeout <= 0 {
		timeout = mail.DefaultTimeout
	}
	return &SMTPSender{timeout: timeout}
}

func (s *SMTPSender) Send(ctx context.Context, config EmailConfig, message EmailMessage) EmailTransmission {
	if err := config.Validate(); err != nil || !config.Enabled || message.Validate() != nil {
		return EmailTransmission{State: EmailConfigurationRejected}
	}

	clientOptions := []mail.Option{
		mail.WithPort(config.Port),
		mail.WithTimeout(s.timeout),
	}
	switch config.Security {
	case EmailSecuritySTARTTLS:
		clientOptions = append(clientOptions, mail.WithTLSPolicy(mail.TLSMandatory))
	case EmailSecurityTLS:
		clientOptions = append(clientOptions, mail.WithSSL())
	case EmailSecurityNone:
		clientOptions = append(clientOptions, mail.WithTLSPolicy(mail.NoTLS))
	}
	if config.Username != "" {
		clientOptions = append(clientOptions,
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
			mail.WithUsername(config.Username),
			mail.WithPassword(config.Password),
		)
	}
	client, err := mail.NewClient(config.Host, clientOptions...)
	if err != nil {
		return EmailTransmission{State: EmailConfigurationRejected}
	}

	msg := mail.NewMsg()
	if config.FromName == "" {
		err = msg.From(config.FromAddress)
	} else {
		err = msg.FromFormat(config.FromName, config.FromAddress)
	}
	if err == nil {
		if message.ToName == "" {
			err = msg.To(message.ToAddress)
		} else {
			err = msg.AddToFormat(message.ToName, message.ToAddress)
		}
	}
	if err != nil {
		return EmailTransmission{State: EmailConfigurationRejected}
	}
	msg.Subject(message.Subject)
	msg.SetMessageID()
	msg.SetDate()
	msg.SetBodyString(mail.ContentType("text/plain"), message.TextBody)
	msg.AddAlternativeString(mail.ContentType("text/html"), message.HTMLBody)

	err = client.DialAndSendWithContext(ctx, msg)
	if msg.IsDelivered() {
		return EmailTransmission{State: EmailAccepted, ProviderMessageID: msg.GetMessageID()}
	}
	if ctx.Err() != nil {
		return EmailTransmission{State: EmailCancelled}
	}
	var sendError *mail.SendError
	if errors.As(err, &sendError) {
		switch sendError.Reason {
		case mail.ErrSMTPDataClose:
			if sendError.IsTemp() || sendError.ErrorCode()/100 == 4 {
				return EmailTransmission{State: EmailTransientPreAcceptance}
			}
			if sendError.ErrorCode()/100 == 5 {
				return EmailTransmission{State: EmailConfigurationRejected}
			}
			return EmailTransmission{State: EmailAcceptanceAmbiguous}
		case mail.ErrSMTPRcptTo:
			if sendError.IsTemp() || sendError.ErrorCode()/100 == 4 {
				return EmailTransmission{State: EmailTransientPreAcceptance}
			}
			return EmailTransmission{State: EmailRecipientRejected}
		case mail.ErrGetSender, mail.ErrGetRcpts, mail.ErrNoUnencoded:
			return EmailTransmission{State: EmailConfigurationRejected}
		default:
			if sendError.IsTemp() {
				return EmailTransmission{State: EmailTransientPreAcceptance}
			}
			return EmailTransmission{State: EmailConfigurationRejected}
		}
	}
	var protocolError *textproto.Error
	if errors.As(err, &protocolError) {
		switch protocolError.Code / 100 {
		case 4:
			return EmailTransmission{State: EmailTransientPreAcceptance}
		case 5:
			return EmailTransmission{State: EmailConfigurationRejected}
		}
	}
	// Dial, greeting, STARTTLS, and authentication failures all precede acceptance. Most are
	// operationally transient; repeated failures remain bounded by the notification retry policy.
	return EmailTransmission{State: EmailTransientPreAcceptance}
}
