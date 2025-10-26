package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/interfaces"
	"github.com/flexprice/flexprice/internal/types"
)

type CostsheetV2Service = interfaces.CostsheetV2Service

type costsheetV2Service struct {
	ServiceParams
}

func NewCostsheetV2Service(
	params ServiceParams,
) CostsheetV2Service {
	return &costsheetV2Service{
		ServiceParams: params,
	}
}

func (s *costsheetV2Service) CreateCostsheetV2(ctx context.Context, req dto.CreateCostsheetV2Request) (*dto.CreateCostsheetV2Response, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid costsheet v2 data provided").
			Mark(ierr.ErrValidation)
	}

	costsheet := req.ToCostsheetV2(ctx)

	// Validate the domain model
	if err := costsheet.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid costsheet v2 data").
			Mark(ierr.ErrValidation)
	}

	// Start a transaction to create costsheet v2
	err := s.DB.WithTx(ctx, func(ctx context.Context) error {
		// Check if a costsheet with the same name already exists in the same tenant and environment
		existing, err := s.CostSheetV2Repo.GetByName(ctx, costsheet.TenantID, costsheet.EnvironmentID, costsheet.Name)
		if err != nil && !ierr.IsNotFound(err) {
			return ierr.WithError(err).
				WithHint("Failed to check for existing costsheet v2").
				Mark(ierr.ErrDatabase)
		}
		if existing != nil {
			return ierr.NewError("costsheet v2 with this name already exists").
				WithHint("A costsheet v2 with this name already exists in the same tenant and environment").
				WithReportableDetails(map[string]any{
					"name":           costsheet.Name,
					"tenant_id":      costsheet.TenantID,
					"environment_id": costsheet.EnvironmentID,
				}).
				Mark(ierr.ErrAlreadyExists)
		}

		// Create the costsheet v2
		if err := s.CostSheetV2Repo.Create(ctx, costsheet); err != nil {
			return ierr.WithError(err).
				WithHint("Failed to create costsheet v2").
				Mark(ierr.ErrDatabase)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	response := &dto.CreateCostsheetV2Response{
		CostsheetV2: dto.ToCostsheetV2Response(costsheet),
	}

	return response, nil
}

func (s *costsheetV2Service) GetCostsheetV2(ctx context.Context, id string) (*dto.GetCostsheetV2Response, error) {
	if id == "" {
		return nil, ierr.NewError("costsheet v2 ID is required").
			WithHint("Please provide a valid costsheet v2 ID").
			Mark(ierr.ErrValidation)
	}

	costsheet, err := s.CostSheetV2Repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Fetch prices associated with this costsheet v2
	priceService := NewPriceService(s.ServiceParams)
	pricesResponse, err := priceService.GetPricesByCostsheetV2ID(ctx, id)
	if err != nil {
		return nil, err
	}

	response := &dto.GetCostsheetV2Response{
		CostsheetV2: dto.ToCostsheetV2ResponseWithPrices(costsheet, pricesResponse.Items),
	}

	return response, nil
}

func (s *costsheetV2Service) GetCostsheetV2s(ctx context.Context, filter *types.CostsheetV2Filter) (*dto.ListCostsheetV2Response, error) {
	if filter == nil {
		filter = types.NewCostsheetV2Filter()
	}

	if err := filter.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation)
	}

	// Fetch costsheet v2 records
	costsheets, err := s.CostSheetV2Repo.List(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve costsheet v2 records").
			Mark(ierr.ErrDatabase)
	}

	// Get count
	count, err := s.CostSheetV2Repo.Count(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Build response
	response := &dto.ListCostsheetV2Response{
		Items: make([]*dto.CostsheetV2Response, len(costsheets)),
		Pagination: &types.PaginationResponse{
			Total:  int(count),
			Limit:  filter.GetLimit(),
			Offset: filter.GetOffset(),
		},
	}

	if len(costsheets) == 0 {
		return response, nil
	}

	// Initialize response items without prices first
	for i, costsheet := range costsheets {
		response.Items[i] = dto.ToCostsheetV2Response(costsheet)
	}

	// Check if prices should be expanded
	if filter.GetExpand().Has(types.ExpandPrices) && len(costsheets) > 0 {
		// Collect all costsheet IDs
		costsheetIDs := make([]string, len(costsheets))
		for i, cs := range costsheets {
			costsheetIDs[i] = cs.ID
		}

		// Fetch all prices for these costsheets in one go
		priceService := NewPriceService(s.ServiceParams)
		priceFilter := types.NewNoLimitPriceFilter().
			WithEntityIDs(costsheetIDs).
			WithStatus(types.StatusPublished).
			WithEntityType(types.PRICE_ENTITY_TYPE_COSTSHEET_V2)

		// If meters should be expanded, propagate the expansion to prices
		if filter.GetExpand().Has(types.ExpandMeters) {
			priceFilter = priceFilter.WithExpand(string(types.ExpandMeters))
		}

		pricesResponse, err := priceService.GetPrices(ctx, priceFilter)
		if err != nil {
			s.Logger.Warnw("Failed to fetch prices for costsheet v2 list", "error", err)
		} else {
			// Group prices by costsheet v2 ID
			pricesByCostsheet := make(map[string][]*dto.PriceResponse)
			for _, price := range pricesResponse.Items {
				if price.EntityID != "" {
					pricesByCostsheet[price.EntityID] = append(pricesByCostsheet[price.EntityID], price)
				}
			}

			// Build response with expanded fields
			for i, costsheet := range costsheets {
				// Add prices if available
				if prices, ok := pricesByCostsheet[costsheet.ID]; ok {
					response.Items[i].Prices = prices
				}
			}
		}
	}

	return response, nil
}

func (s *costsheetV2Service) UpdateCostsheetV2(ctx context.Context, id string, req dto.UpdateCostsheetV2Request) (*dto.UpdateCostsheetV2Response, error) {
	if id == "" {
		return nil, ierr.NewError("costsheet v2 ID is required").
			WithHint("Costsheet v2 ID is required").
			Mark(ierr.ErrValidation)
	}

	// Validate request
	if err := req.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid update data provided").
			Mark(ierr.ErrValidation)
	}

	// Get the existing costsheet v2
	costsheet, err := s.CostSheetV2Repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Start a transaction to update costsheet v2
	err = s.DB.WithTx(ctx, func(ctx context.Context) error {
		// Update the costsheet v2
		req.UpdateCostsheetV2(costsheet, ctx)

		// Save the updated costsheet v2
		if err := s.CostSheetV2Repo.Update(ctx, costsheet); err != nil {
			return ierr.WithError(err).
				WithHint("Failed to update costsheet v2").
				Mark(ierr.ErrDatabase)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	response := &dto.UpdateCostsheetV2Response{
		CostsheetV2: dto.ToCostsheetV2Response(costsheet),
	}

	return response, nil
}

func (s *costsheetV2Service) DeleteCostsheetV2(ctx context.Context, id string) (*dto.DeleteCostsheetV2Response, error) {
	if id == "" {
		return nil, ierr.NewError("costsheet v2 ID is required").
			WithHint("Costsheet v2 ID is required").
			Mark(ierr.ErrValidation)
	}

	// Start a transaction to delete costsheet v2
	err := s.DB.WithTx(ctx, func(ctx context.Context) error {
		// Soft delete the costsheet v2
		if err := s.CostSheetV2Repo.Delete(ctx, id); err != nil {
			return ierr.WithError(err).
				WithHint("Failed to delete costsheet v2").
				Mark(ierr.ErrDatabase)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	response := &dto.DeleteCostsheetV2Response{
		Message: "Costsheet v2 deleted successfully",
		ID:      id,
	}

	return response, nil
}
