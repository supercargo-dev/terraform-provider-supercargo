package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	platformv1 "github.com/supercargo-dev/core/gen/go/platform/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func writeTestManifest(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "product.yaml")
	err := os.WriteFile(p, []byte(content), 0644)
	require.NoError(t, err)
	return p
}

const sampleProductManifestYAML = `meta:
  urn: urn:supercargo:hub:product:order-events-product
  version: v1.0.0
  owner:
    team_name: orders-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:order-events
      version: v1.0.0
    physical:
      bigquery:
        project: gcp-orders-prod
        dataset: orders_ds
        location: US
        partition_by: event_timestamp
`

const sampleMismatchProjectsManifestYAML = `meta:
  urn: urn:supercargo:hub:product:multi-project-product
  version: v1.0.0
  owner:
    team_name: orders-team
output_ports:
  - name: port_alpha
    urn: urn:supercargo:hub:port:port_alpha
    contract:
      urn: urn:supercargo:hub:contract:order-events
      version: v1.0.0
    physical:
      bigquery:
        project: project-alpha
  - name: port_beta
    urn: urn:supercargo:hub:port:port_beta
    contract:
      urn: urn:supercargo:hub:contract:order-events
      version: v1.0.0
    physical:
      bigquery:
        project: project-beta
`

func TestSupercargoDataProductResource_Metadata(t *testing.T) {
	r := NewSupercargoDataProductResource()
	req := resource.MetadataRequest{ProviderTypeName: "supercargo"}
	var resp resource.MetadataResponse

	r.Metadata(context.Background(), req, &resp)
	assert.Equal(t, "supercargo_data_product", resp.TypeName)
}

