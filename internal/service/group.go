package service

import (
	"context"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/internal/api/dto"
	domainGroup "github.com/flexprice/flexprice/internal/domain/group"
	"github.com/flexprice/flexprice/internal/domain/price"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
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
	validators map[types.GroupEntityType]EntityValidator
	log        *logger.Logger
}

func NewGroupService(params ServiceParams, priceRepo price.Repository, log *logger.Logger) GroupService {
	validators := make(map[types.GroupEntityType]EntityValidator)
	validators[types.GroupEntityTypePrice] = NewPriceValidator(priceRepo, log)

	return &groupService{
		ServiceParams: params,
		validators:    validators,
		log:           log,
	}
}

// CreateGroup creates a new group with associated entities
func (s *groupService) CreateGroup(ctx context.Context, req dto.CreateGroupRequest) (*dto.GroupResponse, error) {

	if err := req.Validate(); err != nil {
		s.log.Warn("invalid group creation request",
			"error", err,
			"name", req.Name,
		)
		return nil, err
	}

	var result *dto.GroupResponse

	// Execute all operations within a transaction for atomicity
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		// Check if group with same lookup key already exists (idempotency)
		existingByLookup, err := s.GroupRepo.GetByLookupKey(txCtx, req.LookupKey)
		if err != nil && !ent.IsNotFound(err) {
			return err
		}
		if existingByLookup != nil {
			// Return existing group (idempotent behavior)
			s.log.Info("returning existing group for idempotent request",
				"group_id", existingByLookup.ID,
				"lookup_key", req.LookupKey,
			)
			entityIDs, err := s.getAssociatedEntities(txCtx, existingByLookup.EntityType, existingByLookup.ID)
			if err != nil {
				return err
			}
			result = dto.NewGroupResponse(existingByLookup, entityIDs)
			return nil
		}

		// Convert request to group domain object
		groupObj, err := req.ToGroup(txCtx)
		if err != nil {
			return err
		}

		// Validate and associate entities based on type
		var entityIDs []string
		if len(req.EntityIDs) > 0 {
			s.log.Debug("validating entities for group",
				"group_id", groupObj.ID,
				"entity_count", len(req.EntityIDs),
			)
			entityIDs, err = s.validateEntities(txCtx, groupObj.EntityType, req.EntityIDs, "")
			if err != nil {
				s.log.Error("entity validation failed",
					"error", err,
					"group_id", groupObj.ID,
					"entity_type", groupObj.EntityType,
				)
				return err
			}
		}

		// Create group
		if err := s.GroupRepo.Create(txCtx, groupObj); err != nil {
			s.log.Error("failed to create group",
				"error", err,
				"group_id", groupObj.ID,
				"name", req.Name,
			)
			return err
		}

		// Associate entities with group if provided
		if len(req.EntityIDs) > 0 {
			if err := s.associateEntities(txCtx, groupObj.EntityType, groupObj.ID, req.EntityIDs); err != nil {
				s.log.Error("failed to associate entities with group",
					"error", err,
					"group_id", groupObj.ID,
					"entity_count", len(req.EntityIDs),
				)
				return err
			}
		}

		result = dto.NewGroupResponse(groupObj, entityIDs)
		return nil
	})

	if err != nil {
		s.log.Error("group creation failed", "error", err, "name", req.Name)
		return nil, err
	}

	s.log.Info("group created successfully",
		"group_id", result.ID,
		"name", req.Name,
		"entity_count", len(result.EntityIDs),
	)

	return result, nil
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
		s.log.Warn("invalid update request", "error", err, "group_id", id)
		return nil, err
	}

	var result *dto.GroupResponse

	// Execute all operations within a transaction for atomicity
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		groupObj, err := s.GroupRepo.Get(txCtx, id)
		if err != nil {
			s.log.Error("failed to fetch group for update", "error", err, "group_id", id)
			return err
		}

		// Update name if provided
		if req.Name != nil {
			groupObj.Name = *req.Name
		}

		// Update group
		if err := s.GroupRepo.Update(txCtx, groupObj); err != nil {
			s.log.Error("failed to update group", "error", err, "group_id", id)
			return err
		}

		// Update associated entities if provided
		var entityIDs []string
		if req.EntityIDs != nil {
			// Override approach: Replace all existing associations with new ones

			// 1. Disassociate ALL current entities
			currentEntityIDs, err := s.getAssociatedEntities(txCtx, groupObj.EntityType, id)
			if err != nil {
				return err
			}
			if len(currentEntityIDs) > 0 {
				if err := s.disassociateEntities(txCtx, groupObj.EntityType, currentEntityIDs); err != nil {
					return err
				}
			}

			// 2. Validate and associate NEW entities
			if len(req.EntityIDs) > 0 {
				entityIDs, err = s.validateEntities(txCtx, groupObj.EntityType, req.EntityIDs, id)
				if err != nil {
					return err
				}
				if err := s.associateEntities(txCtx, groupObj.EntityType, id, entityIDs); err != nil {
					return err
				}
			}
		}

		result = dto.NewGroupResponse(groupObj, entityIDs)
		return nil
	})

	if err != nil {
		s.log.Error("group update failed", "error", err, "group_id", id)
		return nil, err
	}

	s.log.Info("group updated successfully", "group_id", id)
	return result, nil
}

