package hub

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
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

// ErrFactoryClosed is returned when attempting to obtain a client from a closed Factory.
var ErrFactoryClosed = errors.New("hub factory is closed")

// Client handles communication with the Supercargo Hub.
type Client struct {
	hubv1.HubServiceClient
	conn   *grpc.ClientConn
	target string
}

// Factory manages a pool of Hub clients.
type Factory struct {
	mu      sync.Mutex
	closed  bool
	clients map[string]*Client
}

// NewFactory creates a new Hub client factory.
func NewFactory() *Factory {
	return &Factory{
		clients: make(map[string]*Client),
	}
}

// SanitizeAddress strips protocol prefixes and trailing slashes from an address,
// returning the clean target address and the isolated host name.
func SanitizeAddress(address string) (cleanAddress, host string) {
	clean := address
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, "https://") {
		clean = clean[8:]
	} else if strings.HasPrefix(lower, "http://") {
		clean = clean[7:]
	}
	clean = strings.TrimRight(clean, "/")

	host = clean
	if h, _, err := net.SplitHostPort(clean); err == nil {
		host = h
	}
	return clean, host
}

// ResolveAudience returns the custom audience if provided, or defaults to "https://" + host.
func ResolveAudience(host, customAudience string) string {
	if customAudience != "" {
		return customAudience
	}
	h := host
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		h = splitHost
	}
	return "https://" + h
}

// IsLoopbackHost returns true if the host is localhost or a loopback IP address.
func IsLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

type tlsContextKey struct{}

// WithTLS returns a child context with an explicit TLS configuration override.
func WithTLS(ctx context.Context, useTLS bool) context.Context {
	return context.WithValue(ctx, tlsContextKey{}, useTLS)
}

// GetClient returns a Hub client for the given address, token, and audience using default TLS settings.
func (f *Factory) GetClient(ctx context.Context, address string, token string, audience string, opts ...grpc.DialOption) (*Client, error) {
	return f.GetClientWithTLS(ctx, address, token, audience, nil, opts...)
}

// GetClientWithTLS returns a Hub client with an optional explicit TLS override.
func (f *Factory) GetClientWithTLS(ctx context.Context, address string, token string, audience string, useTLS *bool, opts ...grpc.DialOption) (*Client, error) {
	cleanAddress, host := SanitizeAddress(address)
	resolvedAudience := ResolveAudience(host, audience)

	// Determine if we use TLS: default is false for localhost/loopback, true for remote hosts.
	isLoopback := IsLoopbackHost(host)
	useSecure := !isLoopback
	if useTLS != nil {
		useSecure = *useTLS
	} else if override, ok := ctx.Value(tlsContextKey{}).(bool); ok {
		useSecure = override
	}

	tokenHash := ""
	if token != "" {
		h := sha256.Sum256([]byte(token))
		tokenHash = hex.EncodeToString(h[:])
	}

	cacheKey := fmt.Sprintf("%s|%s|%s|%t", cleanAddress, tokenHash, resolvedAudience, useSecure)

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil, ErrFactoryClosed
	}
	if client, ok := f.clients[cacheKey]; ok {
		f.mu.Unlock()
		return client, nil
	}
	f.mu.Unlock()

	dialOpts := make([]grpc.DialOption, 0, len(opts)+5)
	dialOpts = append(dialOpts, opts...)

	if !useSecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))

		authToken := token
		if authToken == "" {
			authToken = os.Getenv("SUPERCARGO_MOCK_TOKEN")
		}
		if authToken == "" {
			return nil, fmt.Errorf("token or SUPERCARGO_MOCK_TOKEN environment variable must be set for local development")
		}

		ts := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: authToken,
			Expiry:      time.Now().Add(1 * time.Hour),
		})
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(NewInsecureOIDCCredentials(ts)))
	} else {
		// Use OIDC for cloud / secure environments
		var ts oauth2.TokenSource
		if token != "" {
			// If a token is explicitly provided (e.g., from an ID token data source), use it.
			ts = oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken: token,
			})
		} else {
			// Fallback to ADC with resolved audience
			var err error
			ts, err = idtoken.NewTokenSource(ctx, resolvedAudience)
			if err != nil {
				return nil, RedactBearerToken(fmt.Errorf("failed to create OIDC token source for %s: %w", resolvedAudience, err))
			}
		}

		creds := NewOIDCCredentials(ts)
		tlsConfig := buildTLSConfig(x509.SystemCertPool)
		dialOpts = append(dialOpts,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
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
	dialOpts = append(dialOpts, grpc.WithDefaultServiceConfig(retryPolicy))

	conn, err := grpc.NewClient(cleanAddress, dialOpts...)
	if err != nil {
		return nil, RedactBearerToken(fmt.Errorf("failed to dial hub at %s: %w", cleanAddress, err))
	}

	client := &Client{
		HubServiceClient: hubv1.NewHubServiceClient(conn),
		conn:             conn,
		target:           cleanAddress,
	}

	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		_ = conn.Close()
		return nil, ErrFactoryClosed
	}
	if existing, ok := f.clients[cacheKey]; ok {
		f.mu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	f.clients[cacheKey] = client
	f.mu.Unlock()

	return client, nil
}

