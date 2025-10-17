package service

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/api/dto"
	"github.com/flexprice/flexprice/internal/domain/price"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/types"
)

// GroupService interface defines the business logic for group management
type GroupService interface {
	CreateGroup(ctx context.Context, req dto.CreateGroupRequest) (*dto.GroupResponse, error)
	GetGroup(ctx context.Context, id string) (*dto.GroupResponse, error)
	UpdateGroup(ctx context.Context, id string, req dto.UpdateGroupRequest) (*dto.GroupResponse, error)
	DeleteGroup(ctx context.Context, id string) error
	ListGroups(ctx context.Context, filter *types.GroupFilter) (*dto.ListGroupsResponse, error)
}

type groupService struct {
	ServiceParams
	priceRepo price.Repository
}

func NewGroupService(params ServiceParams, priceRepo price.Repository) GroupService {
	return &groupService{
		ServiceParams: params,
		priceRepo:     priceRepo,
	}
}

// CreateGroup creates a new group with associated entities
func (s *groupService) CreateGroup(ctx context.Context, req dto.CreateGroupRequest) (*dto.GroupResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Convert request to group domain object
	groupObj, err := req.ToGroup(ctx)
	if err != nil {
		return nil, err
	}

	// Check if group name already exists for this entity type
	existingGroup, err := s.GroupRepo.GetByName(ctx, req.Name)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if existingGroup != nil && existingGroup.EntityType == groupObj.EntityType {
		return nil, ierr.NewError("group name already exists").
			WithHint("A group with this name already exists for this entity type").
			Mark(ierr.ErrValidation)
	}

	// Validate and associate entities based on type
	var entityIDs []string
	if len(req.EntityIDs) > 0 {
		entityIDs, err = s.validateAndAssociateEntities(ctx, groupObj.EntityType, req.EntityIDs, "")
		if err != nil {
			return nil, err
		}
	}

	// Create group
	if err := s.GroupRepo.Create(ctx, groupObj); err != nil {
		return nil, err
	}

	// Associate entities with group if provided
	if len(req.EntityIDs) > 0 {
		if err := s.associateEntities(ctx, groupObj.EntityType, groupObj.ID, req.EntityIDs); err != nil {
			return nil, err
		}
	}

	return dto.NewGroupResponse(groupObj, entityIDs), nil
}

// GetGroup retrieves a group by ID
func (s *groupService) GetGroup(ctx context.Context, id string) (*dto.GroupResponse, error) {
	groupObj, err := s.GroupRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get associated entities based on type
	entityIDs, err := s.getAssociatedEntities(ctx, groupObj.EntityType, id)
	if err != nil {
		return nil, err
	}

	return dto.NewGroupResponse(groupObj, entityIDs), nil
}

// UpdateGroup updates a group's details and associated entities
func (s *groupService) UpdateGroup(ctx context.Context, id string, req dto.UpdateGroupRequest) (*dto.GroupResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	groupObj, err := s.GroupRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update name if provided
	if req.Name != nil {
		// Check if new name already exists for this entity type
		existingGroup, err := s.GroupRepo.GetByName(ctx, *req.Name)
		if err != nil && !ent.IsNotFound(err) {
			return nil, err
		}
		if existingGroup != nil && existingGroup.ID != id && existingGroup.EntityType == groupObj.EntityType {
			return nil, ierr.NewError("group name already exists").
				WithHint("A group with this name already exists for this entity type").
				Mark(ierr.ErrValidation)
		}
		groupObj.Name = *req.Name
	}

	// Update group
	if err := s.GroupRepo.Update(ctx, groupObj); err != nil {
		return nil, err
	}

	// Update associated entities if provided
	var entityIDs []string
	if req.EntityIDs != nil {
		// Override approach: Replace all existing associations with new ones

		// 1. Disassociate ALL current entities
		currentEntityIDs, err := s.getAssociatedEntities(ctx, groupObj.EntityType, id)
		if err != nil {
			return nil, err
		}
		if len(currentEntityIDs) > 0 {
			if err := s.disassociateEntities(ctx, groupObj.EntityType, currentEntityIDs); err != nil {
				return nil, err
			}
		}

		// 2. Validate and associate NEW entities
		if len(req.EntityIDs) > 0 {
			entityIDs, err = s.validateAndAssociateEntities(ctx, groupObj.EntityType, req.EntityIDs, id)
			if err != nil {
				return nil, err
			}
			if err := s.associateEntities(ctx, groupObj.EntityType, id, entityIDs); err != nil {
				return nil, err
			}
		}
	}

	return dto.NewGroupResponse(groupObj, entityIDs), nil
}

