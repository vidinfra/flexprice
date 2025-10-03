package stripe

import (
	"context"
	"strings"
	"time"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/stripe/stripe-go/v82"
)

type StripeSubscriptionService interface {
	CreateSubscription(ctx context.Context, stripeSubscriptionID string, services *ServiceDependencies) (*dto.SubscriptionResponse, error)
	UpdateSubscription(ctx context.Context, stripeSubscriptionID string, services *ServiceDependencies) error
	CancelSubscription(ctx context.Context, stripeSubscriptionID string, services *ServiceDependencies) (*dto.SubscriptionResponse, error)
}

type stripeSubscriptionService struct {
	client *Client
	logger *logger.Logger
}

func NewStripeSubscriptionService(client *Client, logger *logger.Logger) *stripeSubscriptionService {
	return &stripeSubscriptionService{
		client: client,
		logger: logger,
	}
}

// TODOS:
/*
	1. Handle Billing Cycle Conversion (Stripe dont have any billing cycle conversion, so we need to handle it)
*/

// fetchStripeSubscription retrieves a subscription from Stripe
func (s *stripeSubscriptionService) fetchStripeSubscription(ctx context.Context, subscriptionID string) (*stripe.Subscription, error) {

	stripeClient, _, err := s.client.GetStripeClient(ctx)

	// Retrieve the subscription from Stripe with expanded fields
	params := &stripe.SubscriptionRetrieveParams{
		Expand: []*string{
			stripe.String("customer"),
			stripe.String("items.data.price.product"),
		},
	}

	stripeSub, err := stripeClient.V1Subscriptions.Retrieve(ctx, subscriptionID, params)
	if err != nil {
		s.logger.Errorw("failed to retrieve subscription from Stripe",
			"error", err,
			"subscription_id", subscriptionID,
		)
		return nil, ierr.NewError("failed to retrieve subscription from Stripe").
			WithHint("Could not fetch subscription information from Stripe").
			WithReportableDetails(map[string]interface{}{
				"subscription_id": subscriptionID,
				"error":           err.Error(),
			}).
			Mark(ierr.ErrSystem)
	}

	return stripeSub, nil
}

func (s *stripeSubscriptionService) CreateSubscription(ctx context.Context, stripeSubscriptionID string, services *ServiceDependencies) (*dto.SubscriptionResponse, error) {

	// Extract Services

	subscriptionService := services.SubscriptionService
	entityMappingService := services.EntityIntegrationMappingService
	// Step 1: Fetch Stripe subscription data
	stripeSubscription, err := s.fetchStripeSubscription(ctx, stripeSubscriptionID)
	if err != nil {
		return nil, err
	}

	// Step 2: Check if the mapping with the stripe subscription id exists
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:        types.IntegrationEntityTypeSubscription,
		ProviderTypes:     []string{"stripe"},
		ProviderEntityIDs: []string{stripeSubscriptionID},
	}

	existingMappings, err := entityMappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to check for existing subscription mapping").
			Mark(ierr.ErrInternal)
	}

	// If mapping exists, return the existing subscription
	if len(existingMappings.Items) > 0 {
		existingMapping := existingMappings.Items[0]
		return subscriptionService.GetSubscription(ctx, existingMapping.EntityID)
	}

	// Step 3: Create or find customer
	customerID, err := s.createOrFindCustomer(ctx, stripeSubscription, services)
	if err != nil {
		return nil, err
	}

	// Step 4: Create or find plan
	planID, err := s.createOrFindPlan(ctx, stripeSubscription, services)
	if err != nil {
		return nil, err
	}

	// Step 5: Create subscription
	subscriptionResp, err := s.createFlexPriceSubscription(ctx, stripeSubscription, customerID, planID, services)
	if err != nil {
		return nil, err
	}

	// Step 6: Create entity mapping
	_, err = entityMappingService.CreateEntityIntegrationMapping(ctx, dto.CreateEntityIntegrationMappingRequest{
		EntityID:         subscriptionResp.ID,
		EntityType:       types.IntegrationEntityTypeSubscription,
		ProviderType:     "stripe",
		ProviderEntityID: stripeSubscriptionID,
		Metadata: map[string]interface{}{
			"created_via":            "stripe_subscription_service",
			"stripe_subscription_id": stripeSubscriptionID,
			"synced_at":              time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		s.logger.Warnw("failed to create entity mapping for subscription",
			"error", err,
			"subscription_id", subscriptionResp.ID,
			"stripe_subscription_id", stripeSubscriptionID)
		// Don't fail the entire operation if entity mapping creation fails
	}

	return subscriptionResp, nil
}

func (s *stripeSubscriptionService) UpdateSubscription(ctx context.Context, stripeSubscriptionID string, services *ServiceDependencies) error {
	// TODO: IMPLEMENT THIS
	/*
		1. Check if the mapping with the stripe subscription id exists
		2. Fetch Stripe subscription data
		3. Update the subscription in Flexprice
		4. Create entity mapping for subscription
		5. Return the subscription

		6. Return the subscription
	*/

	// Step 1: Fetch Stripe Subscription
	stripeSubscription, err := s.fetchStripeSubscription(ctx, stripeSubscriptionID)
	if err != nil {
		return err
	}
	// Step 2: Check if the mapping with the stripe subscription id exists
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:        types.IntegrationEntityTypeSubscription,
		ProviderTypes:     []string{"stripe"},
		ProviderEntityIDs: []string{stripeSubscriptionID},
	}

	existingMappings, err := services.EntityIntegrationMappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to check for existing subscription mapping").
			Mark(ierr.ErrInternal)
	}

	// Step 3: Get the existing mapping
	existingSubscriptionMapping := existingMappings.Items[0]

	// Step 4: Get the exisitng subcription
	existingSubscription, err := services.SubscriptionService.GetSubscription(ctx, existingSubscriptionMapping.EntityID)
	if err != nil {
		return err
	}

	planChange, err := s.isPlanChange(ctx, existingSubscription, stripeSubscription, services)
	if err != nil {
		return err
	}

	if planChange {
		return s.handlePlanChange(ctx, existingSubscription, stripeSubscription, services)
	} else {
		return s.handleNormalChange(ctx, existingSubscription, stripeSubscription)
	}
}

