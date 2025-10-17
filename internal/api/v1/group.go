package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
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
// @Description Create a new group for organizing prices
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

// @Summary Update a group
// @Description Update a group's details and associated prices
// @Tags Groups
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Group ID"
// @Param group body dto.UpdateGroupRequest true "Group"
// @Success 200 {object} dto.GroupResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /groups/{id} [put]
func (h *GroupHandler) UpdateGroup(c *gin.Context) {
	id := c.Param("id")

	var req dto.UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.UpdateGroup(c.Request.Context(), id, req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Delete a group
// @Description Delete a group and remove all price associations
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
