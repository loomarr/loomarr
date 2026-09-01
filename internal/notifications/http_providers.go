package notifications

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	providerResponseLimit = 64 << 10
	providerBodyLimit     = 8 << 10
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ProviderMessage struct {
	EventID   string
	EventType Topic
	Occurred  time.Time
	Severity  string
	Title     string
	Body      string
	Link      string
}

type providerRequest struct {
	method  string
	url     string
	headers http.Header
	body    []byte
	accept  func([]byte) (string, bool)
}

type HTTPProviderAdapter struct {
	means     Means
	client    HTTPDoer
	publicURL func() string
}

func NewHTTPProviderAdapters(client HTTPDoer, publicURL func() string) ([]Adapter, []DestinationValidator) {
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 3 || len(via) == 0 ||
					!strings.EqualFold(request.URL.Hostname(), via[0].URL.Hostname()) ||
					(via[0].URL.Scheme == "https" && request.URL.Scheme != "https") {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
	}
	means := []Means{
		MeansWebhook, MeansDiscord, MeansNtfy, MeansGotify, MeansApprise, MeansPushover,
		MeansTelegram, MeansMattermost, MeansMatrix, MeansSlack,
	}
	adapters := make([]Adapter, 0, len(means))
	validators := make([]DestinationValidator, 0, len(means))
	for _, value := range means {
		adapter := &HTTPProviderAdapter{means: value, client: client, publicURL: publicURL}
		adapters = append(adapters, adapter)
		validators = append(validators, adapter)
	}
	return adapters, validators
}

func (a *HTTPProviderAdapter) Means() Means { return a.means }

func (a *HTTPProviderAdapter) ValidateDestination(configuration, credentials map[string]string) error {
	definition, ok := ProviderDefinitionFor(a.means)
	if !ok {
		return fmt.Errorf("notification provider is unsupported")
	}
	for _, field := range definition.Fields {
		value := configuration[field.Key]
		if field.Sensitive {
			value = credentials[field.Key]
		}
		if field.Required && strings.TrimSpace(value) == "" {
			return fmt.Errorf("notification provider requires %s", field.Label)
		}
	}
	message := ProviderMessage{
		EventID: "validation", EventType: TopicDeliveryTest, Occurred: time.Unix(1, 0).UTC(),
		Severity: "info", Title: "Loomarr test", Body: "Loomarr notification provider test.",
	}
	_, err := buildProviderRequest(a.means, configuration, credentials, message)
	return err
}

func (a *HTTPProviderAdapter) Deliver(ctx context.Context, delivery Delivery) Result {
	if a == nil || a.client == nil || delivery.Destination == nil || delivery.Destination.Means != a.means {
		return providerConfigurationFailure()
	}
	destination := delivery.Destination
	if err := a.ValidateDestination(destination.Configuration, destination.Credentials); err != nil {
		return providerConfigurationFailure()
	}
	message := providerMessage(delivery.Intent, a.publicURL)
	spec, err := buildProviderRequest(a.means, destination.Configuration, destination.Credentials, message)
	if err != nil {
		return providerConfigurationFailure()
	}
	req, err := http.NewRequestWithContext(ctx, spec.method, spec.url, bytes.NewReader(spec.body))
	if err != nil {
		return providerConfigurationFailure()
	}
	req.Header = spec.headers.Clone()
	req.Header.Set("User-Agent", "Loomarr/notification-provider")
	response, err := a.client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Result{Status: StatusFailed, FailureClass: FailureCancelled, OutcomeCode: OutcomeCancelled}
		}
		return providerTransientFailure()
	}
	defer func() { _ = response.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, providerResponseLimit+1))
	if readErr != nil || len(body) > providerResponseLimit {
		return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if a.means == MeansApprise && response.StatusCode == http.StatusNoContent {
			return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeDestinationUnavailable}
		}
		providerID := ""
		if spec.accept != nil {
			var accepted bool
			providerID, accepted = spec.accept(body)
			if !accepted {
				return providerConfigurationFailure()
			}
		}
		return Result{Status: StatusDelivered, ProviderMessageID: truncate(providerID, 200)}
	}
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooEarly ||
		response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		// Apprise may fan out before reporting that one downstream target failed. Retrying a 5xx
		// could therefore duplicate already accepted messages, so its server failures are ambiguous.
		if a.means == MeansApprise && response.StatusCode >= 500 {
			return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
		}
		result := providerTransientFailure()
		result.RetryAfter = providerRetryAfter(response.Header.Get("Retry-After"), time.Now())
		return result
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeDestinationUnavailable}
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeRecipientRejected}
	}
	return providerConfigurationFailure()
}

