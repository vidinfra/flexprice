package service

import (
	"context"

	"github.com/flexprice/flexprice/internal/api/dto"
)

type StripeSubscriptionService interface {
	CreateSubscription(ctx context.Context, req dto.CreateSubscriptionRequest) (*dto.SubscriptionResponse, error)
}

type stripeSubscriptionService struct {
	ServiceParams
}

func NewStripeSubscriptionService(params ServiceParams) *stripeSubscriptionService {
	return &stripeSubscriptionService{
		ServiceParams: params,
	}
}

func (s *stripeSubscriptionService) CreateSubscription(ctx context.Context, req dto.CreateSubscriptionRequest) (*dto.SubscriptionResponse, error) {
	return nil, nil
}
