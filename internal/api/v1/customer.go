package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type CustomerHandler struct {
	service service.CustomerService
	billing service.BillingService
	log     *logger.Logger
}

func NewCustomerHandler(
	service service.CustomerService,
	billing service.BillingService,
	log *logger.Logger,
) *CustomerHandler {
	return &CustomerHandler{
		service: service,
		billing: billing,
		log:     log,
	}
}

// @Summary Create a customer
// @Description Create a customer
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param customer body dto.CreateCustomerRequest true "Customer"
// @Success 201 {object} dto.CustomerResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers [post]
func (h *CustomerHandler) CreateCustomer(c *gin.Context) {
	var req dto.CreateCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.CreateCustomer(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// @Summary Get a customer
// @Description Get a customer
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Customer ID"
// @Success 200 {object} dto.CustomerResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/{id} [get]
func (h *CustomerHandler) GetCustomer(c *gin.Context) {
	id := c.Param("id")

	resp, err := h.service.GetCustomer(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Get customers
// @Description Get customers
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param filter query types.CustomerFilter false "Filter"
// @Success 200 {object} dto.ListCustomersResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers [get]
func (h *CustomerHandler) GetCustomers(c *gin.Context) {
	var filter types.CustomerFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation))
		return
	}

	if filter.GetLimit() == 0 {
		filter.Limit = lo.ToPtr(types.GetDefaultFilter().Limit)
	}

	resp, err := h.service.GetCustomers(c.Request.Context(), &filter)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Update a customer
// @Description Update a customer
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Customer ID"
// @Param customer body dto.UpdateCustomerRequest true "Customer"
// @Success 200 {object} dto.CustomerResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/{id} [put]
func (h *CustomerHandler) UpdateCustomer(c *gin.Context) {
	id := c.Param("id")

	var req dto.UpdateCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.UpdateCustomer(c.Request.Context(), id, req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Delete a customer
// @Description Delete a customer
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Customer ID"
// @Success 204
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/{id} [delete]
func (h *CustomerHandler) DeleteCustomer(c *gin.Context) {
	id := c.Param("id")

	err := h.service.DeleteCustomer(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get a customer by lookup key
// @Description Get a customer by lookup key (external_id)
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param lookup_key path string true "Customer Lookup Key (external_id)"
// @Success 200 {object} dto.CustomerResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/lookup/{lookup_key} [get]
func (h *CustomerHandler) GetCustomerByLookupKey(c *gin.Context) {
	lookupKey := c.Param("lookup_key")
	if lookupKey == "" {
		c.Error(ierr.NewError("lookup key is required").
			WithHint("Lookup key is required").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.GetCustomerByLookupKey(c.Request.Context(), lookupKey)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Get customer entitlements
// @Description Get customer entitlements
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Customer ID"
// @Param filter query dto.GetCustomerEntitlementsRequest false "Filter"
// @Success 200 {object} dto.CustomerEntitlementsResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/{id}/entitlements [get]
func (h *CustomerHandler) GetCustomerEntitlements(c *gin.Context) {
	id := c.Param("id")

	// Parse query parameters using binding
	var req dto.GetCustomerEntitlementsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid query parameters").
			Mark(ierr.ErrValidation))
		return
	}

	// Call billing service instead of customer service
	response, err := h.billing.GetCustomerEntitlements(c.Request.Context(), id, &req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Get customer usage summary
// @Description Get customer usage summary
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Customer ID"
// @Param filter query dto.GetCustomerUsageSummaryRequest false "Filter"
// @Success 200 {object} dto.CustomerUsageSummaryResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/{id}/usage [get]
func (h *CustomerHandler) GetCustomerUsageSummary(c *gin.Context) {
	id := c.Param("id")

	// Parse query parameters using binding
	var req dto.GetCustomerUsageSummaryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid query parameters").
			Mark(ierr.ErrValidation))
		return
	}

	// Call billing service instead of customer service
	response, err := h.billing.GetCustomerUsageSummary(c.Request.Context(), id, &req)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary List customers by filter
// @Description List customers by filter
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param filter body types.CustomerFilter true "Filter"
// @Success 200 {object} dto.ListCustomersResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/search [post]
func (h *CustomerHandler) ListCustomersByFilter(c *gin.Context) {
	var filter types.CustomerFilter
	if err := c.ShouldBindJSON(&filter); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation))
		return
	}

	if filter.GetLimit() == 0 {
		filter.Limit = lo.ToPtr(types.GetDefaultFilter().Limit)
	}

	resp, err := h.service.GetCustomers(c.Request.Context(), &filter)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Get customer payment methods
// @Description Get customer payment methods
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Customer ID"
// @Success 200 {object} dto.PaymentMethodResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/{id}/payment-methods [get]
func (h *CustomerHandler) GetCustomerPaymentMethods(c *gin.Context) {
	id := c.Param("id")
	resp, err := h.service.GetCustomerPaymentMethods(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// @Summary Set default payment method
// @Description Set default payment method
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Customer ID"
// @Success 200 {object} dto.PaymentMethodResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/{id}/payment-methods [put]
func (h *CustomerHandler) SetDefaultPaymentMethod(c *gin.Context) {
	id := c.Param("id")
	var req dto.SetDefaultPaymentMethodRequest
	req.CustomerID = id
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request payload").
			Mark(ierr.ErrValidation))
		return
	}

	if err := h.service.SetDefaultPaymentMethod(c.Request.Context(), id, req.PaymentMethodID); err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Default payment method updated successfully"})
}

// @Summary Delete payment method
// @Description Delete payment method
// @Tags Customers
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Customer ID"
// @Param payment_method_id path string true "Payment Method ID"
// @Success 200 {object} dto.PaymentMethodResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /customers/{id}/payment-methods/{payment_method_id} [delete]
func (h *CustomerHandler) DeletePaymentMethod(c *gin.Context) {
	id := c.Param("id")
	paymentMethodId := c.Param("payment_method_id")

	if err := h.service.DeletePaymentMethod(c.Request.Context(), id, paymentMethodId); err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Payment method deleted successfully"})
}
