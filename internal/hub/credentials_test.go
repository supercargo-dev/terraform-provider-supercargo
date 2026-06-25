package hub

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
)

type mockTokenSource struct {
	token *oauth2.Token
	err   error
	calls int
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	m.calls++
	return m.token, m.err
}

func TestOIDCCredentials_GetRequestMetadata(t *testing.T) {
	t.Run("Refreshes token before expiry", func(t *testing.T) {
		expiry := time.Now().Add(30 * time.Second) // Less than 1 minute
		ts := &mockTokenSource{
			token: &oauth2.Token{
				AccessToken: "token1",
				Expiry:      expiry,
			},
		}

		creds := &OIDCCredentials{
			source: ts,
		}

		// First call should fetch token
		md, err := creds.GetRequestMetadata(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "Bearer token1", md["authorization"])
		assert.Equal(t, 1, ts.calls)

		// Second call should refresh because it's within the 1-minute window
		// We'll update the mock to return a new token
		ts.token = &oauth2.Token{
			AccessToken: "token2",
			Expiry:      time.Now().Add(10 * time.Minute),
		}

		md, err = creds.GetRequestMetadata(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "Bearer token2", md["authorization"])
		assert.Equal(t, 2, ts.calls)
	})

	t.Run("Uses cached token if far from expiry", func(t *testing.T) {
		expiry := time.Now().Add(10 * time.Minute)
		ts := &mockTokenSource{
			token: &oauth2.Token{
				AccessToken: "token1",
				Expiry:      expiry,
			},
		}

		creds := &OIDCCredentials{
			source: ts,
		}

		md, err := creds.GetRequestMetadata(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "Bearer token1", md["authorization"])
		assert.Equal(t, 1, ts.calls)

		// Second call should use cached token
		md, err = creds.GetRequestMetadata(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "Bearer token1", md["authorization"])
		assert.Equal(t, 1, ts.calls) // Still 1 call
	})
}
