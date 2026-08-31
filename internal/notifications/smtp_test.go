package notifications_test

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
)

func TestSMTPSenderSpeaksSMTPAndClassifiesAcceptanceBoundary(t *testing.T) {
	message := notifications.EmailMessage{
		ToAddress: "person@example.com", ToName: "Person",
		Subject: "Welcome to Loomarr", TextBody: "Open the invitation link.",
		HTMLBody: "<p>Open the <a href=\"https://loomarr.example/join\">invitation link</a>.</p>",
	}

	for name, tc := range map[string]struct {
		behavior smtpBehavior
		want     notifications.EmailTransmissionState
	}{
		"accepted":           {smtpBehavior{}, notifications.EmailAccepted},
		"recipient rejected": {smtpBehavior{recipientCode: 550}, notifications.EmailRecipientRejected},
		"temporary rejection": {smtpBehavior{recipientCode: 450},
			notifications.EmailTransientPreAcceptance},
		"temporary data rejection": {smtpBehavior{dataCode: 450},
			notifications.EmailTransientPreAcceptance},
		"permanent data rejection": {smtpBehavior{dataCode: 550},
			notifications.EmailConfigurationRejected},
		"disconnect after data": {smtpBehavior{disconnectAfterData: true},
			notifications.EmailAcceptanceAmbiguous},
		"temporary greeting rejection": {smtpBehavior{greetingCode: 421},
			notifications.EmailTransientPreAcceptance},
		"permanent greeting rejection": {smtpBehavior{greetingCode: 535},
			notifications.EmailConfigurationRejected},
	} {
		t.Run(name, func(t *testing.T) {
			host, port, received := startSMTPServer(t, tc.behavior)
			config := notifications.EmailConfig{
				Enabled: true, Host: host, Port: port, Security: notifications.EmailSecurityNone,
				FromAddress: "loomarr@example.com", FromName: "Loomarr",
			}
			got := notifications.NewSMTPSender(2*time.Second).Send(t.Context(), config, message)
			if got.State != tc.want {
				t.Fatalf("state = %q, want %q", got.State, tc.want)
			}
			if tc.want != notifications.EmailAccepted {
				return
			}
			select {
			case raw := <-received:
				for _, part := range []string{"Subject: Welcome to Loomarr", "Content-Type: multipart/alternative", "text/plain", "text/html", "Open the invitation link."} {
					if !strings.Contains(raw, part) {
						t.Errorf("message omitted %q:\n%s", part, raw)
					}
				}
			case <-time.After(time.Second):
				t.Fatal("SMTP server did not receive DATA")
			}
		})
	}
}

func TestSMTPSenderRequiresSTARTTLSWithoutCleartextDowngrade(t *testing.T) {
	host, port, received := startSMTPServer(t, smtpBehavior{})
	config := notifications.EmailConfig{
		Enabled: true, Host: host, Port: port, Security: notifications.EmailSecuritySTARTTLS,
		FromAddress: "loomarr@example.com", FromName: "Loomarr",
	}
	message := notifications.EmailMessage{
		ToAddress: "person@example.com", Subject: "Security test",
		TextBody: "plain", HTMLBody: "<p>html</p>",
	}
	result := notifications.NewSMTPSender(time.Second).Send(t.Context(), config, message)
	if result.State == notifications.EmailAccepted {
		t.Fatal("STARTTLS policy downgraded to cleartext")
	}
	select {
	case raw := <-received:
		t.Fatalf("message DATA crossed a server that did not advertise STARTTLS: %s", raw)
	case <-time.After(100 * time.Millisecond):
	}
}

type smtpBehavior struct {
	greetingCode        int
	recipientCode       int
	dataCode            int
	disconnectAfterData bool
}

func startSMTPServer(t *testing.T, behavior smtpBehavior) (string, int, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	received := make(chan string, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(connection)
		writer := bufio.NewWriter(connection)
		greetingCode := behavior.greetingCode
		if greetingCode == 0 {
			greetingCode = 220
		}
		writeSMTPLine(writer, fmt.Sprintf("%d loomarr.test ESMTP", greetingCode))
		if greetingCode != 220 {
			return
		}
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			upper := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(upper, "EHLO "), strings.HasPrefix(upper, "HELO "):
				_, _ = writer.WriteString("250-loomarr.test\r\n250 8BITMIME\r\n")
				_ = writer.Flush()
			case upper == "NOOP", upper == "RSET":
				writeSMTPLine(writer, "250 OK")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				writeSMTPLine(writer, "250 Sender accepted")
			case strings.HasPrefix(upper, "RCPT TO:"):
				code := behavior.recipientCode
				if code == 0 {
					code = 250
				}
				writeSMTPLine(writer, fmt.Sprintf("%d recipient response", code))
			case upper == "DATA":
				writeSMTPLine(writer, "354 End data with <CR><LF>.<CR><LF>")
				var body strings.Builder
				for {
					dataLine, dataErr := reader.ReadString('\n')
					if dataErr != nil {
						return
					}
					if dataLine == ".\r\n" {
						break
					}
					body.WriteString(dataLine)
				}
				received <- body.String()
				if behavior.disconnectAfterData {
					return
				}
				code := behavior.dataCode
				if code == 0 {
					code = 250
				}
				writeSMTPLine(writer, fmt.Sprintf("%d 2.0.0 queued as loomarr-42", code))
			case upper == "QUIT":
				writeSMTPLine(writer, "221 Bye")
				return
			default:
				writeSMTPLine(writer, "500 Unsupported")
			}
		}
	}()
	host, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	return host, port, received
}

func writeSMTPLine(writer *bufio.Writer, line string) {
	_, _ = writer.WriteString(line + "\r\n")
	_ = writer.Flush()
}
