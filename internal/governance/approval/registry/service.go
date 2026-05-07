package registry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalapprovalsystem"
	approvalwebhook "kv-shepherd.io/shepherd/internal/governance/approval/webhook"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

const (
	ProviderTypeWebhook       = "webhook"
	DefaultTimeoutSeconds     = 30
	DefaultRetryCount         = 3
	DefaultRetryBackoffSecond = 2
)

type Service struct {
	client *ent.Client
	codec  *SigningKeyCodec
}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

func IsValidationError(err error) bool {
	var validationErr ValidationError
	return errors.As(err, &validationErr)
}

type System struct {
	ID                  string
	Name                string
	ProviderType        string
	Enabled             bool
	WebhookURL          string
	WebhookHeaders      map[string]string
	TimeoutSeconds      int
	RetryCount          int
	RetryBackoffSeconds int
	SigningKeySet       bool
	SortOrder           int
	CreatedBy           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type CreateInput struct {
	Name                string
	ProviderType        string
	Enabled             *bool
	WebhookURL          string
	WebhookHeaders      map[string]string
	TimeoutSeconds      *int
	RetryCount          *int
	RetryBackoffSeconds *int
	SigningKey          string
	SortOrder           *int
	CreatedBy           string
}

type UpdateInput struct {
	Name                *string
	Enabled             *bool
	WebhookURL          *string
	WebhookHeaders      *map[string]string
	TimeoutSeconds      *int
	RetryCount          *int
	RetryBackoffSeconds *int
	SigningKey          *string
	SortOrder           *int
}

type normalizedCreateInput struct {
	Name                string
	ProviderType        string
	Enabled             *bool
	WebhookURL          string
	WebhookHeaders      map[string]string
	TimeoutSeconds      int
	RetryCount          int
	RetryBackoffSeconds int
	SigningKey          string
	SortOrder           *int
	CreatedBy           string
}

func NewService(client *ent.Client, encryptionKey []byte) *Service {
	return &Service{
		client: client,
		codec:  NewSigningKeyCodec(encryptionKey),
	}
}

func (s *Service) WithClient(client *ent.Client) *Service {
	if s == nil {
		return &Service{client: client}
	}
	derived := *s
	derived.client = client
	return &derived
}

func (s *Service) List(ctx context.Context) ([]System, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("external approval registry is not initialized")
	}
	rows, err := s.client.ExternalApprovalSystem.Query().
		Order(
			ent.Asc(externalapprovalsystem.FieldSortOrder),
			ent.Asc(externalapprovalsystem.FieldName),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list external approval systems: %w", err)
	}
	out := make([]System, 0, len(rows))
	for _, row := range rows {
		out = append(out, systemToModel(row))
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*System, error) {
	if s == nil || s.client == nil || s.codec == nil {
		return nil, fmt.Errorf("external approval registry is not initialized")
	}
	normalized, err := normalizeCreateInput(in)
	if err != nil {
		return nil, err
	}
	ciphertext, keyID, err := s.codec.EncryptForStorage(normalized.SigningKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ciphertext) == "" {
		return nil, ValidationError{Message: "signing_key is required"}
	}

	id, uuidErr := uuid.NewV7()
	if uuidErr != nil {
		return nil, fmt.Errorf("generate external approval system id: %w", uuidErr)
	}
	create := s.client.ExternalApprovalSystem.Create().
		SetID(id.String()).
		SetName(normalized.Name).
		SetProviderType(externalapprovalsystem.ProviderType(normalized.ProviderType)).
		SetWebhookURL(normalized.WebhookURL).
		SetWebhookHeaders(normalized.WebhookHeaders).
		SetTimeoutSeconds(normalized.TimeoutSeconds).
		SetRetryCount(normalized.RetryCount).
		SetRetryBackoffSeconds(normalized.RetryBackoffSeconds).
		SetSigningKeyCiphertext(ciphertext).
		SetEncryptionKeyID(keyID).
		SetCreatedBy(normalized.CreatedBy)
	if normalized.Enabled != nil {
		create = create.SetEnabled(*normalized.Enabled)
	}
	if normalized.SortOrder != nil {
		create = create.SetSortOrder(*normalized.SortOrder)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create external approval system: %w", err)
	}
	out := systemToModel(row)
	return &out, nil
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*System, error) {
	if s == nil || s.client == nil || s.codec == nil {
		return nil, fmt.Errorf("external approval registry is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ValidationError{Message: "external approval system id is required"}
	}
	existing, err := s.client.ExternalApprovalSystem.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	update := s.client.ExternalApprovalSystem.UpdateOneID(id)
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, ValidationError{Message: "name cannot be empty"}
		}
		update = update.SetName(name)
	}
	if in.Enabled != nil {
		update = update.SetEnabled(*in.Enabled)
	}
	if in.WebhookURL != nil {
		webhookURL, validateErr := normalizeWebhookURL(*in.WebhookURL)
		if validateErr != nil {
			return nil, validateErr
		}
		update = update.SetWebhookURL(webhookURL)
	}
	if in.WebhookHeaders != nil {
		headers, validateErr := normalizeHeaders(*in.WebhookHeaders)
		if validateErr != nil {
			return nil, validateErr
		}
		update = update.SetWebhookHeaders(headers)
	}
	if in.TimeoutSeconds != nil {
		timeoutSeconds, validateErr := normalizeBoundedInt("timeout_seconds", in.TimeoutSeconds, DefaultTimeoutSeconds, 120)
		if validateErr != nil {
			return nil, validateErr
		}
		update = update.SetTimeoutSeconds(timeoutSeconds)
	}
	if in.RetryCount != nil {
		retryCount, validateErr := normalizeBoundedInt("retry_count", in.RetryCount, DefaultRetryCount, 10)
		if validateErr != nil {
			return nil, validateErr
		}
		update = update.SetRetryCount(retryCount)
	}
	if in.RetryBackoffSeconds != nil {
		backoffSeconds, validateErr := normalizeBoundedInt("retry_backoff_seconds", in.RetryBackoffSeconds, DefaultRetryBackoffSecond, 60)
		if validateErr != nil {
			return nil, validateErr
		}
		update = update.SetRetryBackoffSeconds(backoffSeconds)
	}
	if in.SigningKey != nil && !isProtectedSigningKeyMask(*in.SigningKey) {
		signingKey := strings.TrimSpace(*in.SigningKey)
		if signingKey == "" {
			return nil, ValidationError{Message: "signing_key cannot be empty"}
		}
		ciphertext, keyID, encErr := s.codec.EncryptForStorage(signingKey)
		if encErr != nil {
			return nil, encErr
		}
		update = update.SetSigningKeyCiphertext(ciphertext).SetEncryptionKeyID(keyID)
	}
	if in.SortOrder != nil {
		update = update.SetSortOrder(*in.SortOrder)
	}

	row, err := update.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("update external approval system: %w", err)
	}
	if strings.TrimSpace(row.SigningKeyCiphertext) == "" && strings.TrimSpace(existing.SigningKeyCiphertext) != "" {
		row.SigningKeyCiphertext = existing.SigningKeyCiphertext
		row.EncryptionKeyID = existing.EncryptionKeyID
	}
	out := systemToModel(row)
	return &out, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("external approval registry is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ValidationError{Message: "external approval system id is required"}
	}
	return s.client.ExternalApprovalSystem.DeleteOneID(id).Exec(ctx)
}

