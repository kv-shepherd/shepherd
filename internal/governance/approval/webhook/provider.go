package webhook

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
	"strings"
	"time"

	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

const (
	providerType             = "webhook"
	SignatureHeader          = "X-Signature-256"
	TicketIDHeader           = "X-Ticket-ID"
	defaultTimeout           = 30 * time.Second
	defaultRetryCount        = 3
	defaultRetryBackoff      = 2 * time.Second
	maxErrorResponseBodySize = 4096
)

type Config struct {
	WebhookURL   string
	SigningKey   string
	Headers      map[string]string
	Timeout      time.Duration
	RetryCount   int
	RetryBackoff time.Duration
	HTTPClient   *http.Client

	// AllowInsecureHTTP is intended for local tests only. Production webhook
	// endpoints must use HTTPS.
	AllowInsecureHTTP bool
}

type Provider struct {
	webhookURL   string
	signingKey   []byte
	timeout      time.Duration
	retryCount   int
	retryBackoff time.Duration
	headers      map[string]string
	httpClient   *http.Client
	fallback     approvalcontract.ApprovalProvider
	now          func() time.Time
	sleep        func(context.Context, time.Duration) error
}

type submitPayload struct {
	EventID     string                 `json:"event_id"`
	Requester   string                 `json:"requester"`
	Action      string                 `json:"action"`
	Reason      string                 `json:"reason"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	SubmittedAt string                 `json:"submitted_at"`
}

var _ approvalcontract.ApprovalProvider = (*Provider)(nil)

func NewProvider(config Config, fallback approvalcontract.ApprovalProvider) (*Provider, error) {
	if fallback == nil {
		return nil, errors.New("approval webhook provider: fallback provider is required")
	}
	parsedURL, err := url.Parse(strings.TrimSpace(config.WebhookURL))
	if err != nil {
		return nil, fmt.Errorf("approval webhook provider: invalid webhook URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errors.New("approval webhook provider: webhook URL must be absolute")
	}
	if parsedURL.Scheme != "https" && (!config.AllowInsecureHTTP || parsedURL.Scheme != "http") {
		return nil, errors.New("approval webhook provider: webhook URL must use HTTPS")
	}
	signingKey := strings.TrimSpace(config.SigningKey)
	if signingKey == "" {
		return nil, errors.New("approval webhook provider: signing key is required")
	}
	headers, err := normalizeHeaders(config.Headers)
	if err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	retryCount := config.RetryCount
	if retryCount <= 0 {
		retryCount = defaultRetryCount
	}
	retryBackoff := config.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = defaultRetryBackoff
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	return &Provider{
		webhookURL:   parsedURL.String(),
		signingKey:   []byte(signingKey),
		timeout:      timeout,
		retryCount:   retryCount,
		retryBackoff: retryBackoff,
		headers:      headers,
		httpClient:   httpClient,
		fallback:     fallback,
		now:          time.Now,
		sleep:        sleepWithContext,
	}, nil
}

func (p *Provider) Type() string { return providerType }

func (p *Provider) SubmitForApproval(ctx context.Context, req *approvalcontract.ApprovalRequest) (*approvalcontract.ApprovalResponse, error) {
	if req == nil {
		return nil, errors.New("approval webhook provider: request must not be nil")
	}
	if strings.TrimSpace(req.EventID) == "" {
		return nil, errors.New("approval webhook provider: event_id is required")
	}

	payload := submitPayload{
		EventID:     req.EventID,
		Requester:   req.Requester,
		Action:      req.Action,
		Reason:      req.Reason,
		Metadata:    req.Metadata,
		SubmittedAt: p.now().UTC().Format(time.RFC3339Nano),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("approval webhook provider: marshal payload: %w", err)
	}
	if err := p.sendWithRetry(ctx, req.EventID, body); err == nil {
		return &approvalcontract.ApprovalResponse{
			TicketID: req.EventID,
			Status:   "PENDING_EXTERNAL",
		}, nil
	} else if ctx.Err() != nil {
		return nil, err
	}

	return p.fallback.SubmitForApproval(ctx, req)
}

func (p *Provider) ProcessApproval(ctx context.Context, ticketID string, decision approvalcontract.ApprovalDecision) error {
	return p.fallback.ProcessApproval(ctx, ticketID, decision)
}

func (p *Provider) sendWithRetry(ctx context.Context, ticketID string, body []byte) error {
	var lastErr error
	for attempt := 0; attempt <= p.retryCount; attempt++ {
		if attempt > 0 {
			if err := p.sleep(ctx, backoffDelay(p.retryBackoff, attempt)); err != nil {
				return err
			}
		}
		if err := p.sendOnce(ctx, ticketID, body); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (p *Provider) sendOnce(ctx context.Context, ticketID string, body []byte) error {
	reqCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("approval webhook provider: create request: %w", err)
	}
	for key, value := range p.headers {
		request.Header.Set(key, value)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(SignatureHeader, SignPayload(body, p.signingKey))
	request.Header.Set(TicketIDHeader, ticketID)

	// #nosec G704 -- external approval webhooks are explicitly administrator-configured;
	// NewProvider validates the endpoint as an absolute HTTPS URL before use.
	response, err := p.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("approval webhook provider: send request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	errorBody, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorResponseBodySize))
	if len(errorBody) == 0 {
		return fmt.Errorf("approval webhook provider: webhook returned HTTP %d", response.StatusCode)
	}
	return fmt.Errorf("approval webhook provider: webhook returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(errorBody)))
}

func SignPayload(body, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func VerifySignature(body, secret []byte, signature string) bool {
	normalized := strings.TrimSpace(signature)
	if !strings.HasPrefix(normalized, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(normalized, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), got)
}

func backoffDelay(base time.Duration, attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	delay := base
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	return delay
}

func normalizeHeaders(raw map[string]string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	headers := make(map[string]string, len(raw))
	for key, value := range raw {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, errors.New("approval webhook provider: header name must not be empty")
		}
		if !isHTTPToken(name) {
			return nil, fmt.Errorf("approval webhook provider: invalid header name %q", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("approval webhook provider: invalid header value for %q", name)
		}
		headers[name] = strings.TrimSpace(value)
	}
	return headers, nil
}

func isHTTPToken(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return value != ""
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