// DeleteGroup deletes a group (entity associations are automatically removed by foreign key constraint)
func (s *groupService) DeleteGroup(ctx context.Context, id string) error {
	// Verify group exists
	_, err := s.GroupRepo.Get(ctx, id)
	if err != nil {
		return err
	}

	groupObj, err := s.GroupRepo.Get(ctx, id)
	if err != nil {
		return err
	}

	entityIDs, err := s.getAssociatedEntities(ctx, groupObj.EntityType, id)
	if err != nil {
		return err
	}
	if len(entityIDs) > 0 {
		if err := s.disassociateEntities(ctx, groupObj.EntityType, entityIDs); err != nil {
			return err
		}
	}

	return s.GroupRepo.Delete(ctx, id)
}

// ListGroups retrieves groups with optional filtering
func (s *groupService) ListGroups(ctx context.Context, filter *types.GroupFilter) (*dto.ListGroupsResponse, error) {
	groups, err := s.GroupRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	responses := make([]*dto.GroupResponse, len(groups))
	for i, groupObj := range groups {
		// Get associated entities for each group
		entityIDs, err := s.getAssociatedEntities(ctx, groupObj.EntityType, groupObj.ID)
		if err != nil {
			return nil, err
		}
		responses[i] = dto.NewGroupResponse(groupObj, entityIDs)
	}

	return &dto.ListGroupsResponse{
		Groups: responses,
		Total:  len(responses),
	}, nil
}

// validateAndAssociateEntities validates entities and associates them with group
func (s *groupService) validateAndAssociateEntities(ctx context.Context, entityType types.GroupEntityType, entityIDs []string, excludeGroupID string) ([]string, error) {
	switch entityType {
	case types.GroupEntityTypePrice:
		return s.validateAndAssociatePrices(ctx, entityIDs, excludeGroupID)
	default:
		return nil, ierr.NewError("unsupported entity type").
			WithHint("Only price entities are currently supported").
			Mark(ierr.ErrValidation)
	}
}

// getAssociatedEntities gets entities associated with a group
func (s *groupService) getAssociatedEntities(ctx context.Context, entityType types.GroupEntityType, groupID string) ([]string, error) {
	switch entityType {
	case types.GroupEntityTypePrice:
		return s.getAssociatedPrices(ctx, groupID)
	default:
		return nil, ierr.NewError("unsupported entity type").
			WithHint("Only price entities are currently supported").
			Mark(ierr.ErrValidation)
	}
}

// associateEntities associates entities with a group
func (s *groupService) associateEntities(ctx context.Context, entityType types.GroupEntityType, groupID string, entityIDs []string) error {
	switch entityType {
	case types.GroupEntityTypePrice:
		return s.associatePrices(ctx, groupID, entityIDs)
	default:
		return ierr.NewError("unsupported entity type").
			WithHint("Only price entities are currently supported").
			Mark(ierr.ErrValidation)
	}
}

// disassociateEntities removes group associations from entities
func (s *groupService) disassociateEntities(ctx context.Context, entityType types.GroupEntityType, entityIDs []string) error {
	switch entityType {
	case types.GroupEntityTypePrice:
		return s.disassociatePrices(ctx, entityIDs)
	default:
		return ierr.NewError("unsupported entity type").
			WithHint("Only price entities are currently supported").
			Mark(ierr.ErrValidation)
	}
}

// Price-specific methods
func (s *groupService) validateAndAssociatePrices(ctx context.Context, priceIDs []string, excludeGroupID string) ([]string, error) {
	// Validate prices exist
	count, err := s.priceRepo.CountByIDs(ctx, priceIDs)
	if err != nil {
		return nil, err
	}
	if count != len(priceIDs) {
		return nil, ierr.NewError("one or more prices not found").
			WithHint("One or more price IDs are invalid").
			Mark(ierr.ErrValidation)
	}

	// Check if any prices are already in other groups
	notInGroupCount, err := s.priceRepo.CountNotInGroup(ctx, priceIDs, excludeGroupID)
	if err != nil {
		return nil, err
	}
	if notInGroupCount > 0 {
		return nil, ierr.NewError("one or more prices are already in another group").
			WithHint("One or more prices are already assigned to another group").
			Mark(ierr.ErrValidation)
	}

	// Associate prices with group - we'll do this after group creation
	// For now, just return the price IDs to be associated later

	return priceIDs, nil
}

func (s *groupService) getAssociatedPrices(ctx context.Context, groupID string) ([]string, error) {
	prices, err := s.priceRepo.GetInGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	priceIDs := make([]string, len(prices))
	for i, price := range prices {
		priceIDs[i] = price.ID
	}
	return priceIDs, nil
}

func (s *groupService) associatePrices(ctx context.Context, groupID string, priceIDs []string) error {
	return s.priceRepo.UpdateGroupIDBulk(ctx, priceIDs, &groupID)
}

func (s *groupService) disassociatePrices(ctx context.Context, priceIDs []string) error {
	return s.priceRepo.ClearGroupIDBulk(ctx, priceIDs)
}
