package hub

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// OIDCCredentials implements grpc.PerRPCCredentials with proactive token refresh.
type OIDCCredentials struct {
	source            oauth2.TokenSource
	mu                sync.RWMutex
	token             *oauth2.Token
	insecureTransport bool
}

// NewOIDCCredentials creates a new OIDCCredentials with the given token source.
func NewOIDCCredentials(source oauth2.TokenSource) *OIDCCredentials {
	return &OIDCCredentials{
		source: source,
	}
}

// NewInsecureOIDCCredentials creates a new OIDCCredentials that allows insecure transport.
func NewInsecureOIDCCredentials(source oauth2.TokenSource) *OIDCCredentials {
	return &OIDCCredentials{
		source:            source,
		insecureTransport: true,
	}
}

// GetRequestMetadata returns the authorization metadata.
func (c *OIDCCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"authorization": fmt.Sprintf("Bearer %s", token.AccessToken),
	}, nil
}

// RequireTransportSecurity returns true unless insecureTransport is set.
func (c *OIDCCredentials) RequireTransportSecurity() bool {
	return !c.insecureTransport
}

func (c *OIDCCredentials) getToken() (*oauth2.Token, error) {
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	// Proactive refresh: if token is missing or expires in less than 1 minute
	if token == nil || time.Until(token.Expiry) < time.Minute {
		c.mu.Lock()
		defer c.mu.Unlock()

		// Re-check after acquiring write lock
		if c.token == nil || time.Until(c.token.Expiry) < time.Minute {
			newToken, err := c.source.Token()
			if err != nil {
				return nil, fmt.Errorf("failed to fetch OIDC token: %w", err)
			}
			c.token = newToken
		}
		return c.token, nil
	}

	return token, nil
}
