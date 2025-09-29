package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

type StripePlanService interface {
	CreatePlan(ctx context.Context, planID string) (string, error)
	GetPlan(ctx context.Context, id string) (*dto.PlanResponse, error)
	UpdatePlan(ctx context.Context, id string, req dto.UpdatePlanRequest) (*dto.PlanResponse, error)
	DeletePlan(ctx context.Context, id string) error
}

type stripePlanService struct {
	ServiceParams
}

func NewStripePlanService(params ServiceParams) *stripePlanService {
	return &stripePlanService{
		ServiceParams: params,
	}
}

func (s *stripePlanService) CreatePlan(ctx context.Context, planID string) (*dto.PlanResponse, error) {
	// Check if the plan already exists in Entity Mapping
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:        types.IntegrationEntityTypePlan,
		ProviderTypes:     []string{"stripe"},
		ProviderEntityIDs: []string{planID},
	}

	existingMappings, err := s.EntityIntegrationMappingRepo.List(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to check for existing plan mapping").
			Mark(ierr.ErrInternal)
	}

	// If yes: return the plan
	if len(existingMappings) > 0 {
		existingMapping := existingMappings[0]

		// Get the plan using the existing mapping's entity ID
		planService := NewPlanService(s.ServiceParams)
		planResponse, err := planService.GetPlan(ctx, existingMapping.EntityID)
		if err != nil {
			return nil, ierr.WithError(err).
				WithHint("Failed to retrieve existing plan").
				Mark(ierr.ErrInternal)
		}

		return planResponse, nil
	}

	// If not: Create a PLAN and ADDON empty
	// Create a basic plan with minimal configuration
	createPlanReq := dto.CreatePlanRequest{
		Name:         "Stripe Plan " + planID,
		Description:  "Plan imported from Stripe",
		LookupKey:    planID,
		Prices:       []dto.CreatePlanPriceRequest{},       // Empty prices initially
		Entitlements: []dto.CreatePlanEntitlementRequest{}, // Empty entitlements initially
		CreditGrants: []dto.CreateCreditGrantRequest{},     // Empty credit grants initially
		Metadata: types.Metadata{
			"source":         "stripe",
			"stripe_plan_id": planID,
		},
	}

	// Validate the request
	if err := createPlanReq.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid plan data for Stripe plan creation").
			Mark(ierr.ErrValidation)
	}

	// Create the plan using the plan service
	planService := NewPlanService(s.ServiceParams)
	createPlanResp, err := planService.CreatePlan(ctx, createPlanReq)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create plan").
			Mark(ierr.ErrInternal)
	}

	// Create entity mapping for plan
	entityMappingReq := dto.CreateEntityIntegrationMappingRequest{
		EntityID:         createPlanResp.ID,
		EntityType:       types.IntegrationEntityTypePlan,
		ProviderType:     "stripe",
		ProviderEntityID: planID,
		Metadata: map[string]interface{}{
			"created_via":    "stripe_plan_service",
			"stripe_plan_id": planID,
		},
	}

	// Validate the mapping request
	if err := entityMappingReq.Validate(); err != nil {
		return nil, ierr.WithError(err).
			WithHint("Invalid entity mapping data").
			Mark(ierr.ErrValidation)
	}

	// Create the entity integration mapping
	entityMappingService := NewEntityIntegrationMappingService(s.ServiceParams)
	_, err = entityMappingService.CreateEntityIntegrationMapping(ctx, entityMappingReq)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to create entity integration mapping").
			Mark(ierr.ErrInternal)
	}

	return nil, nil
}

func (s *stripePlanService) GetPlan(ctx context.Context, id string) (*dto.PlanResponse, error) {
	return nil, nil
}

func (s *stripePlanService) UpdatePlan(ctx context.Context, id string, req dto.UpdatePlanRequest) (*dto.PlanResponse, error) {
	return nil, nil
}

func (s *stripePlanService) DeletePlan(ctx context.Context, id string) error {
	return nil
}
