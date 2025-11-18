package chargebee

import (
	"context"
	"fmt"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/domain/entityintegrationmapping"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
)

// ChargebeePlanSyncService defines the interface for Chargebee plan synchronization
type ChargebeePlanSyncService interface {
	SyncPlanToChargebee(ctx context.Context, plan *ent.Plan, prices []*ent.Price) error
}

// PlanSyncService handles synchronization of FlexPrice plans to Chargebee
type PlanSyncService struct {
	client                       ChargebeeClient
	itemFamilyService            ChargebeeItemFamilyService
	itemService                  ChargebeeItemService
	itemPriceService             ChargebeeItemPriceService
	entityIntegrationMappingRepo entityintegrationmapping.Repository
	logger                       *logger.Logger
}

// NewPlanSyncService creates a new Chargebee plan sync service
func NewPlanSyncService(
	client ChargebeeClient,
	itemFamilyService ChargebeeItemFamilyService,
	itemService ChargebeeItemService,
	itemPriceService ChargebeeItemPriceService,
	entityIntegrationMappingRepo entityintegrationmapping.Repository,
	logger *logger.Logger,
) ChargebeePlanSyncService {
	return &PlanSyncService{
		client:                       client,
		itemFamilyService:            itemFamilyService,
		itemService:                  itemService,
		itemPriceService:             itemPriceService,
		entityIntegrationMappingRepo: entityIntegrationMappingRepo,
		logger:                       logger,
	}
}

// SyncPlanToChargebee syncs a FlexPrice plan and its prices to Chargebee
func (s *PlanSyncService) SyncPlanToChargebee(ctx context.Context, plan *ent.Plan, prices []*ent.Price) error {
	s.logger.Infow("syncing plan to Chargebee",
		"plan_id", plan.ID,
		"plan_name", plan.Name,
		"prices_count", len(prices))

	// Step 1: Get or select the latest item family
	itemFamily, err := s.itemFamilyService.GetLatestItemFamily(ctx)
	if err != nil {
		s.logger.Errorw("failed to get item family from Chargebee",
			"plan_id", plan.ID,
			"error", err)
		return ierr.WithError(err).
			WithHint("Failed to get item family from Chargebee. Please create an item family first").
			Mark(ierr.ErrNotFound)
	}

	s.logger.Infow("using item family for plan",
		"plan_id", plan.ID,
		"item_family_id", itemFamily.ID,
		"item_family_name", itemFamily.Name)

	// Step 2: Create a charge-type item for the plan
	// Format: charge_{uuid}
	uniqueUUID := types.GenerateUUID()
	itemID := fmt.Sprintf("charge_%s", uniqueUUID)

	itemReq := &ItemCreateRequest{
		ID:              itemID,
		Name:            itemID,   // Name matches ID format
		Type:            "charge", // Charge type for one-time/recurring charges
		ItemFamilyID:    itemFamily.ID,
		Description:     plan.Description,
		ExternalName:    itemID,
		EnabledInPortal: true,
	}

	item, err := s.itemService.CreateItem(ctx, itemReq)
	if err != nil {
		s.logger.Errorw("failed to create item in Chargebee",
			"plan_id", plan.ID,
			"item_id", itemID,
			"error", err)
		return ierr.WithError(err).
			WithHint("Failed to create item in Chargebee").
			Mark(ierr.ErrValidation)
	}

	s.logger.Infow("successfully created item in Chargebee",
		"plan_id", plan.ID,
		"item_id", item.ID)

	// Step 3: Create an item price for each price in the plan
	for _, price := range prices {
		// Item price ID should just be the FlexPrice price ID
		itemPriceID := price.ID

		// Determine pricing model based on price type
		pricingModel := "flat_fee"
		if price.Type == string(types.PRICE_TYPE_USAGE) {
			pricingModel = "per_unit"
		}

		itemPriceReq := &ItemPriceCreateRequest{
			ID:           itemPriceID,
			ItemID:       item.ID,
			Name:         itemPriceID,
			ExternalName: itemPriceID,
			PricingModel: pricingModel,
			Price:        int64(price.Amount * 100), // Convert to cents
			CurrencyCode: price.Currency,
			Description:  plan.Description,
		}

		itemPrice, err := s.itemPriceService.CreateItemPrice(ctx, itemPriceReq)
		if err != nil {
			s.logger.Errorw("failed to create item price in Chargebee",
				"plan_id", plan.ID,
				"price_id", price.ID,
				"item_price_id", itemPriceID,
				"error", err)
			// Continue with other prices even if one fails
			continue
		}

		s.logger.Infow("successfully created item price in Chargebee",
			"plan_id", plan.ID,
			"price_id", price.ID,
			"item_price_id", itemPrice.ID,
			"amount", itemPrice.Price,
			"currency", itemPrice.CurrencyCode)

		// Save entity mapping for price -> item_price
		// Only one entry per price: FlexPrice price ID -> Chargebee item price ID
		mapping := &entityintegrationmapping.EntityIntegrationMapping{
			ID:               types.GenerateUUIDWithPrefix(types.UUID_PREFIX_ENTITY_INTEGRATION_MAPPING),
			EntityType:       types.IntegrationEntityTypeItemPrice,
			EntityID:         price.ID,
			ProviderType:     string(types.SecretProviderChargebee),
			ProviderEntityID: itemPrice.ID,
			EnvironmentID:    price.EnvironmentID,
			BaseModel:        types.GetDefaultBaseModel(ctx),
			Metadata: map[string]interface{}{
				"chargebee_charge_item_id": itemID,
			},
		}
		// Override tenant_id from price (price has TenantID in BaseModel)
		mapping.TenantID = price.TenantID

		err = s.entityIntegrationMappingRepo.Create(ctx, mapping)
		if err != nil {
			s.logger.Errorw("failed to save price entity mapping",
				"price_id", price.ID,
				"chargebee_item_price_id", itemPrice.ID,
				"error", err)
			// Don't fail the entire operation, just log the error
		}
	}

	s.logger.Infow("successfully synced plan to Chargebee",
		"plan_id", plan.ID,
		"chargebee_item_id", item.ID,
		"prices_synced", len(prices))

	return nil
}
