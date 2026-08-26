package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	platformv1 "github.com/supercargo-dev/core/gen/go/platform/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSupercargoTeamResource_Metadata(t *testing.T) {
	r := NewSupercargoTeamResource()
	req := resource.MetadataRequest{ProviderTypeName: "supercargo"}
	var resp resource.MetadataResponse

	r.Metadata(context.Background(), req, &resp)
	assert.Equal(t, "supercargo_team", resp.TypeName)
}

func TestSupercargoTeamResource_Schema(t *testing.T) {
	r := NewSupercargoTeamResource()
	req := resource.SchemaRequest{}
	var resp resource.SchemaResponse

	r.Schema(context.Background(), req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "schema has errors: %v", resp.Diagnostics)

	assert.NotNil(t, resp.Schema.Attributes["name"])
	assert.True(t, resp.Schema.Attributes["name"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["urn"])
	assert.True(t, resp.Schema.Attributes["urn"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["data_asset"])
	assert.True(t, resp.Schema.Attributes["data_asset"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["members"])
	assert.True(t, resp.Schema.Attributes["members"].IsOptional())

	assert.NotNil(t, resp.Schema.Attributes["metadata"])
	assert.True(t, resp.Schema.Attributes["metadata"].IsOptional())
}

func TestSupercargoTeamResource_Configure(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider data", func(t *testing.T) {
		r := &supercargoTeamResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: nil}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Nil(t, r.client)
	})

	t.Run("invalid provider data type", func(t *testing.T) {
		r := &supercargoTeamResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: "invalid-string"}, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Unexpected Resource Configure Data Type")
		assert.Nil(t, r.client)
	})

	t.Run("valid provider data", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoTeamResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: providerData}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, providerData, r.client)
	})
}

func TestSupercargoTeamResource_Create_Success(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var teamSchemaResp resource.SchemaResponse
	r := &supercargoTeamResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

	var registeredTeam *platformv1.Team
	mockSrv.mu.Lock()
	mockSrv.RegisterTeamHook = func(ctx context.Context, req *hubv1.RegisterTeamRequest) (*hubv1.RegisterTeamResponse, error) {
		registeredTeam = req.Team
		return &hubv1.RegisterTeamResponse{Team: req.Team}, nil
	}
	mockSrv.mu.Unlock()

	membersVal, diags := types.ListValueFrom(ctx, types.StringType, []string{"alice@example.com", "bob@example.com"})
	require.False(t, diags.HasError())

	metaVal, diags := types.MapValueFrom(ctx, types.StringType, map[string]string{"cost_center": "42", "tier": "gold"})
	require.False(t, diags.HasError())

	planModel := supercargoTeamResourceModel{
		Name:      types.StringValue("analytics-core"),
		DataAsset: types.StringValue("go://github.com/supercargo-dev/core/analytics"),
		Members:   membersVal,
		Metadata:  metaVal,
	}

	req := resource.CreateRequest{
		Plan: newTestPlan(ctx, t, teamSchemaResp.Schema, planModel),
	}
	var resp resource.CreateResponse
	resp.State = newTestState(ctx, t, teamSchemaResp.Schema, planModel)

	r.Create(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "unexpected create errors: %v", resp.Diagnostics)

	require.NotNil(t, registeredTeam)
	assert.Equal(t, "analytics-core", registeredTeam.Name)
	assert.Equal(t, "urn:supercargo:hub:team:analytics-core", registeredTeam.Urn)
	assert.Equal(t, "go://github.com/supercargo-dev/core/analytics", registeredTeam.DataAsset)
	assert.Equal(t, []string{"alice@example.com", "bob@example.com"}, registeredTeam.Members)
	assert.Equal(t, map[string]string{"cost_center": "42", "tier": "gold"}, registeredTeam.Metadata)

	var stateModel supercargoTeamResourceModel
	diags = resp.State.Get(ctx, &stateModel)
	require.False(t, diags.HasError())
	assert.Equal(t, "urn:supercargo:hub:team:analytics-core", stateModel.URN.ValueString())
	assert.Equal(t, "analytics-core", stateModel.Name.ValueString())
}

func TestSupercargoTeamResource_Create_Unconfigured(t *testing.T) {
	ctx := context.Background()
	r := &supercargoTeamResource{}

	var teamSchemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

	planModel := supercargoTeamResourceModel{
		Name:      types.StringValue("analytics-core"),
		DataAsset: types.StringValue("go://github.com/supercargo-dev/core/analytics"),
		Members:   types.ListNull(types.StringType),
		Metadata:  types.MapNull(types.StringType),
	}

	req := resource.CreateRequest{
		Plan: newTestPlan(ctx, t, teamSchemaResp.Schema, planModel),
	}
	var resp resource.CreateResponse

	r.Create(ctx, req, &resp)
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
}

