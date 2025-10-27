package hubspot

// Provider constants:
// Use types.SecretProviderHubSpot for provider type (defined in internal/types/secret.go)
// Use types.ConnectionMetadataTypeHubSpot for connection metadata type (defined in internal/types/connection.go)

// HubSpot webhook subscription types
type SubscriptionType string

const (
	SubscriptionTypeDealPropertyChange SubscriptionType = "deal.propertyChange"
)

// HubSpot deal properties
const (
	PropertyNameDealStage = "dealstage"
)

// HubSpot deal stage values
type DealStage string

const (
	DealStageClosedWon DealStage = "closedwon"
)

// HubSpot invoice status values
type InvoiceStatus string

const (
	InvoiceStatusDraft InvoiceStatus = "draft"
	InvoiceStatusOpen  InvoiceStatus = "open"
)