func (s *stripeSubscriptionService) CancelSubscription(ctx context.Context, stripeSubscriptionID string, services *ServiceDependencies) (*dto.SubscriptionResponse, error) {

	filter := &types.EntityIntegrationMappingFilter{
		EntityType:        types.IntegrationEntityTypeSubscription,
		ProviderTypes:     []string{"stripe"},
		ProviderEntityIDs: []string{stripeSubscriptionID},
	}

	existingMappings, err := services.EntityIntegrationMappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to check for existing subscription mapping").
			Mark(ierr.ErrInternal)
	}

	// If mapping exists, return the existing subscription
	if len(existingMappings.Items) > 0 {
		existingMapping := existingMappings.Items[0]

		// Step 3: Cancel the subscription
		_, err := services.SubscriptionService.CancelSubscription(ctx, existingMapping.EntityID, &dto.CancelSubscriptionRequest{
			CancellationType: types.CancellationTypeImmediate,
			Reason:           "Customer cancelled subscription",
		})
		if err != nil {
			return nil, err
		}

	}
	return nil, nil
}

// createOrFindCustomer creates or finds a customer based on Stripe subscription data
func (s *stripeSubscriptionService) createOrFindCustomer(ctx context.Context, stripeSub *stripe.Subscription, services *ServiceDependencies) (string, error) {
	if stripeSub.Customer == nil {
		return "", ierr.NewError("no customer found in Stripe subscription").
			WithHint("Stripe subscription must have a customer").
			Mark(ierr.ErrValidation)
	}

	stripeCustomerID := stripeSub.Customer.ID

	// Check if customer already exists in our system
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:        types.IntegrationEntityTypeCustomer,
		ProviderTypes:     []string{"stripe"},
		ProviderEntityIDs: []string{stripeCustomerID},
	}

	entityMappingService := services.EntityIntegrationMappingService

	existingMappings, err := entityMappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to check for existing customer mapping").
			Mark(ierr.ErrInternal)
	}

	// If customer exists, return the ID
	if len(existingMappings.Items) > 0 {
		return existingMappings.Items[0].EntityID, nil
	}

	// Create customer from Stripe data
	customerService := services.CustomerService

	// Check if customer has flexprice_customer_id in metadata
	var externalID string
	if flexpriceID, exists := stripeSub.Customer.Metadata["flexprice_customer_id"]; exists {
		externalID = flexpriceID
	} else {
		// Use Stripe customer ID as external ID
		externalID = stripeCustomerID
	}

	createReq := dto.CreateCustomerRequest{
		ExternalID: externalID,
		Name:       stripeSub.Customer.Name,
		Email:      stripeSub.Customer.Email,
		Metadata: map[string]string{
			"stripe_customer_id": stripeCustomerID,
		},
	}

	// Add address if available
	if stripeSub.Customer.Address != nil {
		createReq.AddressLine1 = stripeSub.Customer.Address.Line1
		createReq.AddressLine2 = stripeSub.Customer.Address.Line2
		createReq.AddressCity = stripeSub.Customer.Address.City
		createReq.AddressState = stripeSub.Customer.Address.State
		createReq.AddressPostalCode = stripeSub.Customer.Address.PostalCode
		createReq.AddressCountry = stripeSub.Customer.Address.Country
	}

	customerResp, err := customerService.CreateCustomer(ctx, createReq)
	if err != nil {
		return "", err
	}

	// Create entity mapping for customer
	_, err = entityMappingService.CreateEntityIntegrationMapping(ctx, dto.CreateEntityIntegrationMappingRequest{
		EntityID:         customerResp.ID,
		EntityType:       types.IntegrationEntityTypeCustomer,
		ProviderType:     "stripe",
		ProviderEntityID: stripeCustomerID,
		Metadata: map[string]interface{}{
			"created_via":        "stripe_subscription_service",
			"stripe_customer_id": stripeCustomerID,
			"synced_at":          time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		s.logger.Warnw("failed to create entity mapping for customer",
			"error", err,
			"customer_id", customerResp.ID,
			"stripe_customer_id", stripeCustomerID)
		// Don't fail the entire operation if entity mapping creation fails
	}

	return customerResp.ID, nil
}

// createOrFindPlan creates or finds a plan based on Stripe subscription data
func (s *stripeSubscriptionService) createOrFindPlan(ctx context.Context, stripeSub *stripe.Subscription, services *ServiceDependencies) (string, error) {
	if stripeSub.Items == nil || len(stripeSub.Items.Data) == 0 {
		return "", ierr.NewError("no items found in Stripe subscription").
			WithHint("Stripe subscription must have at least one item").
			Mark(ierr.ErrValidation)
	}

	// Get the first item's product
	firstItem := stripeSub.Items.Data[0]
	if firstItem.Price == nil || firstItem.Price.Product == nil {
		return "", ierr.NewError("no product found in Stripe subscription item").
			WithHint("Stripe subscription item must have a product").
			Mark(ierr.ErrValidation)
	}

	stripeProductID := firstItem.Price.Product.ID

	// Check if plan already exists in our system
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:        types.IntegrationEntityTypePlan,
		ProviderTypes:     []string{"stripe"},
		ProviderEntityIDs: []string{stripeProductID},
	}

	existingMappings, err := services.EntityIntegrationMappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		return "", ierr.WithError(err).
			WithHint("Failed to check for existing plan mapping").
			Mark(ierr.ErrInternal)
	}

	// If plan exists, return the ID
	if len(existingMappings.Items) > 0 {
		return existingMappings.Items[0].EntityID, nil
	}

	// Create plan from Stripe product data

	createPlanReq := dto.CreatePlanRequest{
		Name:         firstItem.Price.Product.Name,
		Description:  firstItem.Price.Product.Description,
		LookupKey:    stripeProductID,
		Prices:       []dto.CreatePlanPriceRequest{},       // Empty prices initially
		Entitlements: []dto.CreatePlanEntitlementRequest{}, // Empty entitlements initially
		CreditGrants: []dto.CreateCreditGrantRequest{},     // Empty credit grants initially
		Metadata: types.Metadata{
			"source":            "stripe",
			"stripe_plan_id":    stripeProductID,
			"stripe_product_id": stripeProductID,
		},
	}

	createPlanResp, err := services.PlanService.CreatePlan(ctx, createPlanReq)
	if err != nil {
		return "", err
	}

	// Create entity mapping for plan
	_, err = services.EntityIntegrationMappingService.CreateEntityIntegrationMapping(ctx, dto.CreateEntityIntegrationMappingRequest{
		EntityID:         createPlanResp.ID,
		EntityType:       types.IntegrationEntityTypePlan,
		ProviderType:     "stripe",
		ProviderEntityID: stripeProductID,
		Metadata: map[string]interface{}{
			"created_via":    "stripe_subscription_service",
			"stripe_plan_id": stripeProductID,
			"synced_at":      time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		s.logger.Warnw("failed to create entity mapping for plan",
			"error", err,
			"plan_id", createPlanResp.ID,
			"stripe_product_id", stripeProductID)
		// Don't fail the entire operation if entity mapping creation fails
	}

	return createPlanResp.ID, nil
}

// createFlexPriceSubscription creates a FlexPrice subscription based on Stripe subscription data
func (s *stripeSubscriptionService) createFlexPriceSubscription(ctx context.Context, stripeSub *stripe.Subscription, customerID, planID string, services *ServiceDependencies) (*dto.SubscriptionResponse, error) {
	subscriptionService := services.SubscriptionService

	// Convert Stripe billing interval to FlexPrice billing period
	var billingPeriod types.BillingPeriod
	var billingPeriodCount int
	switch stripeSub.Items.Data[0].Price.Recurring.Interval {
	case stripe.PriceRecurringIntervalDay:
		billingPeriod = types.BILLING_PERIOD_DAILY
		billingPeriodCount = int(stripeSub.Items.Data[0].Price.Recurring.IntervalCount)
	case stripe.PriceRecurringIntervalWeek:
		billingPeriod = types.BILLING_PERIOD_WEEKLY
		billingPeriodCount = int(stripeSub.Items.Data[0].Price.Recurring.IntervalCount)
	case stripe.PriceRecurringIntervalMonth:
		billingPeriod = types.BILLING_PERIOD_MONTHLY
		billingPeriodCount = int(stripeSub.Items.Data[0].Price.Recurring.IntervalCount)
	case stripe.PriceRecurringIntervalYear:
		billingPeriod = types.BILLING_PERIOD_ANNUAL
		billingPeriodCount = int(stripeSub.Items.Data[0].Price.Recurring.IntervalCount)
	default:
		billingPeriod = types.BILLING_PERIOD_MONTHLY
		billingPeriodCount = 1
	}

	// Set start date
	startDate := time.Unix(stripeSub.StartDate, 0).UTC()

	// Set trial dates if applicable
	var trialStart, trialEnd *time.Time
	if stripeSub.TrialStart != 0 {
		trialStartTime := time.Unix(stripeSub.TrialStart, 0).UTC()
		trialStart = &trialStartTime
	}
	if stripeSub.TrialEnd != 0 {
		trialEndTime := time.Unix(stripeSub.TrialEnd, 0).UTC()
		trialEnd = &trialEndTime
	}

	// Set end date if subscription is canceled
	var endDate *time.Time
	if stripeSub.CanceledAt != 0 {
		endDateTime := time.Unix(stripeSub.CanceledAt, 0).UTC()
		endDate = &endDateTime
	}

	createReq := dto.CreateSubscriptionRequest{
		CustomerID:         customerID,
		PlanID:             planID,
		Currency:           strings.ToUpper(string(stripeSub.Currency)),
		LookupKey:          stripeSub.ID,
		StartDate:          &startDate,
		EndDate:            endDate,
		TrialStart:         trialStart,
		TrialEnd:           trialEnd,
		BillingCadence:     types.BILLING_CADENCE_RECURRING,
		BillingPeriod:      billingPeriod,
		BillingPeriodCount: billingPeriodCount,
		BillingCycle:       types.BillingCycleAnniversary,
		Metadata: map[string]string{
			"stripe_subscription_id": stripeSub.ID,
			"source":                 "stripe",
		},
	}

	return subscriptionService.CreateSubscription(ctx, createReq)
}

func (s *stripeSubscriptionService) isPlanChange(ctx context.Context, existingSubscription *dto.SubscriptionResponse, stripeSubscription *stripe.Subscription, services *ServiceDependencies) (bool, error) {
	// Step 1: Get the exisiting Plan Mapping
	entityMappingService := services.EntityIntegrationMappingService
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:    types.IntegrationEntityTypePlan,
		ProviderTypes: []string{"stripe"},
		EntityID:      existingSubscription.PlanID,
	}
	existingPlanMapping, err := entityMappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		return false, ierr.WithError(err).
			WithHint("Failed to get existing plan mapping").
			Mark(ierr.ErrInternal)
	}

	if len(existingPlanMapping.Items) == 0 {
		return false, ierr.NewError("no existing plan mapping found").
			WithHint("Existing plan mapping not found").
			Mark(ierr.ErrInternal)
	}

	existingPlanID := existingPlanMapping.Items[0].ProviderEntityID

	if existingPlanID != stripeSubscription.Items.Data[0].Plan.ID {
		return true, nil
	}

	return false, nil
}

