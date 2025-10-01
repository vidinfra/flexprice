package types

import (
	"time"

	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

// AlertState represents the current state of a wallet alert
type AlertState string

const (
	AlertStateOk      AlertState = "ok"
	AlertStateInAlarm AlertState = "in_alarm"
	AlertStateWarning AlertState = "warning"
)

type AlertType string

const (
	AlertTypeLowOngoingBalance AlertType = "low_ongoing_balance"
	AlertTypeLowCreditBalance  AlertType = "low_credit_balance"
	AlertTypeFeatureBalance    AlertType = "feature_balance"
)

// AlertEntityType represents the type of entity for alerts
type AlertEntityType string

const (
	AlertEntityTypeWallet  AlertEntityType = "wallet"
	AlertEntityTypeFeature AlertEntityType = "feature"
)

func (aet AlertEntityType) Validate() error {
	allowedTypes := []AlertEntityType{
		AlertEntityTypeWallet,
		AlertEntityTypeFeature,
	}
	if !lo.Contains(allowedTypes, aet) {
		return ierr.NewError("invalid alert entity type").
			WithHint("Please provide a valid alert entity type").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// AlertThresholdType represents the type of threshold for alerts
type AlertThresholdType string

const (
	AlertThresholdTypeAmount AlertThresholdType = "amount"
)

func (att AlertThresholdType) Validate() error {
	allowedTypes := []AlertThresholdType{
		AlertThresholdTypeAmount,
	}
	if !lo.Contains(allowedTypes, att) {
		return ierr.NewError("invalid alert threshold type").
			WithHint("Please provide a valid alert threshold type").
			Mark(ierr.ErrValidation)
	}
	return nil
}

func (at AlertType) Validate() error {
	allowedTypes := []AlertType{
		AlertTypeLowOngoingBalance,
		AlertTypeLowCreditBalance,
		AlertTypeFeatureBalance,
	}
	if !lo.Contains(allowedTypes, at) {
		return ierr.NewError("invalid alert type").
			WithHint("Please provide a valid alert type").
			Mark(ierr.ErrValidation)
	}
	return nil
}

type AlertInfo struct {
	Threshold            AlertThreshold        `json:"threshold,omitempty"`              // For wallet alerts
	FeatureAlertSettings *FeatureAlertSettings `json:"feature_alert_settings,omitempty"` // For feature alerts
	ValueAtTime          decimal.Decimal       `json:"value_at_time"`
	Timestamp            time.Time             `json:"timestamp"`
}

// AlertConfig represents the configuration for wallet alerts
type AlertConfig struct {
	Threshold *AlertThreshold `json:"threshold,omitempty"`
}

// AlertThreshold represents the threshold configuration
type AlertThreshold struct {
	Type  AlertThresholdType `json:"type"` // amount
	Value decimal.Decimal    `json:"value"`
}

// AlertLogFilter represents filters for alert log queries
type AlertLogFilter struct {
	*QueryFilter
	*TimeRangeFilter

	// filters allows complex filtering based on multiple fields
	Filters     []*FilterCondition `json:"filters,omitempty" form:"filters" validate:"omitempty"`
	Sort        []*SortCondition   `json:"sort,omitempty" form:"sort" validate:"omitempty"`
	EntityType  AlertEntityType    `json:"entity_type,omitempty" form:"entity_type" validate:"omitempty"`
	EntityID    string             `json:"entity_id,omitempty" form:"entity_id" validate:"omitempty"`
	AlertType   AlertType          `json:"alert_type,omitempty" form:"alert_type" validate:"omitempty"`
	AlertStatus AlertState         `json:"alert_status,omitempty" form:"alert_status" validate:"omitempty"`
}

// NewDefaultAlertLogFilter creates a new AlertLogFilter with default values
func NewDefaultAlertLogFilter() *AlertLogFilter {
	return &AlertLogFilter{
		QueryFilter: NewDefaultQueryFilter(),
	}
}

// NewNoLimitAlertLogFilter creates a new AlertLogFilter with no pagination limits
func NewNoLimitAlertLogFilter() *AlertLogFilter {
	return &AlertLogFilter{
		QueryFilter: NewNoLimitQueryFilter(),
	}
}

// Validate validates the alert log filter
func (f *AlertLogFilter) Validate() error {
	if f.QueryFilter != nil {
		if err := f.QueryFilter.Validate(); err != nil {
			return err
		}
	}

	if f.TimeRangeFilter != nil {
		if err := f.TimeRangeFilter.Validate(); err != nil {
			return err
		}
	}

	if f.AlertType != "" {
		if err := f.AlertType.Validate(); err != nil {
			return err
		}
	}

	if f.EntityType != "" {
		if err := f.EntityType.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// GetLimit implements BaseFilter interface
func (f *AlertLogFilter) GetLimit() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetLimit()
	}
	return f.QueryFilter.GetLimit()
}

// GetOffset implements BaseFilter interface
func (f *AlertLogFilter) GetOffset() int {
	if f.QueryFilter == nil {
		return NewDefaultQueryFilter().GetOffset()
	}
	return f.QueryFilter.GetOffset()
}

type FeatureAlertSettings struct {
	Upperbound *decimal.Decimal `json:"upperbound" validate:"required"`
	Lowerbound *decimal.Decimal `json:"lowerbound" validate:"required"`
}

// Validate validates the feature alert settings
// Both upperbound and lowerbound must be provided and upperbound must be greater than or equal to lowerbound
func (f *FeatureAlertSettings) Validate() error {
	// Check if upperbound is provided
	if f.Upperbound == nil {
		return ierr.NewError("upperbound is required").
			WithHint("Please provide a valid upperbound value").
			Mark(ierr.ErrValidation)
	}

	// Check if lowerbound is provided
	if f.Lowerbound == nil {
		return ierr.NewError("lowerbound is required").
			WithHint("Please provide a valid lowerbound value").
			Mark(ierr.ErrValidation)
	}

	// Check if upperbound is greater than or equal to lowerbound
	if f.Upperbound.LessThan(*f.Lowerbound) {
		return ierr.NewError("upperbound must be greater than or equal to lowerbound").
			WithHint("Please provide valid feature alert settings where upperbound >= lowerbound").
			Mark(ierr.ErrValidation)
	}

	return nil
}

// determineFeatureAlertStatus determines the alert status based on ongoing balance vs alert settings
// if ongoing_balance > upperbound: alert_status = ok
// if upperbound >= ongoing_balance >= lowerbound: alert_status = warning
// if ongoing_balance < lowerbound: alert_status = in_alarm
func (f *FeatureAlertSettings) DetermineFeatureAlertStatus(ongoingBalance decimal.Decimal) AlertState {
	upperbound := *f.Upperbound
	lowerbound := *f.Lowerbound

	// ongoing_balance >= upperbound
	if ongoingBalance.GreaterThanOrEqual(upperbound) {
		return AlertStateOk
	}

	// upperbound > ongoing_balance > lowerbound
	if ongoingBalance.LessThan(upperbound) && ongoingBalance.GreaterThan(lowerbound) {
		return AlertStateWarning
	}

	// ongoing_balance <= lowerbound
	return AlertStateInAlarm
}
