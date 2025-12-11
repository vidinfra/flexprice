package chartmogul

import (
	"fmt"

	cm "github.com/chartmogul/chartmogul-go/v4"
	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
)

type ChartMogulService struct {
	client *cm.API
	cfg    *config.Configuration
	logger *logger.Logger
}

func NewChartMogulService(cfg *config.Configuration, logger *logger.Logger) (*ChartMogulService, error) {
	apiKey := cfg.ChartMogul.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("CHARTMOGUL_API_KEY not set")
	}
	client := &cm.API{
		ApiKey: apiKey,
	}
	return &ChartMogulService{
		client: client,
		cfg:    cfg,
		logger: logger,
	}, nil
}

func (s *ChartMogulService) Ping() (bool, error) {
	return s.client.Ping()
}
