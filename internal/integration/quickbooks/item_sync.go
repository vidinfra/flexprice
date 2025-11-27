package quickbooks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	"github.com/flexprice/flexprice/internal/domain/meter"
	"github.com/flexprice/flexprice/internal/domain/plan"
	"github.com/flexprice/flexprice/internal/domain/price"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// QuickBooksItemSyncService defines the interface for QuickBooks item synchronization
type QuickBooksItemSyncService interface {
	SyncPlanToQuickBooks(ctx context.Context, plan *plan.Plan, prices []*price.Price) error
	SyncPriceToQuickBooks(ctx context.Context, price *price.Price, plan *plan.Plan, meter *meter.Meter, recurringCount int) error
}

// ItemSyncServiceParams holds dependencies for ItemSyncService
type ItemSyncServiceParams struct {
	Client                       QuickBooksClient
	EntityIntegrationMappingRepo entityintegrationmapping.Repository
	MeterRepo                    meter.Repository
	Logger                       *logger.Logger
}

// ItemSyncService handles synchronization of Flexprice plans/prices to QuickBooks items
type ItemSyncService struct {
	ItemSyncServiceParams
}

// NewItemSyncService creates a new QuickBooks item sync service
func NewItemSyncService(params ItemSyncServiceParams) QuickBooksItemSyncService {
	return &ItemSyncService{
		ItemSyncServiceParams: params,
	}
}

// SyncPlanToQuickBooks syncs a Flexprice plan and its prices to QuickBooks items.
// Each price in the plan becomes a Service Item in QuickBooks.
// The method tracks recurring charge count to generate unique item names for recurring charges.
// All prices are synced (both usage-based and fixed/recurring) as per requirements.
func (s *ItemSyncService) SyncPlanToQuickBooks(ctx context.Context, plan *plan.Plan, prices []*price.Price) error {
	s.Logger.Infow("syncing plan to QuickBooks",
		"plan_id", plan.ID,
		"plan_name", plan.Name,
		"prices_count", len(prices))

	// Nothing to sync if no prices
	if len(prices) == 0 {
		s.Logger.Infow("no prices to sync for plan",
			"plan_id", plan.ID,
			"plan_name", plan.Name)
		return nil
	}

	// Track recurring charge count for item naming
	// Recurring charges (FIXED without meter) get names like "PlanName-Recurring-1", "PlanName-Recurring-2", etc.
	recurringCount := 0
	successCount := 0

	for _, priceItem := range prices {
		// Get meter if price has meter ID - used for usage-based charges
		var meterItem *meter.Meter
		if priceItem.MeterID != "" {
			var err error
			meterItem, err = s.MeterRepo.GetMeter(ctx, priceItem.MeterID)
			if err != nil {
				// Log warning but continue - meter name is used for item naming but not critical
				s.Logger.Warnw("failed to find meter for price, continuing without meter name",
					"price_id", priceItem.ID,
					"meter_id", priceItem.MeterID,
					"error", err)
			}
		}

		// Determine if this is a recurring charge (FIXED price without meter)
		// Recurring charges need unique names, so we track the count
		isRecurring := priceItem.Type == types.PRICE_TYPE_FIXED && meterItem == nil
		if isRecurring {
			recurringCount++
		}

		// Sync each price as a QuickBooks item
		// Continue with other prices even if one fails to allow partial sync
		if err := s.SyncPriceToQuickBooks(ctx, priceItem, plan, meterItem, recurringCount); err != nil {
			s.Logger.Errorw("failed to sync price to QuickBooks",
				"price_id", priceItem.ID,
				"plan_id", plan.ID,
				"error", err)
			continue
		}
		successCount++
	}

	// Log sync results - only say "successfully" if at least one price was synced
	if successCount > 0 {
		s.Logger.Infow("successfully synced plan to QuickBooks",
			"plan_id", plan.ID,
			"plan_name", plan.Name,
			"total_prices", len(prices),
			"successfully_synced", successCount)
	} else {
		s.Logger.Errorw("failed to sync plan to QuickBooks - all prices failed",
			"plan_id", plan.ID,
			"plan_name", plan.Name,
			"total_prices", len(prices),
			"successfully_synced", successCount)
		return ierr.NewError("failed to sync any prices to QuickBooks").
			WithHint("All prices failed to sync. Check QuickBooks connection and token validity.").
			WithReportableDetails(map[string]interface{}{
				"plan_id":       plan.ID,
				"plan_name":     plan.Name,
				"total_prices":  len(prices),
				"success_count": successCount,
			}).
			Mark(ierr.ErrInternal)
	}

	return nil
}