func TestSupercargoDataProductResource_Schema(t *testing.T) {
	r := NewSupercargoDataProductResource()
	req := resource.SchemaRequest{}
	var resp resource.SchemaResponse

	r.Schema(context.Background(), req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "schema errors: %v", resp.Diagnostics)

	assert.NotNil(t, resp.Schema.Attributes["manifest_file"])
	assert.True(t, resp.Schema.Attributes["manifest_file"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["urn"])
	assert.True(t, resp.Schema.Attributes["urn"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["version"])
	assert.True(t, resp.Schema.Attributes["version"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["project"])
	assert.True(t, resp.Schema.Attributes["project"].IsOptional())
	assert.True(t, resp.Schema.Attributes["project"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["location"])
	assert.True(t, resp.Schema.Attributes["location"].IsOptional())
	assert.True(t, resp.Schema.Attributes["location"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["partitioning_field"])
	assert.True(t, resp.Schema.Attributes["partitioning_field"].IsOptional())
	assert.True(t, resp.Schema.Attributes["partitioning_field"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["partition_expiration_ms"])
	assert.True(t, resp.Schema.Attributes["partition_expiration_ms"].IsOptional())
	assert.True(t, resp.Schema.Attributes["partition_expiration_ms"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["service_identities"])
	assert.True(t, resp.Schema.Attributes["service_identities"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["sla"])
	assert.True(t, resp.Schema.Attributes["sla"].IsOptional())

	assert.NotNil(t, resp.Schema.Attributes["contracts"])
	assert.True(t, resp.Schema.Attributes["contracts"].IsComputed())
	contractsAttr, ok := resp.Schema.Attributes["contracts"].(schema.MapNestedAttribute)
	require.True(t, ok)
	assert.NotNil(t, contractsAttr.NestedObject.Attributes["id"])
	assert.True(t, contractsAttr.NestedObject.Attributes["id"].IsComputed())
	assert.NotNil(t, contractsAttr.NestedObject.Attributes["schema"])
	assert.True(t, contractsAttr.NestedObject.Attributes["schema"].IsComputed())
	assert.NotNil(t, contractsAttr.NestedObject.Attributes["version"])
	assert.True(t, contractsAttr.NestedObject.Attributes["version"].IsComputed())
	assert.NotNil(t, contractsAttr.NestedObject.Attributes["content_hash"])
	assert.True(t, contractsAttr.NestedObject.Attributes["content_hash"].IsComputed())
	assert.NotNil(t, contractsAttr.NestedObject.Attributes["commit_sha"])
	assert.True(t, contractsAttr.NestedObject.Attributes["commit_sha"].IsComputed())
}

func TestSupercargoDataProductResource_Configure(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider data", func(t *testing.T) {
		r := &supercargoDataProductResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: nil}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Nil(t, r.client)
	})

	t.Run("invalid provider data type", func(t *testing.T) {
		r := &supercargoDataProductResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: "invalid-provider-data"}, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Unexpected Resource Configure Data Type")
		assert.Nil(t, r.client)
	})

	t.Run("valid provider data", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: providerData}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, providerData, r.client)
	})
}

func TestSupercargoDataProductResource_ModifyPlan(t *testing.T) {
	ctx := context.Background()

	t.Run("null plan skipped early", func(t *testing.T) {
		r := &supercargoDataProductResource{}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		req := resource.ModifyPlanRequest{}
		req.Plan.Raw = tftypes.NewValue(tftypes.Object{}, nil)
		var resp resource.ModifyPlanResponse

		r.ModifyPlan(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})

	t.Run("missing manifest file error", func(t *testing.T) {
		r := &supercargoDataProductResource{}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(filepath.Join(t.TempDir(), "nonexistent.yaml")),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Loading Manifest")
	})

	t.Run("invalid YAML error", func(t *testing.T) {
		r := &supercargoDataProductResource{}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		manifestPath := writeTestManifest(t, "invalid: yaml: : ::")
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Loading Manifest")
	})

	t.Run("conflicting project IDs in manifest when HCL project is unset", func(t *testing.T) {
		r := &supercargoDataProductResource{}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		manifestPath := writeTestManifest(t, sampleMismatchProjectsManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Inconsistent GCP Projects in Manifest")
	})

	t.Run("conflicting project IDs in manifest resolved by HCL project override", func(t *testing.T) {
		r := &supercargoDataProductResource{}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		manifestPath := writeTestManifest(t, sampleMismatchProjectsManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringValue("override-gcp-project"),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		var plannedModel supercargoDataProductResourceModel
		diags := resp.Plan.Get(ctx, &plannedModel)
		require.False(t, diags.HasError())

		var identities map[string]string
		diags = plannedModel.ServiceIdentities.ElementsAs(ctx, &identities, false)
		require.False(t, diags.HasError())
		assert.Equal(t, "serviceAccount:gateway-multi-project-product@override-gcp-project.iam.gserviceaccount.com", identities["gateway"])
		assert.Equal(t, "serviceAccount:shovel-multi-project-product@override-gcp-project.iam.gserviceaccount.com", identities["shovel"])
	})

	t.Run("SLA tier validation - valid tiers", func(t *testing.T) {
		validTiers := []string{
			"GOLD", "SILVER", "BRONZE", "tier_1", "MISSION_CRITICAL",
			"SLA_TIER_1_MISSION_CRITICAL", "SLA_TIER_2_IMPORTANT", "SLA_TIER_3_BEST_EFFORT",
		}

		for _, tier := range validTiers {
			t.Run(tier, func(t *testing.T) {
				r := &supercargoDataProductResource{}
				var schemaResp resource.SchemaResponse
				r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

				manifestPath := writeTestManifest(t, sampleProductManifestYAML)
				planModel := supercargoDataProductResourceModel{
					ManifestFile:          types.StringValue(manifestPath),
					URN:                   types.StringNull(),
					Version:               types.StringNull(),
					Project:               types.StringNull(),
					Location:              types.StringNull(),
					PartitioningField:     types.StringNull(),
					PartitionExpirationMs: types.Int64Null(),
					ServiceIdentities:     types.MapNull(types.StringType),
					Contracts:             types.MapNull(contractObjectType),
					SLA: &supercargoSLAModel{
						Tier:      types.StringValue(tier),
						Rto:       types.StringNull(),
						Freshness: types.StringNull(),
						Latency:   types.StringNull(),
					},
				}

				plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
				req := resource.ModifyPlanRequest{
					Plan: plan,
				}
				var resp resource.ModifyPlanResponse
				resp.Plan = plan

				r.ModifyPlan(ctx, req, &resp)
				assert.False(t, resp.Diagnostics.HasError(), "tier '%s' should be valid, got errors: %v", tier, resp.Diagnostics)
			})
		}
	})

	t.Run("SLA tier validation - invalid tier", func(t *testing.T) {
		r := &supercargoDataProductResource{}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
			SLA: &supercargoSLAModel{
				Tier:      types.StringValue("UNKNOWN_TIER_SUPER_DUPER"),
				Rto:       types.StringNull(),
				Freshness: types.StringNull(),
				Latency:   types.StringNull(),
			},
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Invalid SLA Tier")
	})

	t.Run("owner team validation - team exists in Hub", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			assert.Equal(t, "orders-team", req.Name)
			return &hubv1.GetTeamResponse{
				Team: &platformv1.Team{Name: "orders-team"},
			}, nil
		}
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return &hubv1.GetContractResponse{
				Contract: &hubv1.DataContract{
					Meta: &hubv1.Meta{Urn: req.ContractUrn, Version: req.Version},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected errors: %v", resp.Diagnostics)
		assert.Empty(t, resp.Diagnostics.Warnings())
	})

	t.Run("owner team validation - team not found triggers warning", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return nil, status.Error(codes.NotFound, "team not found")
		}
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return &hubv1.GetContractResponse{
				Contract: &hubv1.DataContract{
					Meta: &hubv1.Meta{Urn: req.ContractUrn, Version: req.Version},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "bootstrap mode should not produce error: %v", resp.Diagnostics)
		require.NotEmpty(t, resp.Diagnostics.Warnings())
		assert.Contains(t, resp.Diagnostics.Warnings()[0].Summary(), "Team Not Found (Bootstrap Mode)")
	})

	t.Run("owner team validation - hub internal error returns error diagnostic", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return nil, status.Error(codes.Internal, "database connection failed")
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Governance Handshake Failed")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "database connection failed")
	})

	t.Run("output port contract validation - governing contract missing in Hub", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return &hubv1.GetTeamResponse{Team: &platformv1.Team{Name: req.Name}}, nil
		}
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return nil, status.Error(codes.NotFound, "contract not registered")
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Governing Contract Not Found")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "urn:supercargo:hub:contract:order-events")
	})

	t.Run("output port contract validation - contract hub server error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return &hubv1.GetTeamResponse{Team: &platformv1.Team{Name: req.Name}}, nil
		}
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized contract read")
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Governance Handshake Failed")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "unauthorized contract read")
	})

	t.Run("partitioning field validation against contract schema", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return &hubv1.GetTeamResponse{Team: &platformv1.Team{Name: req.Name}}, nil
		}
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return &hubv1.GetContractResponse{
				Contract: &hubv1.DataContract{
					Meta: &hubv1.Meta{Urn: req.ContractUrn, Version: req.Version},
					Schema: []*hubv1.Field{
						{Name: "order_id", Type: hubv1.DataType_DATA_TYPE_STRING},
						{Name: "created_at", Type: hubv1.DataType_DATA_TYPE_TIMESTAMP},
					},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)

		t.Run("valid field found in contract schema", func(t *testing.T) {
			planModel := supercargoDataProductResourceModel{
				ManifestFile:          types.StringValue(manifestPath),
				URN:                   types.StringNull(),
				Version:               types.StringNull(),
				Project:               types.StringNull(),
				Location:              types.StringNull(),
				PartitioningField:     types.StringValue("created_at"),
				PartitionExpirationMs: types.Int64Null(),
				ServiceIdentities:     types.MapNull(types.StringType),
				Contracts:             types.MapNull(contractObjectType),
			}

			plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
			req := resource.ModifyPlanRequest{
				Plan: plan,
			}
			var resp resource.ModifyPlanResponse
			resp.Plan = plan

			r.ModifyPlan(ctx, req, &resp)
			require.False(t, resp.Diagnostics.HasError(), "unexpected errors: %v", resp.Diagnostics)
		})

		t.Run("invalid field not found in contract schema", func(t *testing.T) {
			planModel := supercargoDataProductResourceModel{
				ManifestFile:          types.StringValue(manifestPath),
				URN:                   types.StringNull(),
				Version:               types.StringNull(),
				Project:               types.StringNull(),
				Location:              types.StringNull(),
				PartitioningField:     types.StringValue("non_existent_timestamp"),
				PartitionExpirationMs: types.Int64Null(),
				ServiceIdentities:     types.MapNull(types.StringType),
				Contracts:             types.MapNull(contractObjectType),
			}

			plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
			req := resource.ModifyPlanRequest{
				Plan: plan,
			}
			var resp resource.ModifyPlanResponse
			resp.Plan = plan

			r.ModifyPlan(ctx, req, &resp)
			require.True(t, resp.Diagnostics.HasError())
			assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Invalid Partitioning Field")
			assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "non_existent_timestamp")
		})
	})
}

