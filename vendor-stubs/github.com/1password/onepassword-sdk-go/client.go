package onepassword

import (
	"context"
	"errors"
)

// Client is a stub client that does nothing
type Client struct{}

// Secrets returns a stub secrets client
func (c *Client) Secrets() *SecretsClient {
	return &SecretsClient{}
}

// SecretsClient is a stub secrets client
type SecretsClient struct{}

// Resolve always returns an error indicating 1Password is not available
func (s *SecretsClient) Resolve(ctx context.Context, ref string) (string, error) {
	return "", errors.New("1Password provider is disabled in this build")
}

// NewClient creates a stub client that does nothing
func NewClient(ctx context.Context, opts ...ClientOption) (*Client, error) {
	return &Client{}, nil
}

// ClientOption is a function type for client options
type ClientOption func(*Client)

// WithServiceAccountToken is a stub option
func WithServiceAccountToken(token string) ClientOption {
	return func(c *Client) {}
}

// WithIntegrationInfo is a stub option
func WithIntegrationInfo(name, version string) ClientOption {
	return func(c *Client) {}
}
