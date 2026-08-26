package provider

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const sampleValidSchemaJSON = `[
	{
		"name": "user_id",
		"type": "STRING",
		"mode": "REQUIRED",
		"description": "Unique identifier for the user"
	},
	{
		"name": "email",
		"type": "STRING",
		"mode": "NULLABLE"
	}
]`

func TestSupercargoContractVersionResource_Metadata(t *testing.T) {
	r := NewSupercargoContractVersionResource()
	req := resource.MetadataRequest{ProviderTypeName: "supercargo"}
	var resp resource.MetadataResponse

	r.Metadata(context.Background(), req, &resp)
	assert.Equal(t, "supercargo_contract_version", resp.TypeName)
}

func TestSupercargoContractVersionResource_Schema(t *testing.T) {
	r := NewSupercargoContractVersionResource()
	req := resource.SchemaRequest{}
	var resp resource.SchemaResponse

	r.Schema(context.Background(), req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "schema errors: %v", resp.Diagnostics)

	assert.NotNil(t, resp.Schema.Attributes["urn"])
	assert.True(t, resp.Schema.Attributes["urn"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["version"])
	assert.True(t, resp.Schema.Attributes["version"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["content_hash"])
	assert.True(t, resp.Schema.Attributes["content_hash"].IsOptional())

	assert.NotNil(t, resp.Schema.Attributes["commit_sha"])
	assert.True(t, resp.Schema.Attributes["commit_sha"].IsOptional())

	assert.NotNil(t, resp.Schema.Attributes["schema_json"])
	assert.True(t, resp.Schema.Attributes["schema_json"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["data_asset"])
	assert.True(t, resp.Schema.Attributes["data_asset"].IsOptional())
	assert.True(t, resp.Schema.Attributes["data_asset"].IsComputed())
}

func TestSupercargoContractVersionResource_Configure(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider data", func(t *testing.T) {
		r := &supercargoContractVersionResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: nil}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Nil(t, r.client)
	})

	t.Run("invalid provider data type", func(t *testing.T) {
		r := &supercargoContractVersionResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: "invalid-type"}, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Unexpected Resource Configure Type")
		assert.Nil(t, r.client)
	})

	t.Run("valid provider data", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: providerData}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, providerData, r.client)
	})
}

func TestSupercargoContractVersionResource_ModifyPlan(t *testing.T) {
	ctx := context.Background()

	t.Run("null plan skipped early", func(t *testing.T) {
		r := &supercargoContractVersionResource{}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		req := resource.ModifyPlanRequest{}
		req.Plan.Raw = tftypes.NewValue(tftypes.Object{}, nil)
		var resp resource.ModifyPlanResponse

		r.ModifyPlan(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})

	t.Run("unconfigured provider error", func(t *testing.T) {
		r := &supercargoContractVersionResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:events"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("hash123"),
			CommitSha:   types.StringValue("sha123"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/events"),
		}

		req := resource.ModifyPlanRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.ModifyPlanResponse

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("unconfigured hub client skipped early", func(t *testing.T) {
		r := &supercargoContractVersionResource{
			client: &ProviderData{
				HubClient: nil,
				Cache:     &sync.Map{},
			},
		}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:events"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("hash123"),
			CommitSha:   types.StringValue("sha123"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/events"),
		}

		req := resource.ModifyPlanRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.ModifyPlanResponse

		r.ModifyPlan(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})

	t.Run("invalid schema JSON", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:events"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("hash123"),
			CommitSha:   types.StringValue("sha123"),
			SchemaJSON:  types.StringValue("{invalid-json}"),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/events"),
		}

		req := resource.ModifyPlanRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.ModifyPlanResponse

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Invalid Schema JSON")
	})

	t.Run("breaking change detected", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return &hubv1.CheckDownstreamImpactResponse{
				Severity:        hubv1.ImpactSeverity_IMPACT_SEVERITY_BREAKING,
				BreakingChanges: []string{"field 'user_id' removed", "field 'type' type changed from STRING to INT64"},
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:events"),
			Version:     types.StringValue("v2.0.0"),
			ContentHash: types.StringValue("hash-breaking"),
			CommitSha:   types.StringValue("sha123"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/events"),
		}

		req := resource.ModifyPlanRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.ModifyPlanResponse

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Breaking Change Detected (Plan-Time Governance)")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "field 'user_id' removed")
	})

	t.Run("non-breaking change and cache hit", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		var callCount int32
		mockSrv.mu.Lock()
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			atomic.AddInt32(&callCount, 1)
			assert.Equal(t, "urn:supercargo:hub:contract:events", req.Contract.Meta.Urn)
			return &hubv1.CheckDownstreamImpactResponse{
				Severity: hubv1.ImpactSeverity_IMPACT_SEVERITY_NONE,
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:events"),
			Version:     types.StringValue("v1.1.0"),
			ContentHash: types.StringValue("hash-nonbreaking-1"),
			CommitSha:   types.StringValue("sha123"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/events"),
		}

		req := resource.ModifyPlanRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}

		// First call: executes RPC and populates cache
		var resp1 resource.ModifyPlanResponse
		r.ModifyPlan(ctx, req, &resp1)
		require.False(t, resp1.Diagnostics.HasError(), "unexpected diags on 1st call: %v", resp1.Diagnostics)
		assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))

		// Second call with same plan: uses cache, no additional RPC
		var resp2 resource.ModifyPlanResponse
		r.ModifyPlan(ctx, req, &resp2)
		require.False(t, resp2.Diagnostics.HasError(), "unexpected diags on 2nd call: %v", resp2.Diagnostics)
		assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
	})

	t.Run("hub CheckDownstreamImpact error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return nil, status.Error(codes.Unavailable, "hub service unavailable")
		}
		mockSrv.mu.Unlock()

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:events"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("hash-err"),
			CommitSha:   types.StringValue("sha123"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/events"),
		}

		req := resource.ModifyPlanRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.ModifyPlanResponse

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Governance Handshake Failed")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "hub service unavailable")
	})
}