// SyncPriceToQuickBooks syncs a Flexprice price to QuickBooks item.
// Creates a Service Item in QuickBooks with:
// - Item Name: {plan name}-{meter name} for usage charges, or {plan name}-Recurring-{count} for recurring charges
// - Item Description: {price_id} (as per requirements)
// - Item Type: "Service"
// - Income Account: Hardcoded to ID "79" (as per requirements)
// If item already exists (by name or mapping), creates mapping instead of creating duplicate.
func (s *ItemSyncService) SyncPriceToQuickBooks(ctx context.Context, priceItem *price.Price, plan *plan.Plan, meterItem *meter.Meter, recurringCount int) error {
	// Check if item is already synced via entity mapping
	existingItemID, err := s.getQuickBooksItemID(ctx, priceItem.ID)
	if err == nil && existingItemID != "" {
		return nil
	}

	// If error is not "not found", it's a database/infrastructure error - return it
	if err != nil && !ierr.IsNotFound(err) {
		return err
	}

	// Build item name based on charge type:
	// - Usage charge (with meter): {plan name}-{meter name}
	// - Recurring charge (without meter): {plan name}-Recurring-{count}
	var itemName string
	if meterItem != nil && meterItem.Name != "" {
		itemName = fmt.Sprintf("%s-%s", plan.Name, meterItem.Name)
	} else {
		itemName = fmt.Sprintf("%s-Recurring-%d", plan.Name, recurringCount)
	}
	// Sanitize name - remove quotes and special characters that QuickBooks doesn't allow
	itemName = s.sanitizeItemName(itemName)

	// Check if item with same name already exists in QuickBooks
	// This handles cases where item was created manually or in a previous sync
	existingItem, err := s.Client.QueryItemByName(ctx, itemName)
	if err == nil && existingItem != nil && existingItem.ID != "" {
		// Create mapping for existing item instead of creating duplicate
		if err := s.createItemMapping(ctx, priceItem.ID, existingItem.ID, existingItem.Name, plan.EnvironmentID, plan.TenantID); err != nil {
			s.Logger.Debugw("failed to create item mapping",
				"error", err,
				"price_id", priceItem.ID)
		}
		return nil
	}

	// Create new item in QuickBooks
	// Income account ID "79" is hardcoded as per requirements
	createReq := &ItemCreateRequest{
		Name:        itemName,
		Type:        "Service",
		Description: priceItem.ID, // Item Description: {price_id} as per requirements
		Active:      true,
		IncomeAccountRef: &AccountRef{
			Value: "79", // Hardcoded income account ID as per requirements
		},
	}

	itemResp, err := s.Client.CreateItem(ctx, createReq)
	if err != nil {
		return ierr.NewError("failed to create item in QuickBooks").
			WithReportableDetails(map[string]interface{}{
				"error":    err.Error(),
				"price_id": priceItem.ID,
			}).
			WithHint("Check QuickBooks API credentials and item data").
			Mark(ierr.ErrValidation)
	}

	s.Logger.Infow("successfully created item in QuickBooks",
		"plan_id", plan.ID,
		"price_id", priceItem.ID,
		"quickbooks_item_id", itemResp.ID,
		"item_name", itemResp.Name)

	// Create entity mapping
	if err := s.createItemMapping(ctx, priceItem.ID, itemResp.ID, itemResp.Name, plan.EnvironmentID, plan.TenantID); err != nil {
		s.Logger.Errorw("failed to create item mapping",
			"error", err,
			"price_id", priceItem.ID,
			"quickbooks_item_id", itemResp.ID)
		// Don't fail the entire operation, just log the error
	}

	return nil
}

// getQuickBooksItemID retrieves the QuickBooks item ID from entity mapping.
// Checks if price has already been synced to QuickBooks by looking up the mapping.
// Returns the QuickBooks item ID if mapping exists, or ErrNotFound if price hasn't been synced.
func (s *ItemSyncService) getQuickBooksItemID(ctx context.Context, priceID string) (string, error) {
	filter := types.NewEntityIntegrationMappingFilter()
	filter.EntityType = types.IntegrationEntityTypePrice
	filter.EntityID = priceID
	filter.ProviderTypes = []string{string(types.SecretProviderQuickBooks)}

	mappings, err := s.EntityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to query entity mapping for item").
			WithReportableDetails(map[string]interface{}{
				"price_id": priceID,
			}).
			Mark(ierr.ErrDatabase)
	}

	if len(mappings) == 0 {
		return "", ierr.NewError("item not synced to QuickBooks").
			WithHint("Item not synced to QuickBooks").
			WithReportableDetails(map[string]interface{}{
				"price_id": priceID,
			}).
			Mark(ierr.ErrNotFound)
	}

	return mappings[0].ProviderEntityID, nil
}

// createItemMapping creates an entity integration mapping for the item.
// Maps Flexprice price_id to QuickBooks item ID for use in invoice line items.
// Stores item name in metadata for reference and debugging.
func (s *ItemSyncService) createItemMapping(
	ctx context.Context,
	priceID string,
	quickBooksItemID string,
	itemName string,
	environmentID string,
	tenantID string,
) error {
	baseModel := types.GetDefaultBaseModel(ctx)
	baseModel.TenantID = tenantID

	mapping := &entityintegrationmapping.EntityIntegrationMapping{
		ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ENTITY_INTEGRATION_MAPPING),
		EntityType:       types.IntegrationEntityTypePrice,
		EntityID:         priceID,
		ProviderType:     string(types.SecretProviderQuickBooks),
		ProviderEntityID: quickBooksItemID,
		EnvironmentID:    environmentID,
		BaseModel:        baseModel,
		Metadata: map[string]interface{}{
			"synced_at":            time.Now().UTC().Format(time.RFC3339),
			"quickbooks_item_name": itemName,
		},
	}

	return s.EntityIntegrationMappingRepo.Create(ctx, mapping)
}

// sanitizeItemName removes special characters that QuickBooks doesn't allow in Item Name.
// QuickBooks explicitly disallows: tab, newline, colon (:), and double-quote (").
// QuickBooks accepts single quotes ('), so we don't remove them.
// This ensures item creation doesn't fail due to invalid characters.
func (s *ItemSyncService) sanitizeItemName(name string) string {
	sanitized := strings.ReplaceAll(name, ":", "")      // Remove colons (explicitly disallowed)
	sanitized = strings.ReplaceAll(sanitized, "\"", "") // Remove double quotes (explicitly disallowed)
	sanitized = strings.ReplaceAll(sanitized, "\t", "") // Remove tabs (explicitly disallowed)
	sanitized = strings.ReplaceAll(sanitized, "\n", "") // Remove newlines (explicitly disallowed)
	sanitized = strings.ReplaceAll(sanitized, "\r", "") // Remove carriage returns
	return strings.TrimSpace(sanitized)
}
