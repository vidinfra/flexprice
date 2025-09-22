package webhookDto

import "github.com/flexprice/flexprice/internal/types"

// InternalAlertEvent represents the internal alert event structure
type InternalAlertEvent struct {
	AlertLogID    string          `json:"alert_log_id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	AlertType     string          `json:"alert_type"`
	AlertStatus   string          `json:"alert_status"`
	AlertInfo     types.AlertInfo `json:"alert_info"`
	TenantID      string          `json:"tenant_id"`
	EnvironmentID string          `json:"environment_id"`
}

// AlertWebhookPayload represents the detailed payload for alert webhooks
type AlertWebhookPayload struct {
	EventType     string          `json:"event_type"`
	AlertLogID    string          `json:"alert_log_id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	AlertType     string          `json:"alert_type"`
	AlertStatus   string          `json:"alert_status"`
	AlertInfo     types.AlertInfo `json:"alert_info"`
	EnvironmentID string          `json:"environment_id"`
}

func NewAlertWebhookPayload(alertEvent *InternalAlertEvent, eventType string) *AlertWebhookPayload {
	return &AlertWebhookPayload{
		EventType:     eventType,
		AlertLogID:    alertEvent.AlertLogID,
		EntityType:    alertEvent.EntityType,
		EntityID:      alertEvent.EntityID,
		AlertType:     alertEvent.AlertType,
		AlertStatus:   alertEvent.AlertStatus,
		AlertInfo:     alertEvent.AlertInfo,
		EnvironmentID: alertEvent.EnvironmentID,
	}
}