func providerMessage(intent Intent, publicURL func() string) ProviderMessage {
	title := "Loomarr: " + topicLabel(intent.Topic)
	body := strings.TrimSpace(intent.Template.SubjectName)
	if summary := strings.TrimSpace(intent.Template.Summary); summary != "" {
		if body != "" {
			body += " — "
		}
		body += summary
	}
	if body == "" {
		body = "Loomarr notification provider test."
	}
	link := ""
	if publicURL != nil {
		link = notificationLink(strings.TrimRight(publicURL(), "/"), intent)
	}
	return ProviderMessage{
		EventID: intent.ID, EventType: intent.Topic, Occurred: intent.CreatedAt.UTC(),
		Severity: topicSeverity(intent.Topic), Title: truncate(title, 250),
		Body: truncate(safeMentionText(body), 4000), Link: truncate(link, 512),
	}
}

func notificationLink(base string, intent Intent) string {
	if base == "" {
		return ""
	}
	switch intent.ReferenceKind {
	case ReferenceProposal:
		return base + "/queue/approval"
	case ReferenceChannel:
		return base + "/channels/" + url.PathEscape(intent.ReferenceID)
	case ReferenceTitle:
		return base + "/queue/flight"
	default:
		return base
	}
}

func topicLabel(topic Topic) string {
	switch topic {
	case TopicProposalSubmitted:
		return "Proposal submitted"
	case TopicProposalApproved:
		return "Proposal approved"
	case TopicProposalDeclined:
		return "Proposal declined"
	case TopicAcquisitionAvailable:
		return "Title available"
	case TopicAcquisitionGaveUp:
		return "Acquisition needs attention"
	case TopicChannelLive:
		return "Channel live"
	case TopicChannelDegraded:
		return "Channel degraded"
	case TopicDeliveryTest:
		return "Test notification"
	default:
		return "Notification"
	}
}

func topicSeverity(topic Topic) string {
	if topic == TopicProposalDeclined || topic == TopicAcquisitionGaveUp || topic == TopicChannelDegraded {
		return "warning"
	}
	return "info"
}

func buildProviderRequest(
	means Means,
	configuration, credentials map[string]string,
	message ProviderMessage,
) (providerRequest, error) {
	switch means {
	case MeansWebhook:
		return webhookRequest(configuration, credentials, message)
	case MeansDiscord:
		return discordRequest(credentials, message)
	case MeansNtfy:
		return ntfyRequest(configuration, credentials, message)
	case MeansGotify:
		return gotifyRequest(configuration, credentials, message)
	case MeansApprise:
		return appriseRequest(configuration, credentials, message)
	case MeansPushover:
		return pushoverRequest(credentials, message)
	case MeansTelegram:
		return telegramRequest(credentials, message)
	case MeansMattermost:
		return mattermostRequest(credentials, message)
	case MeansMatrix:
		return matrixRequest(configuration, credentials, message)
	case MeansSlack:
		return slackRequest(credentials, message)
	default:
		return providerRequest{}, fmt.Errorf("notification provider is unsupported")
	}
}

