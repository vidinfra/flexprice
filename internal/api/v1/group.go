package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
)

type GroupHandler struct {
	service service.GroupService
	log     *logger.Logger
}

func NewGroupHandler(service service.GroupService, log *logger.Logger) *GroupHandler {
	return &GroupHandler{service: service, log: log}
}

// @Summary Create a group
// @Description Create a new group for organizing entities (prices, plans, customers, etc.)
// @Tags Groups
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param group body dto.CreateGroupRequest true "Group"
// @Success 201 {object} dto.GroupResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /groups [post]
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req dto.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.CreateGroup(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// @Summary Get a group
// @Description Get a group by ID
// @Tags Groups
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Group ID"
// @Success 200 {object} dto.GroupResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /groups/{id} [get]
func (h *GroupHandler) GetGroup(c *gin.Context) {
	id := c.Param("id")

	resp, err := h.service.GetGroup(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Delete a group
// @Description Delete a group and remove all entity associations
// @Tags Groups
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Group ID"
// @Success 204 "No Content"
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /groups/{id} [delete]
func (h *GroupHandler) DeleteGroup(c *gin.Context) {
	id := c.Param("id")

	err := h.service.DeleteGroup(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List groups
// @Description List groups with optional filtering
// @Tags Groups
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param entity_type query string false "Filter by entity type"
// @Param name query string false "Filter by group name"
// @Success 200 {object} dto.ListGroupsResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /groups [get]
func (h *GroupHandler) ListGroups(c *gin.Context) {
	// Parse query parameters
	entityType := c.Query("entity_type")
	name := c.Query("name")

	// Create filter
	filter := &types.GroupFilter{
		EntityType: entityType,
		Name:       name,
	}

	resp, err := h.service.ListGroups(c.Request.Context(), filter)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Add entity to group
// @Description Add an entity to a group
// @Tags Groups
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Group ID"
// @Param entity_id path string true "Entity ID"
// @Success 200 {object} dto.GroupResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /groups/{id}/add [post]
func (h *GroupHandler) AddEntityToGroup(c *gin.Context) {
	id := c.Param("id")
	entityID := c.Param("entity_id")

	err := h.service.AddEntityToGroup(c.Request.Context(), id, entityID)
	if err != nil {
		c.Error(err)
		return
	}

	c.Status(http.StatusOK)
}