func TestSupercargoContractVersionResource_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		var registeredContract *hubv1.DataContract
		mockSrv.mu.Lock()
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return &hubv1.CheckDownstreamImpactResponse{
				Severity: hubv1.ImpactSeverity_IMPACT_SEVERITY_NONE,
			}, nil
		}
		mockSrv.RegisterContractHook = func(ctx context.Context, req *hubv1.RegisterContractRequest) (*hubv1.RegisterContractResponse, error) {
			registeredContract = req.Contract
			return &hubv1.RegisterContractResponse{Contract: req.Contract}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("sha256-hash-val"),
			CommitSha:   types.StringValue("commit-sha-val"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/orders"),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, planModel)

		r.Create(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected create errors: %v", resp.Diagnostics)

		require.NotNil(t, registeredContract)
		assert.Equal(t, "urn:supercargo:hub:contract:orders", registeredContract.Meta.Urn)
		assert.Equal(t, "v1.0.0", registeredContract.Meta.Version)
		assert.Equal(t, "sha256-hash-val", registeredContract.Meta.ContentHash)
		assert.Equal(t, "commit-sha-val", registeredContract.Meta.CommitSha)
		assert.Equal(t, "go://github.com/supercargo-dev/core/orders", registeredContract.Meta.DataAsset)
		assert.Len(t, registeredContract.Schema, 2)

		var stateModel supercargoContractVersionResourceModel
		diags := resp.State.Get(ctx, &stateModel)
		require.False(t, diags.HasError())
		assert.Equal(t, "urn:supercargo:hub:contract:orders", stateModel.URN.ValueString())
		assert.Equal(t, "v1.0.0", stateModel.Version.ValueString())
	})

	t.Run("success with unknown data_asset populated from proto", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return &hubv1.CheckDownstreamImpactResponse{
				Severity: hubv1.ImpactSeverity_IMPACT_SEVERITY_NONE,
			}, nil
		}
		mockSrv.RegisterContractHook = func(ctx context.Context, req *hubv1.RegisterContractRequest) (*hubv1.RegisterContractResponse, error) {
			return &hubv1.RegisterContractResponse{Contract: req.Contract}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("sha256-hash-val"),
			CommitSha:   types.StringValue("commit-sha-val"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringUnknown(),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, planModel)

		r.Create(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected create errors: %v", resp.Diagnostics)

		var stateModel supercargoContractVersionResourceModel
		diags := resp.State.Get(ctx, &stateModel)
		require.False(t, diags.HasError())
		assert.True(t, stateModel.DataAsset.IsNull())
	})

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoContractVersionResource{}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("sha256-hash-val"),
			CommitSha:   types.StringValue("commit-sha-val"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/orders"),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("invalid schema JSON", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("sha256-hash-val"),
			CommitSha:   types.StringValue("commit-sha-val"),
			SchemaJSON:  types.StringValue("invalid json text"),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/orders"),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Invalid Schema JSON")
	})

	t.Run("breaking change detected", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return &hubv1.CheckDownstreamImpactResponse{
				Severity:        hubv1.ImpactSeverity_IMPACT_SEVERITY_BREAKING,
				BreakingChanges: []string{"field 'email' removed"},
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v2.0.0"),
			ContentHash: types.StringValue("sha256-hash-val"),
			CommitSha:   types.StringValue("commit-sha-val"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/orders"),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Breaking Change Detected")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "field 'email' removed")
	})

	t.Run("hub CheckDownstreamImpact error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return nil, status.Error(codes.Internal, "impact calculation failure")
		}
		mockSrv.mu.Unlock()

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("sha256-hash-val"),
			CommitSha:   types.StringValue("commit-sha-val"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/orders"),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Governance Handshake Failed")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "impact calculation failure")
	})

	t.Run("hub RegisterContract error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return &hubv1.CheckDownstreamImpactResponse{
				Severity: hubv1.ImpactSeverity_IMPACT_SEVERITY_NONE,
			}, nil
		}
		mockSrv.RegisterContractHook = func(ctx context.Context, req *hubv1.RegisterContractRequest) (*hubv1.RegisterContractResponse, error) {
			return nil, status.Error(codes.AlreadyExists, "contract version already registered")
		}
		mockSrv.mu.Unlock()

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("sha256-hash-val"),
			CommitSha:   types.StringValue("commit-sha-val"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringValue("go://github.com/supercargo-dev/core/orders"),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Registering Contract")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "contract version already registered")
	})
}