// DeleteGroup deletes a group (entity associations are automatically removed by foreign key constraint)
func (s *groupService) DeleteGroup(ctx context.Context, id string) error {

	// Execute all operations within a transaction for atomicity
	err := s.DB.WithTx(ctx, func(txCtx context.Context) error {
		groupObj, err := s.GroupRepo.Get(txCtx, id)
		if err != nil {
			s.log.Error("failed to fetch group for deletion", "error", err, "group_id", id)
			return err
		}

		entityIDs, err := s.getAssociatedEntities(txCtx, groupObj.EntityType, id)
		if err != nil {
			s.log.Error("failed to get associated entities", "error", err, "group_id", id)
			return err
		}
		if len(entityIDs) > 0 {
			s.log.Debug("disassociating entities before deletion", "group_id", id, "entity_count", len(entityIDs))
			if err := s.disassociateEntities(txCtx, groupObj.EntityType, entityIDs); err != nil {
				s.log.Error("failed to disassociate entities", "error", err, "group_id", id)
				return err
			}
		}

		return s.GroupRepo.Delete(txCtx, id)
	})

	if err != nil {
		s.log.Error("group deletion failed", "error", err, "group_id", id)
		return err
	}

	s.log.Info("group deleted successfully", "group_id", id)
	return nil
}

// ListGroups retrieves groups with optional filtering
func (s *groupService) ListGroups(ctx context.Context, filter *types.GroupFilter) (*dto.ListGroupsResponse, error) {
	s.log.Debug("listing groups", "filter", filter)

	groups, err := s.GroupRepo.List(ctx, filter)
	if err != nil {
		s.log.Error("failed to list groups", "error", err)
		return nil, err
	}

	if len(groups) == 0 {
		return &dto.ListGroupsResponse{
			Groups: []*dto.GroupResponse{},
			Total:  0,
		}, nil
	}

	// Get associated entities for all groups in bulk (single query instead of N queries)
	entityMap, err := s.getAssociatedEntitiesBulk(ctx, groups)
	if err != nil {
		s.log.Error("failed to get associated entities in bulk", "error", err)
		return nil, err
	}

	responses := make([]*dto.GroupResponse, len(groups))
	for i, groupObj := range groups {
		entityIDs := entityMap[groupObj.ID] // O(1) lookup
		responses[i] = dto.NewGroupResponse(groupObj, entityIDs)
	}

	s.log.Debug("groups listed successfully", "group_count", len(groups))
	return &dto.ListGroupsResponse{
		Groups: responses,
		Total:  len(responses),
	}, nil
}

// validateEntities validates entities using the appropriate validator
func (s *groupService) validateEntities(ctx context.Context, entityType types.GroupEntityType, entityIDs []string, excludeGroupID string) ([]string, error) {
	validator, exists := s.validators[entityType]
	if !exists {
		s.log.Error("unsupported entity type", "entity_type", entityType)
		return nil, ierr.NewError("unsupported entity type").
			WithHint("Only price entities are currently supported").
			Mark(ierr.ErrValidation)
	}

	return validator.Validate(ctx, entityIDs, excludeGroupID)
}

// getAssociatedEntities gets entities associated with a group using the appropriate validator
func (s *groupService) getAssociatedEntities(ctx context.Context, entityType types.GroupEntityType, groupID string) ([]string, error) {
	validator, exists := s.validators[entityType]
	if !exists {
		s.log.Error("unsupported entity type", "entity_type", entityType)
		return nil, ierr.NewError("unsupported entity type").
			WithHint("Only price entities are currently supported").
			Mark(ierr.ErrValidation)
	}

	return validator.GetAssociated(ctx, groupID)
}

// getAssociatedEntitiesBulk gets entities associated with multiple groups in bulk
func (s *groupService) getAssociatedEntitiesBulk(ctx context.Context, groups []*domainGroup.Group) (map[string][]string, error) {
	if len(groups) == 0 {
		return make(map[string][]string), nil
	}

	// Group by entity type to handle different validators
	groupsByType := make(map[types.GroupEntityType][]string)
	for _, group := range groups {
		groupsByType[group.EntityType] = append(groupsByType[group.EntityType], group.ID)
	}

	// Fetch entities for each type in bulk
	result := make(map[string][]string)
	for entityType, groupIDs := range groupsByType {
		validator, exists := s.validators[entityType]
		if !exists {
			s.log.Error("unsupported entity type in bulk fetch", "entity_type", entityType)
			return nil, ierr.NewError("unsupported entity type").
				WithHint("Only price entities are currently supported").
				Mark(ierr.ErrValidation)
		}

		typeResult, err := validator.GetAssociatedBulk(ctx, groupIDs)
		if err != nil {
			return nil, err
		}

		// Merge results
		for groupID, entityIDs := range typeResult {
			result[groupID] = entityIDs
		}
	}

	return result, nil
}

// associateEntities associates entities with a group using the appropriate validator
func (s *groupService) associateEntities(ctx context.Context, entityType types.GroupEntityType, groupID string, entityIDs []string) error {
	validator, exists := s.validators[entityType]
	if !exists {
		s.log.Error("unsupported entity type", "entity_type", entityType)
		return ierr.NewError("unsupported entity type").
			WithHint("Only price entities are currently supported").
			Mark(ierr.ErrValidation)
	}

	return validator.Associate(ctx, groupID, entityIDs)
}

// disassociateEntities removes group associations from entities using the appropriate validator
func (s *groupService) disassociateEntities(ctx context.Context, entityType types.GroupEntityType, entityIDs []string) error {
	validator, exists := s.validators[entityType]
	if !exists {
		s.log.Error("unsupported entity type", "entity_type", entityType)
		return ierr.NewError("unsupported entity type").
			WithHint("Only price entities are currently supported").
			Mark(ierr.ErrValidation)
	}

	return validator.Disassociate(ctx, entityIDs)
}