func TestSupercargoDataProductResource_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("success product registration in Hub with overrides", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		var registeredManifest *hubv1.ProductManifest
		mockSrv.mu.Lock()
		mockSrv.RegisterProductHook = func(ctx context.Context, req *hubv1.RegisterProductRequest) (*hubv1.RegisterProductResponse, error) {
			registeredManifest = req.Manifest
			return &hubv1.RegisterProductResponse{
				ProductUrn: "urn:supercargo:hub:product:order-events-product",
				Version:    "v1.0.0",
			}, nil
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringValue("custom-gcp-project"),
			Location:              types.StringValue("EU"),
			PartitioningField:     types.StringValue("order_time"),
			PartitionExpirationMs: types.Int64Value(86400000), // 1 day
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
			SLA: &supercargoSLAModel{
				Tier:      types.StringValue("GOLD"),
				Rto:       types.StringValue("2h"),
				Freshness: types.StringValue("10m"),
				Latency:   types.StringValue("1s"),
			},
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, planModel)

		r.Create(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected create errors: %v", resp.Diagnostics)

		require.NotNil(t, registeredManifest)
		require.Len(t, registeredManifest.OutputPorts, 1)
		bq := registeredManifest.OutputPorts[0].Physical.Bigquery
		assert.Equal(t, "custom-gcp-project", bq.Project)
		assert.Equal(t, "EU", bq.Location)
		assert.Equal(t, "order_time", bq.PartitionBy)
		assert.Equal(t, int64(86400000), bq.PartitionExpiration.AsDuration().Milliseconds())

		require.NotNil(t, registeredManifest.Sla)
		assert.Equal(t, hubv1.SLATier_SLA_TIER_1_MISSION_CRITICAL, registeredManifest.Sla.Tier)
		assert.Equal(t, "2h", registeredManifest.Sla.Rto)
		assert.Equal(t, "10m", registeredManifest.Sla.Freshness)
		assert.Equal(t, "1s", registeredManifest.Sla.Latency)

		var stateModel supercargoDataProductResourceModel
		diags := resp.State.Get(ctx, &stateModel)
		require.False(t, diags.HasError())
		assert.Equal(t, "urn:supercargo:hub:product:order-events-product", stateModel.URN.ValueString())
		assert.Equal(t, "v1.0.0", stateModel.Version.ValueString())
		assert.Equal(t, "custom-gcp-project", stateModel.Project.ValueString())
		assert.Equal(t, "EU", stateModel.Location.ValueString())

		var identities map[string]string
		diags = stateModel.ServiceIdentities.ElementsAs(ctx, &identities, false)
		require.False(t, diags.HasError())
		assert.Equal(t, "serviceAccount:gateway-order-events-product@custom-gcp-project.iam.gserviceaccount.com", identities["gateway"])
		assert.Equal(t, "serviceAccount:shovel-order-events-product@custom-gcp-project.iam.gserviceaccount.com", identities["shovel"])
	})

	t.Run("provider not configured error", func(t *testing.T) {
		r := &supercargoDataProductResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("missing manifest file error", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(filepath.Join(t.TempDir(), "not-found.yaml")),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Loading Manifest")
	})

	t.Run("hub RegisterProduct gRPC error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.RegisterProductHook = func(ctx context.Context, req *hubv1.RegisterProductRequest) (*hubv1.RegisterProductResponse, error) {
			return nil, status.Error(codes.AlreadyExists, "product URN and version collision")
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Registering Product")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "product URN and version collision")
	})
}