func TestSupercargoContractVersionResource_Read(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			assert.Equal(t, "urn:supercargo:hub:contract:orders", req.ContractUrn)
			assert.Equal(t, "v1.0.0", req.Version)
			return &hubv1.GetContractResponse{
				Contract: &hubv1.DataContract{
					Meta: &hubv1.Meta{
						Urn:         "urn:supercargo:hub:contract:orders",
						Version:     "v1.0.0",
						ContentHash: "updated-content-hash",
						CommitSha:   "updated-commit-sha",
						DataAsset:   "go://github.com/supercargo-dev/core/orders-v1",
					},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("old-hash"),
			CommitSha:   types.StringValue("old-sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, stateModel)

		r.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected read error: %v", resp.Diagnostics)

		var updatedState supercargoContractVersionResourceModel
		diags := resp.State.Get(ctx, &updatedState)
		require.False(t, diags.HasError())
		assert.Equal(t, "updated-content-hash", updatedState.ContentHash.ValueString())
		assert.Equal(t, "updated-commit-sha", updatedState.CommitSha.ValueString())
		assert.Equal(t, "go://github.com/supercargo-dev/core/orders-v1", updatedState.DataAsset.ValueString())
	})

	t.Run("not found removes resource from state", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return nil, status.Error(codes.NotFound, "contract version not found")
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("old-hash"),
			CommitSha:   types.StringValue("old-sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, stateModel)

		r.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError())
		assert.True(t, resp.State.Raw.IsNull(), "state should be null after NotFound removal")
	})

	t.Run("hub server error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return nil, status.Error(codes.Internal, "backend database failure")
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("old-hash"),
			CommitSha:   types.StringValue("old-sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse

		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Contract")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "backend database failure")
	})

	t.Run("empty contract response", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return &hubv1.GetContractResponse{Contract: nil}, nil
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("old-hash"),
			CommitSha:   types.StringValue("old-sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse

		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Contract")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "Hub returned an empty or invalid contract response.")
	})

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoContractVersionResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("old-hash"),
			CommitSha:   types.StringValue("old-sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse

		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})
}

func TestSupercargoContractVersionResource_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoContractVersionResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("hash"),
			CommitSha:   types.StringValue("sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.UpdateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse

		r.Update(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("configured provider no-op", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("hash"),
			CommitSha:   types.StringValue("sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.UpdateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, planModel)

		r.Update(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})
}

func TestSupercargoContractVersionResource_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoContractVersionResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("hash"),
			CommitSha:   types.StringValue("sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.DeleteRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.DeleteResponse

		r.Delete(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("configured provider no-op", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoContractVersionResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoContractVersionResourceModel{
			URN:         types.StringValue("urn:supercargo:hub:contract:orders"),
			Version:     types.StringValue("v1.0.0"),
			ContentHash: types.StringValue("hash"),
			CommitSha:   types.StringValue("sha"),
			SchemaJSON:  types.StringValue(sampleValidSchemaJSON),
			DataAsset:   types.StringNull(),
		}

		req := resource.DeleteRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.DeleteResponse

		r.Delete(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})
}