func webhookRequest(configuration, credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	target, err := providerURL(credentials["url"], false, nil)
	if err != nil {
		return providerRequest{}, err
	}
	payload := providerEventPayload(message)
	body, err := json.Marshal(payload)
	if err != nil {
		return providerRequest{}, fmt.Errorf("encode notification payload")
	}
	headers := jsonHeaders()
	headers.Set("X-Loomarr-Event-ID", message.EventID)
	if token := credentials["bearerToken"]; token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	if secret := credentials["hmacSecret"]; secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		headers.Set("X-Loomarr-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	return post(target, headers, body), nil
}

func providerEventPayload(message ProviderMessage) map[string]any {
	payload := map[string]any{
		"version": 1, "eventId": message.EventID, "eventType": message.EventType,
		"occurredAt": message.Occurred.Format(time.RFC3339), "severity": message.Severity,
		"subject": message.Title, "summary": message.Body,
	}
	if message.Link != "" {
		payload["link"] = message.Link
	}
	return payload
}

func discordRequest(credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	target, err := providerURL(credentials["webhookUrl"], true, []string{"discord.com", "discordapp.com"})
	if err != nil || !strings.Contains(target.Path, "/api/webhooks/") {
		return providerRequest{}, fmt.Errorf("Discord requires an official incoming webhook URL")
	}
	content := safeMentionText(message.Title + "\n" + message.Body)
	if message.Link != "" {
		content += "\n" + message.Link
	}
	body, _ := json.Marshal(map[string]any{
		"content":          truncate(content, 2000),
		"allowed_mentions": map[string]any{"parse": []string{}},
	})
	return post(target, jsonHeaders(), body), nil
}

func slackRequest(credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	target, err := providerURL(credentials["webhookUrl"], true, []string{"hooks.slack.com", "hooks.slack-gov.com"})
	if err != nil || !strings.HasPrefix(target.Path, "/services/") {
		return providerRequest{}, fmt.Errorf("Slack requires an official incoming webhook URL")
	}
	text := safeMentionText(message.Title + "\n" + message.Body)
	if message.Link != "" {
		text += "\n" + message.Link
	}
	body, _ := json.Marshal(map[string]any{
		"text": truncate(text, 3000),
		"blocks": []any{map[string]any{
			"type": "section", "text": map[string]string{"type": "plain_text", "text": truncate(text, 3000)},
		}},
	})
	return post(target, jsonHeaders(), body), nil
}

func mattermostRequest(credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	target, err := providerURL(credentials["webhookUrl"], false, nil)
	if err != nil || !strings.Contains(target.Path, "/hooks/") {
		return providerRequest{}, fmt.Errorf("Mattermost requires an incoming webhook URL")
	}
	text := safeMentionText(message.Title + "\n" + message.Body)
	if message.Link != "" {
		text += "\n" + message.Link
	}
	body, _ := json.Marshal(map[string]string{"text": truncate(text, 16000)})
	return post(target, jsonHeaders(), body), nil
}

func ntfyRequest(configuration, credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	base, err := providerURL(configuration["baseUrl"], false, nil)
	if err != nil {
		return providerRequest{}, err
	}
	base = base.JoinPath(credentials["topic"])
	body := []byte(truncate(message.Body, providerBodyLimit))
	headers := http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}}
	headers.Set("Title", truncate(message.Title, 250))
	if message.Severity == "warning" {
		headers.Set("Priority", "4")
		headers.Set("Tags", "warning")
	} else {
		headers.Set("Priority", "3")
		headers.Set("Tags", "tv")
	}
	if message.Link != "" {
		headers.Set("Click", message.Link)
	}
	if password := credentials["password"]; password != "" {
		if username := configuration["username"]; username != "" {
			headers.Set("Authorization", "Basic "+basicAuth(username, password))
		} else {
			headers.Set("Authorization", "Bearer "+password)
		}
	}
	return post(base, headers, body), nil
}

func gotifyRequest(configuration, credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	target, err := providerURL(configuration["serverUrl"], false, nil)
	if err != nil {
		return providerRequest{}, err
	}
	target.Path = strings.TrimRight(target.Path, "/") + "/message"
	headers := jsonHeaders()
	headers.Set("X-Gotify-Key", credentials["applicationToken"])
	priority := 2
	if message.Severity == "warning" {
		priority = 5
	}
	payload := map[string]any{
		"title": truncate(message.Title, 250), "message": truncate(joinBodyLink(message), 4000), "priority": priority,
	}
	if message.Link != "" {
		payload["extras"] = map[string]any{
			"client::notification": map[string]any{"click": map[string]string{"url": message.Link}},
		}
	}
	body, _ := json.Marshal(payload)
	return post(target, headers, body), nil
}

func appriseRequest(configuration, credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	base, err := providerURL(configuration["baseUrl"], false, nil)
	if err != nil {
		return providerRequest{}, err
	}
	key, destination := credentials["configurationKey"], credentials["destinationUrl"]
	if (key == "") == (destination == "") {
		return providerRequest{}, fmt.Errorf("Apprise requires either a configuration key or destination URL")
	}
	if key != "" {
		base.Path = strings.TrimRight(base.Path, "/") + "/notify/" + url.PathEscape(key)
	} else {
		base.Path = strings.TrimRight(base.Path, "/") + "/notify"
	}
	payload := map[string]any{
		"title": truncate(message.Title, 250), "body": truncate(joinBodyLink(message), 4000),
		"type": map[string]string{"warning": "warning", "info": "info"}[message.Severity],
	}
	if destination != "" {
		payload["urls"] = destination
	}
	body, _ := json.Marshal(payload)
	headers := jsonHeaders()
	if token := credentials["token"]; token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	return post(base, headers, body), nil
}

