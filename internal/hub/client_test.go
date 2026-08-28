package hub

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestFactory_GetClient_Concurrent(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	s := grpc.NewServer()
	go func() {
		_ = s.Serve(lis)
	}()
	defer s.Stop()

	os.Setenv("SUPERCARGO_MOCK_TOKEN", "mock-token-for-test")
	defer os.Unsetenv("SUPERCARGO_MOCK_TOKEN")

	factory := NewFactory()
	defer factory.Close()

	addr := "localhost:50051"
	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	numGoroutines := 20
	clients := make([]*Client, numGoroutines)
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()
			c, err := factory.GetClient(
				context.Background(),
				addr,
				"mock-token",
				"",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(dialer),
			)
			if err == nil {
				clients[idx] = c
			}
		}()
	}

	wg.Wait()

	// All returned clients must be identical pointer instance
	var first *Client
	for i, c := range clients {
		require.NotNil(t, c, "client at index %d was nil", i)
		if first == nil {
			first = c
		} else {
			assert.Same(t, first, c, "all concurrent GetClient calls should return the exact same cached instance")
		}
	}

	// Verify only 1 client is stored in the map
	factory.mu.Lock()
	count := len(factory.clients)
	factory.mu.Unlock()
	assert.Equal(t, 1, count, "should have exactly 1 client instance in factory map")
}

func TestFactory_GetClient_AfterClose(t *testing.T) {
	factory := NewFactory()
	err := factory.Close()
	require.NoError(t, err)

	client, err := factory.GetClient(context.Background(), "localhost:50051", "mock-token", "")
	assert.Nil(t, client)
	assert.ErrorIs(t, err, ErrFactoryClosed)
}

func TestFactory_Close_Idempotent(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	factory := NewFactory()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	client, err := factory.GetClient(
		context.Background(),
		"localhost:50051",
		"mock-token",
		"",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
	)
	require.NoError(t, err)
	require.NotNil(t, client)

	// First Close
	err = factory.Close()
	require.NoError(t, err)

	// Second Close should be idempotent
	err = factory.Close()
	require.NoError(t, err)
}

func TestFactory_GetClient_LoopbackMissingToken(t *testing.T) {
	prevToken := os.Getenv("SUPERCARGO_MOCK_TOKEN")
	os.Unsetenv("SUPERCARGO_MOCK_TOKEN")
	defer func() {
		if prevToken != "" {
			os.Setenv("SUPERCARGO_MOCK_TOKEN", prevToken)
		}
	}()

	factory := NewFactory()
	defer factory.Close()

	client, err := factory.GetClient(context.Background(), "localhost:50051", "", "")
	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token or SUPERCARGO_MOCK_TOKEN environment variable must be set")
}

func TestSanitizeAddress(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedAddr string
		expectedHost string
	}{
		{
			name:         "plain host:port",
			input:        "localhost:50051",
			expectedAddr: "localhost:50051",
			expectedHost: "localhost",
		},
		{
			name:         "https prefix with port and trailing slash",
			input:        "https://hub.example.com:443/",
			expectedAddr: "hub.example.com:443",
			expectedHost: "hub.example.com",
		},
		{
			name:         "http prefix with port",
			input:        "http://localhost:50051",
			expectedAddr: "localhost:50051",
			expectedHost: "localhost",
		},
		{
			name:         "https prefix without port",
			input:        "https://hub.example.com",
			expectedAddr: "hub.example.com",
			expectedHost: "hub.example.com",
		},
		{
			name:         "multiple trailing slashes",
			input:        "https://hub.example.com:8443///",
			expectedAddr: "hub.example.com:8443",
			expectedHost: "hub.example.com",
		},
		{
			name:         "ipv4 with http prefix",
			input:        "http://127.0.0.1:50051/",
			expectedAddr: "127.0.0.1:50051",
			expectedHost: "127.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanAddr, host := SanitizeAddress(tt.input)
			assert.Equal(t, tt.expectedAddr, cleanAddr)
			assert.Equal(t, tt.expectedHost, host)
		})
	}
}

func TestResolveAudience(t *testing.T) {
	tests := []struct {
		name           string
		host           string
		customAudience string
		expected       string
	}{
		{
			name:           "empty custom audience falls back to https://host",
			host:           "hub.example.com",
			customAudience: "",
			expected:       "https://hub.example.com",
		},
		{
			name:           "empty custom audience with host containing port strips port",
			host:           "hub.example.com:443",
			customAudience: "",
			expected:       "https://hub.example.com",
		},
		{
			name:           "custom audience overrides fallback",
			host:           "hub.example.com",
			customAudience: "https://custom-audience.googleusercontent.com",
			expected:       "https://custom-audience.googleusercontent.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved := ResolveAudience(tt.host, tt.customAudience)
			assert.Equal(t, tt.expected, resolved)
		})
	}
}

func TestFactory_GetClient_CacheKeyIsolation(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	s := grpc.NewServer()
	go func() {
		_ = s.Serve(lis)
	}()
	defer s.Stop()

	factory := NewFactory()
	defer factory.Close()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
	}

	ctx := context.Background()

	// 1. Same address and token, but different audiences -> must return distinct client instances
	clientAudienceA, err := factory.GetClient(ctx, "localhost:50051", "token-1", "audience-a", opts...)
	require.NoError(t, err)
	require.NotNil(t, clientAudienceA)

	clientAudienceB, err := factory.GetClient(ctx, "localhost:50051", "token-1", "audience-b", opts...)
	require.NoError(t, err)
	require.NotNil(t, clientAudienceB)

	assert.NotSame(t, clientAudienceA, clientAudienceB, "different audiences must not share cache entries")

	// 2. Calling with same audience again returns cached client
	clientAudienceA2, err := factory.GetClient(ctx, "localhost:50051", "token-1", "audience-a", opts...)
	require.NoError(t, err)
	assert.Same(t, clientAudienceA, clientAudienceA2, "same address, token, and audience must return cached client")

	// 3. Address sanitization: "http://localhost:50051/" with same token & audience should hit the same cache
	clientSanitized, err := factory.GetClient(ctx, "http://localhost:50051/", "token-1", "audience-a", opts...)
	require.NoError(t, err)
	assert.Same(t, clientAudienceA, clientSanitized, "sanitized address should match existing cached client")

	factory.mu.Lock()
	count := len(factory.clients)
	factory.mu.Unlock()
	assert.Equal(t, 2, count, "factory map should have exactly 2 clients (audience-a and audience-b)")
}