func (s *stripeSubscriptionService) handlePlanChange(ctx context.Context, existingSubscription *dto.SubscriptionResponse, stripeSubscription *stripe.Subscription, services *ServiceDependencies) error {
	s.logger.Infow("handling plan change for subscription",
		"existing_subscription_id", existingSubscription.ID,
		"stripe_subscription_id", stripeSubscription.ID)

	// STEP 1: Cancel existing subscription which will automatically generate proration invoice
	subscriptionService := services.SubscriptionService
	cancelReq := &dto.CancelSubscriptionRequest{
		CancellationType:  types.CancellationTypeImmediate,
		ProrationBehavior: types.ProrationBehaviorNone,
		Reason:            "Plan change - upgrading to new plan",
	}

	_, err := subscriptionService.CancelSubscription(ctx, existingSubscription.ID, cancelReq)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to cancel existing subscription during plan change").
			Mark(ierr.ErrInternal)
	}

	s.logger.Infow("successfully cancelled existing subscription",
		"subscription_id", existingSubscription.ID)

	// STEP 2: Create new subscription with updated plan
	// First, get or create the new plan
	newPlanID, err := s.createOrFindPlan(ctx, stripeSubscription, services)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to create or find new plan for subscription").
			Mark(ierr.ErrInternal)
	}

	// Get or create customer (should already exist)
	customerID, err := s.createOrFindCustomer(ctx, stripeSubscription, services)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to get customer for new subscription").
			Mark(ierr.ErrInternal)
	}

	// Create new subscription with the updated plan
	newSubscription, err := s.createFlexPriceSubscription(ctx, stripeSubscription, customerID, newPlanID, services)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to create new subscription with updated plan").
			Mark(ierr.ErrInternal)
	}

	s.logger.Infow("successfully created new subscription",
		"new_subscription_id", newSubscription.ID,
		"new_plan_id", newPlanID)

	// STEP 3: Update the entity mapping to point to the new subscription
	entityMappingService := services.EntityIntegrationMappingService

	// Find the existing mapping
	filter := &types.EntityIntegrationMappingFilter{
		EntityType:        types.IntegrationEntityTypeSubscription,
		ProviderTypes:     []string{"stripe"},
		ProviderEntityIDs: []string{stripeSubscription.ID},
	}

	existingMappings, err := entityMappingService.GetEntityIntegrationMappings(ctx, filter)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to find existing entity mapping").
			Mark(ierr.ErrInternal)
	}

	if len(existingMappings.Items) == 0 {
		return ierr.NewError("no existing entity mapping found").
			WithHint("Entity mapping should exist for the subscription").
			Mark(ierr.ErrInternal)
	}

	// Delete the old mapping and create a new one pointing to the new subscription
	existingMapping := existingMappings.Items[0]

	// Delete the old mapping
	err = entityMappingService.DeleteEntityIntegrationMapping(ctx, existingMapping.ID)
	if err != nil {
		s.logger.Warnw("failed to delete old entity mapping for subscription",
			"error", err,
			"mapping_id", existingMapping.ID,
			"old_subscription_id", existingSubscription.ID,
			"stripe_subscription_id", stripeSubscription.ID)
		// Don't fail the entire operation if entity mapping deletion fails
	}

	// Create new mapping pointing to the new subscription
	_, err = entityMappingService.CreateEntityIntegrationMapping(ctx, dto.CreateEntityIntegrationMappingRequest{
		EntityID:         newSubscription.ID,
		EntityType:       types.IntegrationEntityTypeSubscription,
		ProviderType:     "stripe",
		ProviderEntityID: stripeSubscription.ID,
		Metadata: map[string]interface{}{
			"created_via":              "stripe_subscription_service_plan_change",
			"stripe_subscription_id":   stripeSubscription.ID,
			"previous_subscription_id": existingSubscription.ID,
			"created_at":               time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		s.logger.Warnw("failed to create new entity mapping for subscription",
			"error", err,
			"new_subscription_id", newSubscription.ID,
			"stripe_subscription_id", stripeSubscription.ID)
		// Don't fail the entire operation if entity mapping creation fails
	}

	s.logger.Infow("successfully handled plan change",
		"old_subscription_id", existingSubscription.ID,
		"new_subscription_id", newSubscription.ID,
		"stripe_subscription_id", stripeSubscription.ID)

	return nil
}

func (s *stripeSubscriptionService) handleNormalChange(ctx context.Context, existingSubscription *dto.SubscriptionResponse, stripeSubscription *stripe.Subscription) error {
	return nil
}