func TestSupercargoDataProductResource_Read(t *testing.T) {
	ctx := context.Background()

	t.Run("success read and state refresh", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			assert.Equal(t, "urn:supercargo:hub:product:order-events-product", req.ProductUrn)
			assert.Equal(t, "v1.0.0", req.Version)
			return &hubv1.GetProductResponse{
				Manifest: &hubv1.ProductManifest{
					Meta: &hubv1.ProductMeta{
						Urn:     "urn:supercargo:hub:product:order-events-product",
						Version: "v1.0.0",
					},
					OutputPorts: []*hubv1.OutputPort{
						{
							Name: "main",
							Physical: &hubv1.PhysicalConfig{
								Bigquery: &hubv1.BigQueryConfig{
									Project:             "gcp-orders-prod",
									Location:            "US",
									PartitionBy:         "event_timestamp",
									PartitionExpiration: durationpb.New(48 * time.Hour),
								},
							},
						},
					},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue("/dummy/path.yaml"),
			URN:                   types.StringValue("urn:supercargo:hub:product:order-events-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, stateModel)

		r.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected read error: %v", resp.Diagnostics)

		var updatedState supercargoDataProductResourceModel
		diags := resp.State.Get(ctx, &updatedState)
		require.False(t, diags.HasError())
		assert.Equal(t, "gcp-orders-prod", updatedState.Project.ValueString())
		assert.Equal(t, "US", updatedState.Location.ValueString())
		assert.Equal(t, "event_timestamp", updatedState.PartitioningField.ValueString())
		assert.Equal(t, int64(172800000), updatedState.PartitionExpirationMs.ValueInt64())

		var identities map[string]string
		diags = updatedState.ServiceIdentities.ElementsAs(ctx, &identities, false)
		require.False(t, diags.HasError())
		assert.Equal(t, "serviceAccount:gateway-order-events-product@gcp-orders-prod.iam.gserviceaccount.com", identities["gateway"])
		assert.Equal(t, "serviceAccount:shovel-order-events-product@gcp-orders-prod.iam.gserviceaccount.com", identities["shovel"])
	})

	t.Run("codes.NotFound removes resource from state", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue("/dummy/path.yaml"),
			URN:                   types.StringValue("urn:supercargo:hub:product:deleted-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
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

	t.Run("server error diagnostics", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return nil, status.Error(codes.Internal, "database timeout")
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue("/dummy/path.yaml"),
			URN:                   types.StringValue("urn:supercargo:hub:product:order-events-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse

		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Product")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "database timeout")
	})

	t.Run("empty response from hub", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return &hubv1.GetProductResponse{Manifest: nil}, nil
		}
		mockSrv.mu.Unlock()

		stateModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue("/dummy/path.yaml"),
			URN:                   types.StringValue("urn:supercargo:hub:product:order-events-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse

		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Invalid Product Manifest")
	})

	t.Run("provider not configured error", func(t *testing.T) {
		r := &supercargoDataProductResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue("/dummy/path.yaml"),
			URN:                   types.StringValue("urn:supercargo:hub:product:order-events-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
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

func TestSupercargoDataProductResource_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("successful update calling RegisterProduct", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		called := false
		mockSrv.mu.Lock()
		mockSrv.RegisterProductHook = func(ctx context.Context, req *hubv1.RegisterProductRequest) (*hubv1.RegisterProductResponse, error) {
			called = true
			return &hubv1.RegisterProductResponse{
				ProductUrn: req.Manifest.Meta.Urn,
				Version:    req.Manifest.Meta.Version,
			}, nil
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringValue("urn:supercargo:hub:product:order-events-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringValue("gcp-orders-prod"),
			Location:              types.StringValue("US"),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.UpdateRequest{
			Plan:  newTestPlan(ctx, t, schemaResp.Schema, planModel),
			State: newTestState(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, planModel)

		r.Update(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected update errors: %v", resp.Diagnostics)
		assert.True(t, called)
	})

	t.Run("provider not configured error", func(t *testing.T) {
		r := &supercargoDataProductResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)
		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringValue("urn:supercargo:hub:product:order-events-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.UpdateRequest{
			Plan:  newTestPlan(ctx, t, schemaResp.Schema, planModel),
			State: newTestState(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse

		r.Update(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})
}

func TestSupercargoDataProductResource_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider error", func(t *testing.T) {
		r := &supercargoDataProductResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue("/path/to/manifest.yaml"),
			URN:                   types.StringValue("urn:supercargo:hub:product:order-events-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
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
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue("/path/to/manifest.yaml"),
			URN:                   types.StringValue("urn:supercargo:hub:product:order-events-product"),
			Version:               types.StringValue("v1.0.0"),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.DeleteRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.DeleteResponse

		r.Delete(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})
}

func TestSupercargoDataProductResource_NilPortDefensiveness(t *testing.T) {
	r := &supercargoDataProductResource{}

	// Test ensureBqConfig with nil
	assert.Nil(t, ensureBqConfig(nil))

	// Test durationToMs with nil
	assert.True(t, durationToMs(nil).IsNull())
	assert.True(t, durationToMs(&hubv1.PhysicalConfig{}).IsNull())

	// Test applyOverrides with nil port element
	manifestWithNilPort := &hubv1.ProductManifest{
		OutputPorts: []*hubv1.OutputPort{nil},
	}
	planModel := &supercargoDataProductResourceModel{
		Project:               types.StringValue("p1"),
		Location:              types.StringValue("EU"),
		PartitioningField:     types.StringValue("ts"),
		PartitionExpirationMs: types.Int64Value(3600000),
		SLA: &supercargoSLAModel{
			Tier:      types.StringValue("GOLD"),
			Rto:       types.StringValue("1h"),
			Freshness: types.StringValue("5m"),
			Latency:   types.StringValue("100ms"),
		},
	}
	assert.NotPanics(t, func() {
		r.applyOverrides(manifestWithNilPort, planModel)
	})
	assert.Equal(t, hubv1.SLATier_SLA_TIER_1_MISSION_CRITICAL, manifestWithNilPort.Sla.Tier)
}

func TestSupercargoDataProductResource_ModifyPlan_Contracts(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves local contracts and populates plan.Contracts and checks CheckDownstreamImpact", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "product.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		schemasDir := filepath.Join(dir, "schemas")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))
		require.NoError(t, os.MkdirAll(schemasDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:order-events-product
  version: v1.0.0
  owner:
    team_name: orders-team
output_ports:
  - name: page_views
    urn: urn:supercargo:hub:port:page-views
    contract:
      urn: urn:supercargo:hub:contract:page-views
      version: "1.0.0"
    physical:
      bigquery:
        project: gcp-orders-prod
        dataset: orders_ds
        location: US
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:page-views
  version: "1.0.0"
  data_asset: go://github.com/supercargo-dev/core/examples/data-producer
  commit_sha: commit-sha-xyz123
schema:
  - name: user_id
    description: The user ID
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REQUIRED
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "page_views.yaml"), []byte(contractYAML), 0644))

		schemaJSON := `[{"name":"user_id","type":"STRING","mode":"REQUIRED","description":"The user ID"}]`
		require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "page_views.1.0.0.bigquery.json"), []byte(schemaJSON), 0644))

		impactChecked := false
		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return &hubv1.GetTeamResponse{Team: &platformv1.Team{Name: req.Name}}, nil
		}
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			impactChecked = true
			assert.Equal(t, "urn:supercargo:hub:contract:page-views", req.Contract.Meta.Urn)
			assert.Equal(t, "1.0.0", req.Contract.Meta.Version)
			return &hubv1.CheckDownstreamImpactResponse{
				Severity: hubv1.ImpactSeverity_IMPACT_SEVERITY_NONE,
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected errors: %v", resp.Diagnostics)
		assert.True(t, impactChecked)

		var plannedModel supercargoDataProductResourceModel
		diags := resp.Plan.Get(ctx, &plannedModel)
		require.False(t, diags.HasError())
		assert.False(t, plannedModel.Contracts.IsNull())
		assert.False(t, plannedModel.Contracts.IsUnknown())

		var contracts map[string]supercargoContractModel
		diags = plannedModel.Contracts.ElementsAs(ctx, &contracts, false)
		require.False(t, diags.HasError())
		require.Len(t, contracts, 1)

		c, ok := contracts["urn:supercargo:hub:contract:page-views"]
		require.True(t, ok)
		assert.Equal(t, "urn:supercargo:hub:contract:page-views", c.ID.ValueString())
		assert.Equal(t, "1.0.0", c.Version.ValueString())
		assert.Equal(t, schemaJSON, c.Schema.ValueString())
		assert.Equal(t, "commit-sha-xyz123", c.CommitSha.ValueString())
		assert.NotEmpty(t, c.ContentHash.ValueString())
	})

	t.Run("resolves input port contracts and populates plan.Contracts", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "product.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:rockify-streaming
  version: v1.0.0
  owner:
    team_name: rockify-team
input_ports:
  - name: streaming-orders
    source: pubsub://projects/rockify-data/topics/rockify-orders
    contract:
      urn: urn:supercargo:hub:contract:rockify-orders
      version: "1.0.0"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:rockify-orders
  version: "1.0.0"
schema:
  - name: order_id
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REQUIRED
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "rockify-orders.1.0.0.contract.yaml"), []byte(contractYAML), 0644))

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return &hubv1.GetTeamResponse{Team: &platformv1.Team{Name: req.Name}}, nil
		}
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return &hubv1.CheckDownstreamImpactResponse{
				Severity: hubv1.ImpactSeverity_IMPACT_SEVERITY_NONE,
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.ModifyPlanRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = newTestPlan(ctx, t, schemaResp.Schema, planModel)

		r.ModifyPlan(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		var plannedModel supercargoDataProductResourceModel
		diags := resp.Plan.Get(ctx, &plannedModel)
		require.False(t, diags.HasError())
		assert.False(t, plannedModel.Contracts.IsNull())

		var contracts map[string]supercargoContractModel
		diags = plannedModel.Contracts.ElementsAs(ctx, &contracts, false)
		require.False(t, diags.HasError())
		require.Len(t, contracts, 1)

		c, ok := contracts["urn:supercargo:hub:contract:rockify-orders"]
		require.True(t, ok)
		assert.Equal(t, "urn:supercargo:hub:contract:rockify-orders", c.ID.ValueString())
		assert.Equal(t, "1.0.0", c.Version.ValueString())
		assert.NotEmpty(t, c.Schema.ValueString())
	})

	t.Run("breaking change detected in CheckDownstreamImpact adds error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "product.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:order-events-product
  version: v1.0.0
  owner:
    team_name: orders-team
output_ports:
  - name: page_views
    contract:
      urn: urn:supercargo:hub:contract:page-views
      version: "1.0.0"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:page-views
  version: "1.0.0"
schema:
  - name: user_id
    type: DATA_TYPE_STRING
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "page_views.yaml"), []byte(contractYAML), 0644))

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return &hubv1.GetTeamResponse{Team: &platformv1.Team{Name: req.Name}}, nil
		}
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return &hubv1.CheckDownstreamImpactResponse{
				Severity:        hubv1.ImpactSeverity_IMPACT_SEVERITY_BREAKING,
				BreakingChanges: []string{"Removed required field 'order_id'"},
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Breaking Change Detected")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "Removed required field 'order_id'")
	})

	t.Run("polyrepo decoupled fallback retrieves contract from Hub when local files absent", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)

		mockSrv.mu.Lock()
		mockSrv.GetTeamHook = func(ctx context.Context, req *hubv1.GetTeamRequest) (*hubv1.GetTeamResponse, error) {
			return &hubv1.GetTeamResponse{Team: &platformv1.Team{Name: req.Name}}, nil
		}
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			assert.Equal(t, "urn:supercargo:hub:contract:order-events", req.ContractUrn)
			assert.Equal(t, "v1.0.0", req.Version)
			return &hubv1.GetContractResponse{
				Contract: &hubv1.DataContract{
					Meta: &hubv1.Meta{
						Urn:         "urn:supercargo:hub:contract:order-events",
						Version:     "v1.0.0",
						ContentHash: "remote-hash-123",
						CommitSha:   "remote-sha-456",
					},
					Schema: []*hubv1.Field{
						{Name: "event_timestamp", Type: hubv1.DataType_DATA_TYPE_TIMESTAMP, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
					},
				},
			}, nil
		}
		mockSrv.CheckDownstreamImpactHook = func(ctx context.Context, req *hubv1.CheckDownstreamImpactRequest) (*hubv1.CheckDownstreamImpactResponse, error) {
			return &hubv1.CheckDownstreamImpactResponse{
				Severity: hubv1.ImpactSeverity_IMPACT_SEVERITY_NONE,
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringValue("event_timestamp"),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		plan := newTestPlan(ctx, t, schemaResp.Schema, planModel)
		req := resource.ModifyPlanRequest{
			Plan: plan,
		}
		var resp resource.ModifyPlanResponse
		resp.Plan = plan

		r.ModifyPlan(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected errors: %v", resp.Diagnostics)

		var plannedModel supercargoDataProductResourceModel
		diags := resp.Plan.Get(ctx, &plannedModel)
		require.False(t, diags.HasError())
		assert.False(t, plannedModel.Contracts.IsNull())

		var contracts map[string]supercargoContractModel
		diags = plannedModel.Contracts.ElementsAs(ctx, &contracts, false)
		require.False(t, diags.HasError())
		require.Len(t, contracts, 1)

		c, ok := contracts["urn:supercargo:hub:contract:order-events"]
		require.True(t, ok)
		assert.Equal(t, "urn:supercargo:hub:contract:order-events", c.ID.ValueString())
		assert.Equal(t, "v1.0.0", c.Version.ValueString())
		assert.Equal(t, "remote-hash-123", c.ContentHash.ValueString())
		assert.Equal(t, "remote-sha-456", c.CommitSha.ValueString())
		assert.Contains(t, c.Schema.ValueString(), "event_timestamp")
	})
}

