package webhookDto

import "github.com/flexprice/flexprice/internal/api/dto"

type InternalAlertEvent struct {
	FeatureID   string `json:"feature_id,omitempty"`
	WalletID    string `json:"wallet_id,omitempty"`
	AlertType   string `json:"alert_type"`
	AlertStatus string `json:"alert_status"`
	TenantID    string `json:"tenant_id"`
}

type AlertWebhookPayload struct {
	EventType    string               `json:"event_type"`
	Alert_type   string               `json:"alert_type"`
	Alert_status string               `json:"alert_status"`
	Feature      *dto.FeatureResponse `json:"feature,omitempty"`
	Wallet       *dto.WalletResponse  `json:"wallet,omitempty"`
}

func NewAlertWebhookPayload(feature *dto.FeatureResponse, wallet *dto.WalletResponse, alert_type string, alert_status string, eventType string) *AlertWebhookPayload {
	return &AlertWebhookPayload{EventType: eventType, Alert_type: alert_type, Alert_status: alert_status, Feature: feature, Wallet: wallet}
}