func pushoverRequest(credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	values := url.Values{
		"token": {credentials["applicationToken"]}, "user": {credentials["recipientKey"]},
		"title": {truncate(message.Title, 250)}, "message": {truncate(message.Body, 1024)},
		"priority": {map[string]string{"warning": "1", "info": "0"}[message.Severity]},
	}
	if device := credentials["device"]; device != "" {
		values.Set("device", device)
	}
	if message.Link != "" {
		values.Set("url", message.Link)
		values.Set("url_title", "Open Loomarr")
	}
	target, _ := url.Parse("https://api.pushover.net/1/messages.json")
	spec := post(target, http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}}, []byte(values.Encode()))
	spec.accept = func(body []byte) (string, bool) {
		var response struct {
			Status  int    `json:"status"`
			Request string `json:"request"`
		}
		return response.Request, json.Unmarshal(body, &response) == nil && response.Status == 1
	}
	return spec, nil
}

func telegramRequest(credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	token := credentials["botToken"]
	if token == "" || strings.ContainsAny(token, "/?#") {
		return providerRequest{}, fmt.Errorf("Telegram requires a valid bot token")
	}
	target, _ := url.Parse("https://api.telegram.org/bot" + token + "/sendMessage")
	payload := map[string]any{
		"chat_id": credentials["chatId"], "text": truncate(joinBodyLink(message), 4096),
		"link_preview_options": map[string]bool{"is_disabled": true},
	}
	if thread := credentials["threadId"]; thread != "" {
		value, err := strconv.ParseInt(thread, 10, 64)
		if err != nil {
			return providerRequest{}, fmt.Errorf("Telegram thread ID must be a number")
		}
		payload["message_thread_id"] = value
	}
	body, _ := json.Marshal(payload)
	spec := post(target, jsonHeaders(), body)
	spec.accept = func(body []byte) (string, bool) {
		var response struct {
			OK     bool `json:"ok"`
			Result struct {
				MessageID int64 `json:"message_id"`
			} `json:"result"`
		}
		if json.Unmarshal(body, &response) != nil || !response.OK {
			return "", false
		}
		return strconv.FormatInt(response.Result.MessageID, 10), true
	}
	return spec, nil
}

func matrixRequest(configuration, credentials map[string]string, message ProviderMessage) (providerRequest, error) {
	base, err := providerURL(configuration["homeserverUrl"], false, nil)
	if err != nil {
		return providerRequest{}, err
	}
	room := credentials["roomId"]
	if room == "" {
		return providerRequest{}, fmt.Errorf("Matrix requires a room ID")
	}
	base = base.JoinPath("_matrix", "client", "v3", "rooms", room, "send", "m.room.message", message.EventID)
	body, _ := json.Marshal(map[string]string{"msgtype": "m.text", "body": truncate(joinBodyLink(message), 4000)})
	headers := jsonHeaders()
	headers.Set("Authorization", "Bearer "+credentials["accessToken"])
	spec := providerRequest{method: http.MethodPut, url: base.String(), headers: headers, body: body}
	spec.accept = func(body []byte) (string, bool) {
		var response struct {
			EventID string `json:"event_id"`
		}
		return response.EventID, json.Unmarshal(body, &response) == nil && response.EventID != ""
	}
	return spec, nil
}

func providerURL(raw string, httpsOnly bool, hosts []string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") || (httpsOnly && parsed.Scheme != "https") {
		return nil, fmt.Errorf("notification provider requires a valid HTTP endpoint")
	}
	if len(hosts) > 0 {
		matched := false
		for _, host := range hosts {
			if strings.EqualFold(parsed.Hostname(), host) {
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("notification provider endpoint host is not supported")
		}
	}
	return parsed, nil
}

func post(target *url.URL, headers http.Header, body []byte) providerRequest {
	return providerRequest{method: http.MethodPost, url: target.String(), headers: headers, body: body}
}

func jsonHeaders() http.Header {
	return http.Header{"Content-Type": []string{"application/json"}}
}

func joinBodyLink(message ProviderMessage) string {
	value := message.Title + "\n" + message.Body
	if message.Link != "" {
		value += "\n" + message.Link
	}
	return value
}

func safeMentionText(value string) string {
	return strings.ReplaceAll(value, "@", "@\u200b")
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	for len(value) > limit {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}

func basicAuth(username, password string) string {
	req, _ := http.NewRequest(http.MethodGet, "http://localhost", nil)
	req.SetBasicAuth(username, password)
	return strings.TrimPrefix(req.Header.Get("Authorization"), "Basic ")
}

func providerConfigurationFailure() Result {
	return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeConfigurationInvalid}
}

func providerTransientFailure() Result {
	return Result{Status: StatusFailed, FailureClass: FailureTransientPreAcceptance, OutcomeCode: OutcomeTransportUnavailable}
}

func providerRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		return boundedProviderRetryAfter(time.Duration(seconds) * time.Second)
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	return boundedProviderRetryAfter(when.Sub(now))
}
