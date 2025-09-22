package payload

import (
	"context"
	"encoding/json"
	"fmt"

	ierr "github.com/flexprice/flexprice/internal/errors"
	webhookDto "github.com/flexprice/flexprice/internal/webhook/dto"
)

type AlertPayloadBuilder struct {
	services *Services
}

func NewAlertPayloadBuilder(services *Services) PayloadBuilder {
	return &AlertPayloadBuilder{services: services}
}

func (b *AlertPayloadBuilder) BuildPayload(ctx context.Context, eventType string, data json.RawMessage) (json.RawMessage, error) {
	var parsedPayload webhookDto.InternalAlertEvent

	err := json.Unmarshal(data, &parsedPayload)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Unable to unmarshal alert event payload").
			Mark(ierr.ErrInvalidOperation)
	}

	alertLogID, tenantID := parsedPayload.AlertLogID, parsedPayload.TenantID
	if alertLogID == "" || tenantID == "" {
		return nil, ierr.NewError("invalid data type for alert event").
			WithHint("Please provide a valid alert log ID and tenant ID").
			WithReportableDetails(map[string]any{
				"expected": "string",
				"got":      fmt.Sprintf("%T", data),
			}).
			Mark(ierr.ErrInvalidOperation)
	}

	// Build the webhook payload directly from the internal event
	// since alert logs are immutable and contain all necessary information
	payload := webhookDto.NewAlertWebhookPayload(&parsedPayload, eventType)

	return json.Marshal(payload)
}
