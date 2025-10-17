package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/domain/price"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
)

// EntityValidator defines the contract for entity-specific operations
type EntityValidator interface {
	Validate(ctx context.Context, ids []string, excludeGroupID string) ([]string, error)
	GetAssociated(ctx context.Context, groupID string) ([]string, error)
	GetAssociatedBulk(ctx context.Context, groupIDs []string) (map[string][]string, error)
	Associate(ctx context.Context, groupID string, ids []string) error
	Disassociate(ctx context.Context, ids []string) error
}

// PriceValidator implements EntityValidator for prices
type PriceValidator struct {
	priceRepo price.Repository
	log       *logger.Logger
}

// NewPriceValidator creates a new price validator
func NewPriceValidator(priceRepo price.Repository, log *logger.Logger) EntityValidator {
	return &PriceValidator{
		priceRepo: priceRepo,
		log:       log,
	}
}

// Validate validates that price IDs exist and are not already in another group
func (v *PriceValidator) Validate(ctx context.Context, ids []string, excludeGroupID string) ([]string, error) {
	// Validate prices exist
	count, err := v.priceRepo.CountByIDs(ctx, ids)
	if err != nil {
		v.log.Error("failed to count prices", "error", err, "price_ids", ids)
		return nil, err
	}
	if count != len(ids) {
		v.log.Warn("some prices not found", "expected_count", len(ids), "actual_count", count)
		return nil, ierr.NewError("one or more prices not found").
			WithHint("One or more price IDs are invalid").
			Mark(ierr.ErrValidation)
	}

	// Check if any prices are already in other groups
	notInGroupCount, err := v.priceRepo.CountNotInGroup(ctx, ids, excludeGroupID)
	if err != nil {
		v.log.Error("failed to check group availability", "error", err, "price_ids", ids)
		return nil, err
	}
	if notInGroupCount > 0 {
		v.log.Warn("some prices already in other groups", "already_grouped_count", notInGroupCount)
		return nil, ierr.NewError("one or more prices are already in another group").
			WithHint("One or more prices are already assigned to another group").
			Mark(ierr.ErrValidation)
	}

	return ids, nil
}

// GetAssociated retrieves all entity IDs associated with a group
func (v *PriceValidator) GetAssociated(ctx context.Context, groupID string) ([]string, error) {
	// Use GetByGroupIDs with a single group ID
	prices, err := v.priceRepo.GetByGroupIDs(ctx, []string{groupID})
	if err != nil {
		v.log.Error("failed to get prices in group", "error", err, "group_id", groupID)
		return nil, err
	}

	priceIDs := make([]string, len(prices))
	for i, price := range prices {
		priceIDs[i] = price.ID
	}

	v.log.Debug("retrieved associated prices", "group_id", groupID, "price_count", len(priceIDs))
	return priceIDs, nil
}

// GetAssociatedBulk retrieves all entity IDs associated with multiple groups in a single query
func (v *PriceValidator) GetAssociatedBulk(ctx context.Context, groupIDs []string) (map[string][]string, error) {
	if len(groupIDs) == 0 {
		return make(map[string][]string), nil
	}

	prices, err := v.priceRepo.GetByGroupIDs(ctx, groupIDs)
	if err != nil {
		v.log.Error("failed to get prices by group IDs", "error", err, "group_ids", groupIDs)
		return nil, err
	}

	// Group prices by group_id
	result := make(map[string][]string)
	for _, price := range prices {
		if price.GroupID != "" {
			result[price.GroupID] = append(result[price.GroupID], price.ID)
		}
	}

	// Ensure all groups have entries (even if empty)
	for _, groupID := range groupIDs {
		if _, exists := result[groupID]; !exists {
			result[groupID] = []string{}
		}
	}

	v.log.Debug("retrieved associated prices in bulk", "group_count", len(groupIDs), "total_prices", len(prices))
	return result, nil
}

// Associate associates entities with a group
func (v *PriceValidator) Associate(ctx context.Context, groupID string, ids []string) error {
	v.log.Debug("associating prices with group", "group_id", groupID, "price_ids", ids)
	return v.priceRepo.UpdateGroupIDBulk(ctx, ids, &groupID)
}

// Disassociate removes entities from their current group
func (v *PriceValidator) Disassociate(ctx context.Context, ids []string) error {
	v.log.Debug("disassociating prices from groups", "price_ids", ids)
	return v.priceRepo.ClearGroupIDBulk(ctx, ids)
}
