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
)

func TestFactory_GetClient_Concurrent(t *testing.T) {
	// Start a dummy local gRPC listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := grpc.NewServer()
	go func() {
		_ = s.Serve(lis)
	}()
	defer s.Stop()

	os.Setenv("SUPERCARGO_MOCK_TOKEN", "mock-token-for-test")
	defer os.Unsetenv("SUPERCARGO_MOCK_TOKEN")

	factory := NewFactory()
	defer factory.Close()

	addr := lis.Addr().String()
	numGoroutines := 20
	clients := make([]*Client, numGoroutines)
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()
			c, err := factory.GetClient(context.Background(), addr, "mock-token")
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