func (s *Service) ActiveProvider(ctx context.Context, fallback approvalcontract.ApprovalProvider) (approvalcontract.ApprovalProvider, error) {
	if fallback == nil {
		return nil, fmt.Errorf("external approval registry requires a fallback provider")
	}
	if s == nil || s.client == nil || s.codec == nil {
		return fallback, nil
	}
	row, err := s.client.ExternalApprovalSystem.Query().
		Where(externalapprovalsystem.EnabledEQ(true)).
		Order(
			ent.Asc(externalapprovalsystem.FieldSortOrder),
			ent.Asc(externalapprovalsystem.FieldName),
		).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fallback, nil
		}
		return nil, fmt.Errorf("load active external approval system: %w", err)
	}
	signingKey, err := s.codec.DecryptForUse(row.SigningKeyCiphertext, row.EncryptionKeyID)
	if err != nil {
		return nil, fmt.Errorf("decrypt external approval signing key: %w", err)
	}
	provider, err := approvalwebhook.NewProvider(approvalwebhook.Config{
		WebhookURL:   row.WebhookURL,
		SigningKey:   signingKey,
		Headers:      row.WebhookHeaders,
		Timeout:      time.Duration(row.TimeoutSeconds) * time.Second,
		RetryCount:   row.RetryCount,
		RetryBackoff: time.Duration(row.RetryBackoffSeconds) * time.Second,
	}, fallback)
	if err != nil {
		return nil, fmt.Errorf("create external approval webhook provider: %w", err)
	}
	return provider, nil
}

