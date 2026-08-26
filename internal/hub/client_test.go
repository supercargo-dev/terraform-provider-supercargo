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

	client, err := factory.GetClient(context.Background(), "localhost:50051", "mock-token")
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

	client, err := factory.GetClient(context.Background(), "localhost:50051", "")
	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token or SUPERCARGO_MOCK_TOKEN environment variable must be set")
}