// Close closes all client connections.
func (f *Factory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil
	}
	f.closed = true

	var errs []error
	for _, client := range f.clients {
		if err := client.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close connection to %s: %w", client.target, err))
		}
	}
	f.clients = make(map[string]*Client)

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func buildTLSConfig(systemPoolFn func() (*x509.CertPool, error)) *tls.Config {
	var systemPoolFailed bool
	rootCAs, err := systemPoolFn()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
		systemPoolFailed = true
	}

	seenPaths := make(map[string]struct{})
	caFiles := make([]string, 0, 4)

	addFile := func(path string) {
		cleanPath := filepath.Clean(strings.TrimSpace(path))
		if cleanPath == "" || cleanPath == "." {
			return
		}
		if _, seen := seenPaths[cleanPath]; !seen {
			seenPaths[cleanPath] = struct{}{}
			caFiles = append(caFiles, cleanPath)
		}
	}

	if certFile := os.Getenv("SSL_CERT_FILE"); certFile != "" {
		addFile(certFile)
	}
	if curlBundle := os.Getenv("CURL_CA_BUNDLE"); curlBundle != "" {
		addFile(curlBundle)
	}
	if certDir := os.Getenv("SSL_CERT_DIR"); certDir != "" {
		for _, dir := range filepath.SplitList(certDir) {
			cleanDir := filepath.Clean(strings.TrimSpace(dir))
			if cleanDir == "" || cleanDir == "." {
				continue
			}
			if entries, err := os.ReadDir(cleanDir); err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					name := entry.Name()
					if strings.HasSuffix(name, ".crt") || strings.HasSuffix(name, ".pem") {
						addFile(filepath.Join(cleanDir, name))
					}
				}
			}
		}
	}

	var certsAdded int
	for _, caFile := range caFiles {
		data, err := os.ReadFile(caFile)
		if err == nil && len(data) > 0 {
			if rootCAs.AppendCertsFromPEM(data) {
				certsAdded++
			}
		}
	}

	// Empty CertPool blackhole prevention:
	// If SystemCertPool failed and no valid certificates were added from the environment,
	// setting RootCAs to an empty pool would cause all TLS handshakes to fail.
	// Falling back to nil allows standard Go fallback resolution.
	if systemPoolFailed && certsAdded == 0 {
		rootCAs = nil
	}

	return &tls.Config{
		RootCAs:            rootCAs,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	}
}

var bearerRegex = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9\-._~+/]+=*`)

// RedactBearerToken redacts sensitive bearer tokens from error messages and appends
// a Darwin Seatbelt TLS diagnostic hint if a certificate/handshake issue is detected.
func RedactBearerToken(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	msg = bearerRegex.ReplaceAllString(msg, "Bearer [REDACTED]")

	lower := strings.ToLower(msg)
	if strings.Contains(lower, "x509") || strings.Contains(lower, "certificate") || strings.Contains(lower, "handshake") {
		const darwinHint = "In restricted Darwin Seatbelt environments, export custom root CA bundles via SSL_CERT_FILE or CURL_CA_BUNDLE"
		if !strings.Contains(msg, darwinHint) {
			msg = fmt.Sprintf("%s. %s", msg, darwinHint)
		}
	}
	return errors.New(msg)
}