func TestSupercargoTeamResource_Create_HubError(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var teamSchemaResp resource.SchemaResponse
	r := &supercargoTeamResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.RegisterTeamHook = func(ctx context.Context, req *hubv1.RegisterTeamRequest) (*hubv1.RegisterTeamResponse, error) {
		return nil, status.Error(codes.AlreadyExists, "team name collision in catalog")
	}
	mockSrv.mu.Unlock()

	planModel := supercargoTeamResourceModel{
		Name:      types.StringValue("analytics-core"),
		DataAsset: types.StringValue("go://github.com/supercargo-dev/core/analytics"),
		Members:   types.ListNull(types.StringType),
		Metadata:  types.MapNull(types.StringType),
	}

	req := resource.CreateRequest{
		Plan: newTestPlan(ctx, t, teamSchemaResp.Schema, planModel),
	}
	var resp resource.CreateResponse

	r.Create(ctx, req, &resp)
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Creating Team")
	assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "team name collision in catalog")
}

func TestSupercargoTeamResource_Read_Success(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var teamSchemaResp resource.SchemaResponse
	r := &supercargoTeamResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
		assert.Equal(t, "data-platform", req.Name)
		return &hubv1.GetTeamResponse{
			Team: &platformv1.Team{
				Name:      "data-platform",
				Urn:       "urn:supercargo:hub:team:data-platform",
				DataAsset: "go://github.com/supercargo-dev/core/platform",
				Members:   []string{"platform-lead@example.com"},
				Metadata:  map[string]string{"env": "prod"},
			},
		}, nil
	}
	mockSrv.mu.Unlock()

	stateModel := supercargoTeamResourceModel{
		Name:      types.StringValue("data-platform"),
		URN:       types.StringValue("urn:supercargo:hub:team:data-platform"),
		DataAsset: types.StringValue("go://github.com/supercargo-dev/core/platform-old"),
		Members:   types.ListNull(types.StringType),
		Metadata:  types.MapNull(types.StringType),
	}

	req := resource.ReadRequest{
		State: newTestState(ctx, t, teamSchemaResp.Schema, stateModel),
	}
	var resp resource.ReadResponse
	resp.State = newTestState(ctx, t, teamSchemaResp.Schema, stateModel)

	r.Read(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "unexpected read errors: %v", resp.Diagnostics)

	var updatedState supercargoTeamResourceModel
	diags := resp.State.Get(ctx, &updatedState)
	require.False(t, diags.HasError())
	assert.Equal(t, "data-platform", updatedState.Name.ValueString())
	assert.Equal(t, "urn:supercargo:hub:team:data-platform", updatedState.URN.ValueString())
	assert.Equal(t, "go://github.com/supercargo-dev/core/platform", updatedState.DataAsset.ValueString())

	var members []string
	diags = updatedState.Members.ElementsAs(ctx, &members, false)
	require.False(t, diags.HasError())
	assert.Equal(t, []string{"platform-lead@example.com"}, members)

	var metadata map[string]string
	diags = updatedState.Metadata.ElementsAs(ctx, &metadata, false)
	require.False(t, diags.HasError())
	assert.Equal(t, map[string]string{"env": "prod"}, metadata)
}

func TestSupercargoTeamResource_Read_NotFound(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var teamSchemaResp resource.SchemaResponse
	r := &supercargoTeamResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
		return nil, status.Error(codes.NotFound, "team deleted")
	}
	mockSrv.mu.Unlock()

	stateModel := supercargoTeamResourceModel{
		Name:      types.StringValue("deleted-team"),
		URN:       types.StringValue("urn:supercargo:hub:team:deleted-team"),
		DataAsset: types.StringValue("go://github.com/supercargo-dev/core/deleted"),
		Members:   types.ListNull(types.StringType),
		Metadata:  types.MapNull(types.StringType),
	}

	req := resource.ReadRequest{
		State: newTestState(ctx, t, teamSchemaResp.Schema, stateModel),
	}
	var resp resource.ReadResponse
	resp.State = newTestState(ctx, t, teamSchemaResp.Schema, stateModel)

	r.Read(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError())
	assert.True(t, resp.State.Raw.IsNull(), "state should be null after NotFound removal")
}