func TestSupercargoDataProductResource_Create_Contracts(t *testing.T) {
	ctx := context.Background()

	t.Run("registers resolved contracts with Hub before RegisterProduct", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "product.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:order-events-product
  version: v1.0.0
  owner:
    team_name: orders-team
output_ports:
  - name: page_views
    contract:
      urn: urn:supercargo:hub:contract:page-views
      version: "1.0.0"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:page-views
  version: "1.0.0"
schema:
  - name: user_id
    type: DATA_TYPE_STRING
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "page_views.yaml"), []byte(contractYAML), 0644))

		var callOrder []string
		mockSrv.mu.Lock()
		mockSrv.RegisterContractHook = func(ctx context.Context, req *hubv1.RegisterContractRequest) (*hubv1.RegisterContractResponse, error) {
			callOrder = append(callOrder, "RegisterContract")
			assert.Equal(t, "urn:supercargo:hub:contract:page-views", req.Contract.Meta.Urn)
			assert.Equal(t, "1.0.0", req.Contract.Meta.Version)
			return &hubv1.RegisterContractResponse{Contract: req.Contract}, nil
		}
		mockSrv.RegisterProductHook = func(ctx context.Context, req *hubv1.RegisterProductRequest) (*hubv1.RegisterProductResponse, error) {
			callOrder = append(callOrder, "RegisterProduct")
			return &hubv1.RegisterProductResponse{
				ProductUrn: req.Manifest.Meta.Urn,
				Version:    req.Manifest.Meta.Version,
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, planModel)

		r.Create(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected create errors: %v", resp.Diagnostics)
		assert.Equal(t, []string{"RegisterContract", "RegisterProduct"}, callOrder)

		var stateModel supercargoDataProductResourceModel
		diags := resp.State.Get(ctx, &stateModel)
		require.False(t, diags.HasError())
		assert.False(t, stateModel.Contracts.IsNull())

		var contracts map[string]supercargoContractModel
		diags = stateModel.Contracts.ElementsAs(ctx, &contracts, false)
		require.False(t, diags.HasError())
		require.Len(t, contracts, 1)
		assert.Equal(t, "urn:supercargo:hub:contract:page-views", contracts["urn:supercargo:hub:contract:page-views"].ID.ValueString())
	})

	t.Run("polyrepo fallback registers contract retrieved from Hub", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		r := &supercargoDataProductResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		manifestPath := writeTestManifest(t, sampleProductManifestYAML)

		var registeredContract *hubv1.DataContract
		mockSrv.mu.Lock()
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return &hubv1.GetContractResponse{
				Contract: &hubv1.DataContract{
					Meta: &hubv1.Meta{
						Urn:         req.ContractUrn,
						Version:     req.Version,
						ContentHash: "hash-remote-123",
					},
					Schema: []*hubv1.Field{
						{Name: "order_id", Type: hubv1.DataType_DATA_TYPE_STRING},
					},
				},
			}, nil
		}
		mockSrv.RegisterContractHook = func(ctx context.Context, req *hubv1.RegisterContractRequest) (*hubv1.RegisterContractResponse, error) {
			registeredContract = req.Contract
			return &hubv1.RegisterContractResponse{Contract: req.Contract}, nil
		}
		mockSrv.RegisterProductHook = func(ctx context.Context, req *hubv1.RegisterProductRequest) (*hubv1.RegisterProductResponse, error) {
			return &hubv1.RegisterProductResponse{
				ProductUrn: req.Manifest.Meta.Urn,
				Version:    req.Manifest.Meta.Version,
			}, nil
		}
		mockSrv.mu.Unlock()

		planModel := supercargoDataProductResourceModel{
			ManifestFile:          types.StringValue(manifestPath),
			URN:                   types.StringNull(),
			Version:               types.StringNull(),
			Project:               types.StringNull(),
			Location:              types.StringNull(),
			PartitioningField:     types.StringNull(),
			PartitionExpirationMs: types.Int64Null(),
			ServiceIdentities:     types.MapNull(types.StringType),
			Contracts:             types.MapNull(contractObjectType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, planModel)

		r.Create(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected create errors: %v", resp.Diagnostics)
		require.NotNil(t, registeredContract)
		assert.Equal(t, "urn:supercargo:hub:contract:order-events", registeredContract.Meta.Urn)

		var stateModel supercargoDataProductResourceModel
		diags := resp.State.Get(ctx, &stateModel)
		require.False(t, diags.HasError())
		assert.False(t, stateModel.Contracts.IsNull())
	})
}
