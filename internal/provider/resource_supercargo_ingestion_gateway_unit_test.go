package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSupercargoIngestionGatewayResource_Metadata(t *testing.T) {
	r := NewSupercargoIngestionGatewayResource()
	req := resource.MetadataRequest{ProviderTypeName: "supercargo"}
	var resp resource.MetadataResponse

	r.Metadata(context.Background(), req, &resp)
	assert.Equal(t, "supercargo_ingestion_gateway", resp.TypeName)
}

func TestSupercargoIngestionGatewayResource_Schema(t *testing.T) {
	r := NewSupercargoIngestionGatewayResource()
	req := resource.SchemaRequest{}
	var resp resource.SchemaResponse

	r.Schema(context.Background(), req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "schema errors: %v", resp.Diagnostics)

	assert.NotNil(t, resp.Schema.Attributes["id"])
	assert.True(t, resp.Schema.Attributes["id"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["contract_id"])
	assert.True(t, resp.Schema.Attributes["contract_id"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["contract_version"])
	assert.True(t, resp.Schema.Attributes["contract_version"].IsRequired())
}

func TestSupercargoIngestionGatewayResource_Configure(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider data", func(t *testing.T) {
		r := &supercargoIngestionGatewayResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: nil}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Nil(t, r.client)
	})

	t.Run("invalid provider data type", func(t *testing.T) {
		r := &supercargoIngestionGatewayResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: "invalid-type"}, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Unexpected Resource Configure Data Type")
		assert.Nil(t, r.client)
	})

	t.Run("valid provider data", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoIngestionGatewayResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: providerData}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, providerData, r.client)
	})
}

func TestSupercargoIngestionGatewayResource_Create_Success(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var gwSchemaResp resource.SchemaResponse
	r := &supercargoIngestionGatewayResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

	called := false
	mockSrv.mu.Lock()
	mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
		called = true
		assert.Equal(t, "urn:supercargo:hub:contract:orders", req.ContractUrn)
		assert.Equal(t, "v1.0.0", req.Version)
		return &hubv1.GetContractResponse{
			Contract: &hubv1.DataContract{
				Meta: &hubv1.Meta{
					Urn:     "urn:supercargo:hub:contract:orders",
					Version: "v1.0.0",
				},
			},
		}, nil
	}
	mockSrv.mu.Unlock()

	planModel := supercargoIngestionGatewayResourceModel{
		ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
		ContractVersion: types.StringValue("v1.0.0"),
	}

	req := resource.CreateRequest{
		Plan: newTestPlan(ctx, t, gwSchemaResp.Schema, planModel),
	}
	var resp resource.CreateResponse
	resp.State = newTestState(ctx, t, gwSchemaResp.Schema, planModel)

	r.Create(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "create error: %v", resp.Diagnostics)
	assert.True(t, called)

	var stateModel supercargoIngestionGatewayResourceModel
	diags := resp.State.Get(ctx, &stateModel)
	require.False(t, diags.HasError())
	assert.Equal(t, "gateway-urn:supercargo:hub:contract:orders", stateModel.ID.ValueString())
	assert.Equal(t, "urn:supercargo:hub:contract:orders", stateModel.ContractID.ValueString())
	assert.Equal(t, "v1.0.0", stateModel.ContractVersion.ValueString())
}

func TestSupercargoIngestionGatewayResource_Create_ContractNotFound(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var gwSchemaResp resource.SchemaResponse
	r := &supercargoIngestionGatewayResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
		return nil, status.Error(codes.NotFound, "contract not found in catalog")
	}
	mockSrv.mu.Unlock()

	planModel := supercargoIngestionGatewayResourceModel{
		ContractID:      types.StringValue("urn:supercargo:hub:contract:nonexistent"),
		ContractVersion: types.StringValue("v1.0.0"),
	}

	req := resource.CreateRequest{
		Plan: newTestPlan(ctx, t, gwSchemaResp.Schema, planModel),
	}
	var resp resource.CreateResponse

	r.Create(ctx, req, &resp)
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Governance Validation Failed")
	assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "contract not found in catalog")
}

func TestSupercargoIngestionGatewayResource_Create_Unconfigured(t *testing.T) {
	ctx := context.Background()
	r := &supercargoIngestionGatewayResource{}

	var gwSchemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

	planModel := supercargoIngestionGatewayResourceModel{
		ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
		ContractVersion: types.StringValue("v1.0.0"),
	}

	req := resource.CreateRequest{
		Plan: newTestPlan(ctx, t, gwSchemaResp.Schema, planModel),
	}
	var resp resource.CreateResponse

	r.Create(ctx, req, &resp)
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
}