func normalizeCreateInput(in CreateInput) (normalizedCreateInput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return normalizedCreateInput{}, ValidationError{Message: "name is required"}
	}
	providerType := normalizeProviderType(in.ProviderType)
	if providerType != ProviderTypeWebhook {
		return normalizedCreateInput{}, ValidationError{Message: "type must be webhook"}
	}
	webhookURL, err := normalizeWebhookURL(in.WebhookURL)
	if err != nil {
		return normalizedCreateInput{}, err
	}
	headers, err := normalizeHeaders(in.WebhookHeaders)
	if err != nil {
		return normalizedCreateInput{}, err
	}
	timeoutSeconds, err := normalizeBoundedInt("timeout_seconds", in.TimeoutSeconds, DefaultTimeoutSeconds, 120)
	if err != nil {
		return normalizedCreateInput{}, err
	}
	retryCount, err := normalizeBoundedInt("retry_count", in.RetryCount, DefaultRetryCount, 10)
	if err != nil {
		return normalizedCreateInput{}, err
	}
	backoffSeconds, err := normalizeBoundedInt("retry_backoff_seconds", in.RetryBackoffSeconds, DefaultRetryBackoffSecond, 60)
	if err != nil {
		return normalizedCreateInput{}, err
	}
	createdBy := strings.TrimSpace(in.CreatedBy)
	if createdBy == "" {
		return normalizedCreateInput{}, ValidationError{Message: "created_by is required"}
	}
	signingKey := strings.TrimSpace(in.SigningKey)
	if signingKey == "" || isProtectedSigningKeyMask(signingKey) {
		return normalizedCreateInput{}, ValidationError{Message: "signing_key is required"}
	}
	return normalizedCreateInput{
		Name:                name,
		ProviderType:        providerType,
		Enabled:             in.Enabled,
		WebhookURL:          webhookURL,
		WebhookHeaders:      headers,
		TimeoutSeconds:      timeoutSeconds,
		RetryCount:          retryCount,
		RetryBackoffSeconds: backoffSeconds,
		SigningKey:          signingKey,
		SortOrder:           in.SortOrder,
		CreatedBy:           createdBy,
	}, nil
}

func normalizeProviderType(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ProviderTypeWebhook
	}
	return value
}

func normalizeWebhookURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", ValidationError{Message: "webhook_url is required"}
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", ValidationError{Message: "webhook_url is invalid"}
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", ValidationError{Message: "webhook_url must be absolute"}
	}
	if parsed.Scheme != "https" {
		return "", ValidationError{Message: "webhook_url must use HTTPS"}
	}
	return parsed.String(), nil
}

func normalizeHeaders(raw map[string]string) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}
	headers := make(map[string]string, len(raw))
	for key, value := range raw {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, ValidationError{Message: "webhook_headers contains an empty header name"}
		}
		if !isHTTPToken(name) {
			return nil, ValidationError{Message: "webhook_headers contains an invalid header name"}
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, ValidationError{Message: "webhook_headers contains an invalid header value"}
		}
		headers[name] = strings.TrimSpace(value)
	}
	return headers, nil
}

func normalizeBoundedInt(name string, value *int, defaultValue, maxValue int) (int, error) {
	if value == nil {
		return defaultValue, nil
	}
	if *value < 1 || *value > maxValue {
		return 0, ValidationError{Message: fmt.Sprintf("%s must be between 1 and %d", name, maxValue)}
	}
	return *value, nil
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

func systemToModel(row *ent.ExternalApprovalSystem) System {
	if row == nil {
		return System{}
	}
	headers := make(map[string]string, len(row.WebhookHeaders))
	for key, value := range row.WebhookHeaders {
		headers[key] = value
	}
	return System{
		ID:                  row.ID,
		Name:                row.Name,
		ProviderType:        string(row.ProviderType),
		Enabled:             row.Enabled,
		WebhookURL:          row.WebhookURL,
		WebhookHeaders:      headers,
		TimeoutSeconds:      row.TimeoutSeconds,
		RetryCount:          row.RetryCount,
		RetryBackoffSeconds: row.RetryBackoffSeconds,
		SigningKeySet:       strings.TrimSpace(row.SigningKeyCiphertext) != "",
		SortOrder:           row.SortOrder,
		CreatedBy:           row.CreatedBy,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}
