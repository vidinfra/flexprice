package sslcommerz

import (
	"resty.dev/v3"
)

type SSLCommerzClient struct {
	IPNURL string
	// IsSandbox bool

	storeID       string
	storePassword string
	baseURL       string
	client        *resty.Client
}

func (s *SSLCommerzClient) GetClient() *SSLCommerzClient {
	if s.client == nil {
		s.client = resty.New()
	}

	return s
}

func (s *SSLCommerzClient) SetOpts(store, pass, url string) *SSLCommerzClient {
	if store != "" {
		s.storeID = store
	}
	if pass != "" {
		s.storePassword = pass
	}
	if url != "" {
		s.baseURL = url
	}
	if s.client == nil {
		s.client = resty.New()
	}

	return s
}