func TestSupercargoIngestionGatewayResource_Read_ContractExists(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var gwSchemaResp resource.SchemaResponse
	r := &supercargoIngestionGatewayResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
		return &hubv1.GetContractResponse{
			Contract: &hubv1.DataContract{
				Meta: &hubv1.Meta{
					Urn:     "urn:supercargo:hub:contract:orders",
					Version: "v1.0.0",
				},
			},
		}, nil
	}
	mockSrv.mu.Unlock()

	stateModel := supercargoIngestionGatewayResourceModel{
		ID:              types.StringValue("gateway-urn:supercargo:hub:contract:orders"),
		ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
		ContractVersion: types.StringValue("v1.0.0"),
	}

	req := resource.ReadRequest{
		State: newTestState(ctx, t, gwSchemaResp.Schema, stateModel),
	}
	var resp resource.ReadResponse
	resp.State = newTestState(ctx, t, gwSchemaResp.Schema, stateModel)

	r.Read(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "read error: %v", resp.Diagnostics)

	var updatedState supercargoIngestionGatewayResourceModel
	diags := resp.State.Get(ctx, &updatedState)
	require.False(t, diags.HasError())
	assert.Equal(t, "gateway-urn:supercargo:hub:contract:orders", updatedState.ID.ValueString())
	assert.Equal(t, "urn:supercargo:hub:contract:orders", updatedState.ContractID.ValueString())
	assert.Equal(t, "v1.0.0", updatedState.ContractVersion.ValueString())
}

func TestSupercargoIngestionGatewayResource_Read_ContractNotFound(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var gwSchemaResp resource.SchemaResponse
	r := &supercargoIngestionGatewayResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
		return nil, status.Error(codes.NotFound, "contract has been deleted from hub")
	}
	mockSrv.mu.Unlock()

	stateModel := supercargoIngestionGatewayResourceModel{
		ID:              types.StringValue("gateway-urn:supercargo:hub:contract:orders"),
		ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
		ContractVersion: types.StringValue("v1.0.0"),
	}

	req := resource.ReadRequest{
		State: newTestState(ctx, t, gwSchemaResp.Schema, stateModel),
	}
	var resp resource.ReadResponse
	resp.State = newTestState(ctx, t, gwSchemaResp.Schema, stateModel)

	r.Read(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError())
	assert.True(t, resp.State.Raw.IsNull(), "gateway resource should be removed from state when contract is not found")
}

func TestSupercargoIngestionGatewayResource_Read_ServerError(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var gwSchemaResp resource.SchemaResponse
	r := &supercargoIngestionGatewayResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
		return nil, status.Error(codes.Internal, "storage failure")
	}
	mockSrv.mu.Unlock()

	stateModel := supercargoIngestionGatewayResourceModel{
		ID:              types.StringValue("gateway-urn:supercargo:hub:contract:orders"),
		ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
		ContractVersion: types.StringValue("v1.0.0"),
	}

	req := resource.ReadRequest{
		State: newTestState(ctx, t, gwSchemaResp.Schema, stateModel),
	}
	var resp resource.ReadResponse
	resp.State = newTestState(ctx, t, gwSchemaResp.Schema, stateModel)

	r.Read(ctx, req, &resp)
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Contract for Gateway")
	assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "storage failure")
}

func TestSupercargoIngestionGatewayResource_Read_Unconfigured(t *testing.T) {
	ctx := context.Background()
	r := &supercargoIngestionGatewayResource{}

	var gwSchemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

	stateModel := supercargoIngestionGatewayResourceModel{
		ID:              types.StringValue("gateway-urn:supercargo:hub:contract:orders"),
		ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
		ContractVersion: types.StringValue("v1.0.0"),
	}

	req := resource.ReadRequest{
		State: newTestState(ctx, t, gwSchemaResp.Schema, stateModel),
	}
	var resp resource.ReadResponse

	r.Read(ctx, req, &resp)
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
}

func TestSupercargoIngestionGatewayResource_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoIngestionGatewayResource{}
		var gwSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

		planModel := supercargoIngestionGatewayResourceModel{
			ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
			ContractVersion: types.StringValue("v2.0.0"),
		}

		req := resource.UpdateRequest{
			Plan: newTestPlan(ctx, t, gwSchemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse
		r.Update(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("configured provider", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoIngestionGatewayResource{client: providerData}
		var gwSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

		planModel := supercargoIngestionGatewayResourceModel{
			ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
			ContractVersion: types.StringValue("v2.0.0"),
		}

		req := resource.UpdateRequest{
			Plan: newTestPlan(ctx, t, gwSchemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse
		resp.State = newTestState(ctx, t, gwSchemaResp.Schema, planModel)

		r.Update(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError())

		var stateModel supercargoIngestionGatewayResourceModel
		diags := resp.State.Get(ctx, &stateModel)
		require.False(t, diags.HasError())
		assert.Equal(t, "gateway-urn:supercargo:hub:contract:orders", stateModel.ID.ValueString())
	})
}

func TestSupercargoIngestionGatewayResource_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoIngestionGatewayResource{}
		var gwSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

		stateModel := supercargoIngestionGatewayResourceModel{
			ID:              types.StringValue("gateway-urn:supercargo:hub:contract:orders"),
			ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
			ContractVersion: types.StringValue("v1.0.0"),
		}

		req := resource.DeleteRequest{
			State: newTestState(ctx, t, gwSchemaResp.Schema, stateModel),
		}
		var resp resource.DeleteResponse
		r.Delete(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("configured provider", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoIngestionGatewayResource{client: providerData}
		var gwSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &gwSchemaResp)

		stateModel := supercargoIngestionGatewayResourceModel{
			ID:              types.StringValue("gateway-urn:supercargo:hub:contract:orders"),
			ContractID:      types.StringValue("urn:supercargo:hub:contract:orders"),
			ContractVersion: types.StringValue("v1.0.0"),
		}

		req := resource.DeleteRequest{
			State: newTestState(ctx, t, gwSchemaResp.Schema, stateModel),
		}
		var resp resource.DeleteResponse
		r.Delete(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})
}
