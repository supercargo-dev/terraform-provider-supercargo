package provider

import (
	"context"
	"net"
	"sync"
	"testing"

	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	platformv1 "github.com/supercargo-dev/core/gen/go/platform/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/hub"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

type mockHubServer struct {
	hubv1.UnimplementedHubServiceServer

	mu sync.RWMutex

	RegisterTeamHook          func(context.Context, *hubv1.RegisterTeamRequest) (*hubv1.RegisterTeamResponse, error)
	GetTeamHook               func(context.Context, *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error)
	RegisterContractHook      func(context.Context, *hubv1.RegisterContractRequest) (*hubv1.RegisterContractResponse, error)
	GetContractHook           func(context.Context, *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error)
	CheckDownstreamImpactHook func(context.Context, *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error)
	RegisterProductHook       func(context.Context, *hubv1.RegisterProductRequest) (*hubv1.RegisterProductResponse, error)
	GetProductHook            func(context.Context, *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error)
}

func (m *mockHubServer) RegisterTeam(ctx context.Context, req *hubv1.RegisterTeamRequest) (*hubv1.RegisterTeamResponse, error) {
	m.mu.RLock()
	hook := m.RegisterTeamHook
	m.mu.RUnlock()
	if hook != nil {
		return hook(ctx, req)
	}
	return m.UnimplementedHubServiceServer.RegisterTeam(ctx, req)
}

func (m *mockHubServer) GetTeam(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
	m.mu.RLock()
	hook := m.GetTeamHook
	m.mu.RUnlock()
	if hook != nil {
		return hook(ctx, req)
	}
	return m.UnimplementedHubServiceServer.GetTeam(ctx, req)
}

func (m *mockHubServer) RegisterContract(ctx context.Context, req *hubv1.RegisterContractRequest) (*hubv1.RegisterContractResponse, error) {
	m.mu.RLock()
	hook := m.RegisterContractHook
	m.mu.RUnlock()
	if hook != nil {
		return hook(ctx, req)
	}
	return m.UnimplementedHubServiceServer.RegisterContract(ctx, req)
}

func (m *mockHubServer) GetContract(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
	m.mu.RLock()
	hook := m.GetContractHook
	m.mu.RUnlock()
	if hook != nil {
		return hook(ctx, req)
	}
	return m.UnimplementedHubServiceServer.GetContract(ctx, req)
}

func (m *mockHubServer) CheckDownstreamImpact(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
	m.mu.RLock()
	hook := m.CheckDownstreamImpactHook
	m.mu.RUnlock()
	if hook != nil {
		return hook(ctx, req)
	}
	return m.UnimplementedHubServiceServer.CheckDownstreamImpact(ctx, req)
}

func (m *mockHubServer) RegisterProduct(ctx context.Context, req *hubv1.RegisterProductRequest) (*hubv1.RegisterProductResponse, error) {
	m.mu.RLock()
	hook := m.RegisterProductHook
	m.mu.RUnlock()
	if hook != nil {
		return hook(ctx, req)
	}
	return m.UnimplementedHubServiceServer.RegisterProduct(ctx, req)
}

func (m *mockHubServer) GetProduct(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
	m.mu.RLock()
	hook := m.GetProductHook
	m.mu.RUnlock()
	if hook != nil {
		return hook(ctx, req)
	}
	return m.UnimplementedHubServiceServer.GetProduct(ctx, req)
}

func startMockHubServer(t *testing.T) (*mockHubServer, *ProviderData, string) {
	t.Helper()
	lis := bufconn.Listen(bufSize)

	grpcServer := grpc.NewServer()
	mockSrv := &mockHubServer{}
	hubv1.RegisterHubServiceServer(grpcServer, mockSrv)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	t.Cleanup(func() {
		grpcServer.Stop()
		_ = lis.Close()
	})

	addr := "localhost:50051"

	factory := hub.NewFactory()
	t.Cleanup(func() {
		_ = factory.Close()
	})

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	client, err := factory.GetClient(
		context.Background(),
		addr,
		"mock-token",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer),
	)
	require.NoError(t, err)

	providerData := &ProviderData{
		HubAddress: addr,
		HubClient:  client,
		Cache:      &sync.Map{},
	}

	return mockSrv, providerData, addr
}

func newTestPlan(ctx context.Context, t *testing.T, s schema.Schema, model any) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{
		Schema: s,
	}
	diags := plan.Set(ctx, model)
	require.False(t, diags.HasError(), "diags setting test plan: %v", diags)
	return plan
}

func newTestState(ctx context.Context, t *testing.T, s schema.Schema, model any) tfsdk.State {
	t.Helper()
	state := tfsdk.State{
		Schema: s,
	}
	diags := state.Set(ctx, model)
	require.False(t, diags.HasError(), "diags setting test state: %v", diags)
	return state
}

func newTestDataSourceConfig(ctx context.Context, t *testing.T, s dschema.Schema, model any) tfsdk.Config {
	t.Helper()
	state := tfsdk.State{
		Schema: s,
	}
	diags := state.Set(ctx, model)
	require.False(t, diags.HasError(), "diags setting test datasource config: %v", diags)
	return tfsdk.Config{
		Schema: s,
		Raw:    state.Raw,
	}
}

func newTestDataSourceState(ctx context.Context, t *testing.T, s dschema.Schema, model any) tfsdk.State {
	t.Helper()
	state := tfsdk.State{
		Schema: s,
	}
	diags := state.Set(ctx, model)
	require.False(t, diags.HasError(), "diags setting test datasource state: %v", diags)
	return state
}

func TestMockHubServer_LifecycleAndHooks(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, addr := startMockHubServer(t)
	require.NotEmpty(t, addr)
	require.NotNil(t, providerData)
	require.NotNil(t, providerData.HubClient)

	// Set hook and test execution
	called := false
	mockSrv.mu.Lock()
	mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
		called = true
		assert.Equal(t, "test-team", req.Name)
		return &hubv1.GetTeamResponse{
			Team: &platformv1.Team{
				Name: "test-team",
				Urn:  "urn:supercargo:hub:team:test-team",
			},
		}, nil
	}
	mockSrv.mu.Unlock()

	resp, err := providerData.HubClient.GetTeam(ctx, &hubv1.GetTeamRequest{Name: "test-team"})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "test-team", resp.Team.Name)
}