func TestSupercargoTeamResource_Read_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoTeamResource{}
		var teamSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

		stateModel := supercargoTeamResourceModel{
			Name:      types.StringValue("team-x"),
			DataAsset: types.StringValue("go://github.com/supercargo-dev/core/x"),
			Members:   types.ListNull(types.StringType),
			Metadata:  types.MapNull(types.StringType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, teamSchemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse
		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("hub server error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoTeamResource{client: providerData}
		var teamSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return nil, status.Error(codes.Internal, "database disk failure")
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoTeamResourceModel{
			Name:      types.StringValue("team-x"),
			DataAsset: types.StringValue("go://github.com/supercargo-dev/core/x"),
			Members:   types.ListNull(types.StringType),
			Metadata:  types.MapNull(types.StringType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, teamSchemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse
		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Team")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "database disk failure")
	})

	t.Run("empty response from hub", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoTeamResource{client: providerData}
		var teamSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return &hubv1.GetTeamResponse{Team: nil}, nil
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoTeamResourceModel{
			Name:      types.StringValue("team-x"),
			DataAsset: types.StringValue("go://github.com/supercargo-dev/core/x"),
			Members:   types.ListNull(types.StringType),
			Metadata:  types.MapNull(types.StringType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, teamSchemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse
		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Team")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "Hub returned an empty response")
	})
}

func TestSupercargoTeamResource_Update_Success(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var teamSchemaResp resource.SchemaResponse
	r := &supercargoTeamResource{client: providerData}
	r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

	var updatedTeam *platformv1.Team
	mockSrv.mu.Lock()
	mockSrv.RegisterTeamHook = func(ctx context.Context, req *hubv1.RegisterTeamRequest) (*hubv1.RegisterTeamResponse, error) {
		updatedTeam = req.Team
		return &hubv1.RegisterTeamResponse{Team: req.Team}, nil
	}
	mockSrv.mu.Unlock()

	membersVal, diags := types.ListValueFrom(ctx, types.StringType, []string{"charlie@example.com"})
	require.False(t, diags.HasError())

	metaVal, diags := types.MapValueFrom(ctx, types.StringType, map[string]string{"tier": "platinum"})
	require.False(t, diags.HasError())

	planModel := supercargoTeamResourceModel{
		Name:      types.StringValue("analytics-core"),
		DataAsset: types.StringValue("go://github.com/supercargo-dev/core/analytics-v2"),
		Members:   membersVal,
		Metadata:  metaVal,
	}

	req := resource.UpdateRequest{
		Plan: newTestPlan(ctx, t, teamSchemaResp.Schema, planModel),
	}
	var resp resource.UpdateResponse
	resp.State = newTestState(ctx, t, teamSchemaResp.Schema, planModel)

	r.Update(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "unexpected update errors: %v", resp.Diagnostics)

	require.NotNil(t, updatedTeam)
	assert.Equal(t, "analytics-core", updatedTeam.Name)
	assert.Equal(t, "urn:supercargo:hub:team:analytics-core", updatedTeam.Urn)
	assert.Equal(t, "go://github.com/supercargo-dev/core/analytics-v2", updatedTeam.DataAsset)
	assert.Equal(t, []string{"charlie@example.com"}, updatedTeam.Members)
	assert.Equal(t, map[string]string{"tier": "platinum"}, updatedTeam.Metadata)

	var stateModel supercargoTeamResourceModel
	diags = resp.State.Get(ctx, &stateModel)
	require.False(t, diags.HasError())
	assert.Equal(t, "urn:supercargo:hub:team:analytics-core", stateModel.URN.ValueString())
}

func TestSupercargoTeamResource_Update_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoTeamResource{}
		var teamSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

		planModel := supercargoTeamResourceModel{
			Name:      types.StringValue("team-x"),
			DataAsset: types.StringValue("go://github.com/supercargo-dev/core/x"),
			Members:   types.ListNull(types.StringType),
			Metadata:  types.MapNull(types.StringType),
		}

		req := resource.UpdateRequest{
			Plan: newTestPlan(ctx, t, teamSchemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse
		r.Update(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("hub update error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoTeamResource{client: providerData}
		var teamSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

		mockSrv.mu.Lock()
		mockSrv.RegisterTeamHook = func(ctx context.Context, req *hubv1.RegisterTeamRequest) (*hubv1.RegisterTeamResponse, error) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized write")
		}
		mockSrv.mu.Unlock()

		planModel := supercargoTeamResourceModel{
			Name:      types.StringValue("team-x"),
			DataAsset: types.StringValue("go://github.com/supercargo-dev/core/x"),
			Members:   types.ListNull(types.StringType),
			Metadata:  types.MapNull(types.StringType),
		}

		req := resource.UpdateRequest{
			Plan: newTestPlan(ctx, t, teamSchemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse
		r.Update(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Updating Team")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "unauthorized write")
	})
}

func TestSupercargoTeamResource_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider", func(t *testing.T) {
		r := &supercargoTeamResource{}
		var teamSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

		stateModel := supercargoTeamResourceModel{
			Name:      types.StringValue("team-x"),
			DataAsset: types.StringValue("go://github.com/supercargo-dev/core/x"),
			Members:   types.ListNull(types.StringType),
			Metadata:  types.MapNull(types.StringType),
		}

		req := resource.DeleteRequest{
			State: newTestState(ctx, t, teamSchemaResp.Schema, stateModel),
		}
		var resp resource.DeleteResponse
		r.Delete(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("configured provider", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoTeamResource{client: providerData}
		var teamSchemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &teamSchemaResp)

		stateModel := supercargoTeamResourceModel{
			Name:      types.StringValue("team-x"),
			DataAsset: types.StringValue("go://github.com/supercargo-dev/core/x"),
			Members:   types.ListNull(types.StringType),
			Metadata:  types.MapNull(types.StringType),
		}

		req := resource.DeleteRequest{
			State: newTestState(ctx, t, teamSchemaResp.Schema, stateModel),
		}
		var resp resource.DeleteResponse
		r.Delete(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})
}
