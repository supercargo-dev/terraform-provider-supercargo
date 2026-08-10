package hub

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Client handles communication with the Supercargo Hub.
type Client struct {
	hubv1.HubServiceClient
	conn *grpc.ClientConn
}

// Factory manages a pool of Hub clients.
type Factory struct {
	mu      sync.Mutex
	clients map[string]*Client
}

// NewFactory creates a new Hub client factory.
func NewFactory() *Factory {
	return &Factory{
		clients: make(map[string]*Client),
	}
}

// GetClient returns a Hub client for the given address.
func (f *Factory) GetClient(ctx context.Context, address string, token string, opts ...grpc.DialOption) (*Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if client, ok := f.clients[address]; ok {
		return client, nil
	}

	host := address
	if h, _, err := net.SplitHostPort(address); err == nil {
		host = h
	}

	// Determine if we need OIDC (GCP environment) or insecure (localhost)
	if host == "localhost" || host == "127.0.0.1" {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

		// For local development against emulators, we inject a mock token that passes AuthZ
		mockToken := os.Getenv("SUPERCARGO_MOCK_TOKEN")
		if mockToken == "" {
			return nil, fmt.Errorf("SUPERCARGO_MOCK_TOKEN environment variable is required for local development")
		}

		ts := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: mockToken,
			Expiry:      time.Now().Add(1 * time.Hour),
		})
		opts = append(opts, grpc.WithPerRPCCredentials(NewInsecureOIDCCredentials(ts)))
	} else {
		// Use OIDC for cloud environments
		// The audience is the Hub's URL (usually the address without port)
		audience := "https://" + host

		var ts oauth2.TokenSource
		if token != "" {
			// If a token is explicitly provided (e.g., from an ID token data source), use it.
			ts = oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken: token,
			})
		} else {
			// Fallback to ADC
			var err error
			ts, err = idtoken.NewTokenSource(ctx, audience)
			if err != nil {
				return nil, fmt.Errorf("failed to create OIDC token source for %s: %w", audience, err)
			}
		}

		creds := NewOIDCCredentials(ts)
		opts = append(opts,
			grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
			grpc.WithPerRPCCredentials(creds),
		)
	}

	retryPolicy := `{
		"methodConfig": [{
		  "name": [{"service": ""}],
		  "retryPolicy": {
			"maxAttempts": 5,
			"initialBackoff": "1s",
			"maxBackoff": "10s",
			"backoffMultiplier": 2,
			"retryableStatusCodes": ["UNAVAILABLE", "RESOURCE_EXHAUSTED"]
		  }
		}]
	}`
	opts = append(opts, grpc.WithDefaultServiceConfig(retryPolicy))

	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial hub at %s: %w", address, err)
	}

	client := &Client{
		HubServiceClient: hubv1.NewHubServiceClient(conn),
		conn:             conn,
	}

	f.clients[address] = client
	return client, nil
}

// Close closes all client connections.
func (f *Factory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var errs []error
	for addr, client := range f.clients {
		if err := client.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close connection to %s: %w", addr, err))
		}
	}
	f.clients = make(map[string]*Client)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing hub factory: %v", errs)
	}
	return nil
}
