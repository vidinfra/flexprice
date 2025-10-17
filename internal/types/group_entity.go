package types

import (
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/samber/lo"
)

// GroupEntityType represents the type of entity that can be grouped
type GroupEntityType string

const (
	GroupEntityTypePrice        GroupEntityType = "price"
	GroupEntityTypePlan         GroupEntityType = "plan"
	GroupEntityTypeCustomer     GroupEntityType = "customer"
	GroupEntityTypeInvoice      GroupEntityType = "invoice"
	GroupEntityTypeSubscription GroupEntityType = "subscription"
	GroupEntityTypeAddon        GroupEntityType = "addon"
	GroupEntityTypeFeature      GroupEntityType = "feature"
	GroupEntityTypeEntitlement  GroupEntityType = "entitlement"
)

func (e GroupEntityType) String() string {
	return string(e)
}

func (e GroupEntityType) Validate() error {
	allowed := []GroupEntityType{
		GroupEntityTypePrice,
		GroupEntityTypePlan,
		GroupEntityTypeCustomer,
		GroupEntityTypeInvoice,
		GroupEntityTypeSubscription,
		GroupEntityTypeAddon,
		GroupEntityTypeFeature,
		GroupEntityTypeEntitlement,
	}
	if !lo.Contains(allowed, e) {
		return ierr.NewError("invalid group entity type").
			WithHint("Entity type must be one of: price, plan, customer, invoice, subscription, addon, feature, entitlement").
			Mark(ierr.ErrValidation)
	}
	return nil
}

// GetAllGroupEntityTypes returns all supported group entity types
func GetAllGroupEntityTypes() []GroupEntityType {
	return []GroupEntityType{
		GroupEntityTypePrice,
		GroupEntityTypePlan,
		GroupEntityTypeCustomer,
		GroupEntityTypeInvoice,
		GroupEntityTypeSubscription,
		GroupEntityTypeAddon,
		GroupEntityTypeFeature,
		GroupEntityTypeEntitlement,
	}
}

// IsValidGroupEntityType checks if the given string is a valid group entity type
func IsValidGroupEntityType(entityType string) bool {
	groupEntityType := GroupEntityType(entityType)
	return groupEntityType.Validate() == nil
}
