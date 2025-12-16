package types

import "github.com/shopspring/decimal"

// CommitmentType defines how commitment is specified - either as an amount or quantity
type CommitmentType string

const (
	// COMMITMENT_TYPE_AMOUNT indicates commitment is specified as a monetary amount
	COMMITMENT_TYPE_AMOUNT CommitmentType = "amount"
	// COMMITMENT_TYPE_QUANTITY indicates commitment is specified as a usage quantity
	COMMITMENT_TYPE_QUANTITY CommitmentType = "quantity"
)

// Validate checks if the commitment type is valid
func (ct CommitmentType) Validate() bool {
	switch ct {
	case COMMITMENT_TYPE_AMOUNT, COMMITMENT_TYPE_QUANTITY:
		return true
	default:
		return false
	}
}

// String returns the string representation of the commitment type
func (ct CommitmentType) String() string {
	return string(ct)
}

// CommitmentInfo holds information about a commitment
type CommitmentInfo struct {
	Type             CommitmentType   `json:"type"`
	Amount           decimal.Decimal  `json:"amount" swaggertype:"string"`
	Quantity         decimal.Decimal  `json:"quantity,omitempty" swaggertype:"string"`
	Utilized         decimal.Decimal  `json:"computed_utilized_amount" swaggertype:"string"`
	Overage          decimal.Decimal  `json:"computed_overage_amount" swaggertype:"string"`
	TrueUp           decimal.Decimal  `json:"computed_true_up_amount" swaggertype:"string"`
	OverageFactor    *decimal.Decimal `json:"overage_factor,omitempty" swaggertype:"string"`
	TrueUpEnabled    bool             `json:"true_up_enabled"`
	IsWindowed       bool             `json:"is_windowed"`
	WindowSize       *string          `json:"window_size,omitempty"`
	UsageResetPeriod string           `json:"usage_reset_period,omitempty"`
}
