package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func writeTestDataProductManifest(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "dataproduct.yaml")
	err := os.WriteFile(p, []byte(content), 0644)
	require.NoError(t, err)
	return p
}

func newTestConfigModel(manifestFile, id, version, project string) supercargoDataProductDataSourceModel {
	m := supercargoDataProductDataSourceModel{
		ServiceIdentities:     types.MapNull(types.StringType),
		Contracts:             types.MapNull(contractObjectType),
		URN:                   types.StringNull(),
		Location:              types.StringNull(),
		PartitioningField:     types.StringNull(),
		PartitionExpirationMs: types.Int64Null(),
	}
	if manifestFile != "" {
		m.ManifestFile = types.StringValue(manifestFile)
	} else {
		m.ManifestFile = types.StringNull()
	}
	if id != "" {
		m.ID = types.StringValue(id)
	} else {
		m.ID = types.StringNull()
	}
	if version != "" {
		m.Version = types.StringValue(version)
	} else {
		m.Version = types.StringNull()
	}
	if project != "" {
		m.Project = types.StringValue(project)
	} else {
		m.Project = types.StringNull()
	}
	return m
}

const sampleDataProductManifestYAML = `meta:
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

const sampleMismatchProjectsDataProductYAML = `meta:
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

func TestSupercargoDataProductDataSource_Metadata(t *testing.T) {
	d := NewSupercargoDataProductDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "supercargo"}
	var resp datasource.MetadataResponse

	d.Metadata(context.Background(), req, &resp)
	assert.Equal(t, "supercargo_data_product", resp.TypeName)
}

func TestSupercargoDataProductDataSource_Schema(t *testing.T) {
	d := NewSupercargoDataProductDataSource()
	req := datasource.SchemaRequest{}
	var resp datasource.SchemaResponse

	d.Schema(context.Background(), req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "schema errors: %v", resp.Diagnostics)

	assert.NotNil(t, resp.Schema.Attributes["id"])
	assert.True(t, resp.Schema.Attributes["id"].IsOptional())
	assert.True(t, resp.Schema.Attributes["id"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["version"])
	assert.True(t, resp.Schema.Attributes["version"].IsOptional())
	assert.True(t, resp.Schema.Attributes["version"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["manifest_file"])
	assert.True(t, resp.Schema.Attributes["manifest_file"].IsOptional())

	assert.NotNil(t, resp.Schema.Attributes["project"])
	assert.True(t, resp.Schema.Attributes["project"].IsOptional())
	assert.True(t, resp.Schema.Attributes["project"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["urn"])
	assert.True(t, resp.Schema.Attributes["urn"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["location"])
	assert.True(t, resp.Schema.Attributes["location"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["partitioning_field"])
	assert.True(t, resp.Schema.Attributes["partitioning_field"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["partition_expiration_ms"])
	assert.True(t, resp.Schema.Attributes["partition_expiration_ms"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["service_identities"])
	assert.True(t, resp.Schema.Attributes["service_identities"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["contracts"])
	assert.True(t, resp.Schema.Attributes["contracts"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["sla"])
	assert.True(t, resp.Schema.Attributes["sla"].IsComputed())
}

func TestSupercargoDataProductDataSource_Configure(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider data", func(t *testing.T) {
		d := &supercargoDataProductDataSource{}
		var resp datasource.ConfigureResponse
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: nil}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})

	t.Run("invalid provider data type", func(t *testing.T) {
		d := &supercargoDataProductDataSource{}
		var resp datasource.ConfigureResponse
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: 12345}, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Unexpected Data Source Configure Data Type")
	})

	t.Run("valid provider data", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		d := &supercargoDataProductDataSource{}
		var resp datasource.ConfigureResponse
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})
}

func TestSupercargoDataProductDataSource_Validation(t *testing.T) {
	ctx := context.Background()

	t.Run("missing both id and manifest_file returns diagnostic error", func(t *testing.T) {
		d := NewSupercargoDataProductDataSource()
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel("", "", "", "")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Missing Required Attribute")
	})

	t.Run("invalid SLA tier in manifest returns diagnostic error", func(t *testing.T) {
		invalidSLAManifest := `meta:
  urn: urn:supercargo:hub:product:invalid-sla-product
  version: v1.0.0
  owner:
    team_name: orders-team
sla:
  tier: UNKNOWN_TIER_SUPER_DUPER
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
`
		manifestPath := writeTestDataProductManifest(t, invalidSLAManifest)

		d := NewSupercargoDataProductDataSource()
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel(manifestPath, "", "", "")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Invalid SLA Tier")
	})

	t.Run("inconsistent projects across output ports without override returns diagnostic error", func(t *testing.T) {
		manifestPath := writeTestDataProductManifest(t, sampleMismatchProjectsDataProductYAML)

		d := NewSupercargoDataProductDataSource()
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel(manifestPath, "", "", "")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Inconsistent GCP Projects in Manifest")
	})

	t.Run("inconsistent projects across output ports WITH project override succeeds", func(t *testing.T) {
		manifestPath := writeTestDataProductManifest(t, sampleMismatchProjectsDataProductYAML)

		d := NewSupercargoDataProductDataSource()
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel(manifestPath, "", "", "override-project")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected errors: %v", resp.Diagnostics)

		var state supercargoDataProductDataSourceModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
		require.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, "override-project", state.Project.ValueString())
	})
}

func TestSupercargoDataProductDataSource_Read_HubSuccess(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	productURN := "urn:supercargo:hub:product:hub-order-product"
	mockSrv.mu.Lock()
	mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
		assert.Equal(t, productURN, req.ProductUrn)
		return &hubv1.GetProductResponse{
			Manifest: &hubv1.ProductManifest{
				Meta: &hubv1.ProductMeta{
					Urn:     productURN,
					Version: "v2.0.0",
					Owner: &hubv1.Owner{
						TeamName: "orders-team",
					},
				},
				Sla: &hubv1.SLA{
					Tier:      hubv1.SLATier_SLA_TIER_1_MISSION_CRITICAL,
					Rto:       "4h",
					Freshness: "15m",
					Latency:   "30s",
				},
				OutputPorts: []*hubv1.OutputPort{
					{
						Name: "main",
						Physical: &hubv1.PhysicalConfig{
							Bigquery: &hubv1.BigQueryConfig{
								Project:             "hub-project-prod",
								Location:            "EU",
								PartitionBy:         "event_time",
								PartitionExpiration: durationpb.New(30 * 24 * time.Hour),
							},
						},
					},
				},
			},
			Contracts: []*hubv1.DataContract{
				{
					Meta: &hubv1.Meta{
						Urn:         "urn:supercargo:hub:contract:hub-contract-1",
						Version:     "v1.0.0",
						ContentHash: "hash123456",
						CommitSha:   "sha123456",
					},
					Schema: []*hubv1.Field{
						{Name: "order_id", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
					},
				},
			},
		}, nil
	}
	mockSrv.mu.Unlock()

	d := &supercargoDataProductDataSource{}
	d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

	cfgModel := newTestConfigModel("", productURN, "", "")

	req := datasource.ReadRequest{
		Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
	}
	var resp datasource.ReadResponse
	resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

	d.Read(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

	var state supercargoDataProductDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
	require.False(t, resp.Diagnostics.HasError())

	assert.Equal(t, productURN, state.URN.ValueString())
	assert.Equal(t, productURN, state.ID.ValueString())
	assert.Equal(t, "v2.0.0", state.Version.ValueString())
	assert.Equal(t, "hub-project-prod", state.Project.ValueString())
	assert.Equal(t, "EU", state.Location.ValueString())
	assert.Equal(t, "event_time", state.PartitioningField.ValueString())
	assert.Equal(t, int64(2592000000), state.PartitionExpirationMs.ValueInt64())

	// Validate service identities
	var identities map[string]string
	resp.Diagnostics.Append(state.ServiceIdentities.ElementsAs(ctx, &identities, false)...)
	require.False(t, resp.Diagnostics.HasError())
	assert.Equal(t, "serviceAccount:gateway-hub-order-product@hub-project-prod.iam.gserviceaccount.com", identities["gateway"])
	assert.Equal(t, "serviceAccount:shovel-hub-order-product@hub-project-prod.iam.gserviceaccount.com", identities["shovel"])

	// Validate SLA
	require.NotNil(t, state.SLA)
	assert.Equal(t, "SLA_TIER_1_MISSION_CRITICAL", state.SLA.Tier.ValueString())
	assert.Equal(t, "4h", state.SLA.Rto.ValueString())
	assert.Equal(t, "15m", state.SLA.Freshness.ValueString())
	assert.Equal(t, "30s", state.SLA.Latency.ValueString())

	// Validate contracts
	var contracts map[string]supercargoContractModel
	resp.Diagnostics.Append(state.Contracts.ElementsAs(ctx, &contracts, false)...)
	require.False(t, resp.Diagnostics.HasError())
	require.Contains(t, contracts, "urn:supercargo:hub:contract:hub-contract-1")
	assert.Equal(t, "urn:supercargo:hub:contract:hub-contract-1", contracts["urn:supercargo:hub:contract:hub-contract-1"].ID.ValueString())
	assert.Equal(t, "hash123456", contracts["urn:supercargo:hub:contract:hub-contract-1"].ContentHash.ValueString())
}

func TestSupercargoDataProductDataSource_Read_Fallback(t *testing.T) {
	ctx := context.Background()

	t.Run("Hub NotFound falls back to local manifest loading", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		mockSrv.mu.Unlock()

		manifestPath := writeTestDataProductManifest(t, sampleDataProductManifestYAML)

		// Create schemas dir and order-events bigquery json schema for local contract resolution
		schemasDir := filepath.Join(filepath.Dir(manifestPath), "schemas")
		require.NoError(t, os.MkdirAll(schemasDir, 0755))
		schemaPath := filepath.Join(schemasDir, "order-events.bigquery.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`[{"name":"event_id","type":"STRING"}]`), 0644))

		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})

		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel(manifestPath, "", "", "")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		var state supercargoDataProductDataSourceModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
		require.False(t, resp.Diagnostics.HasError())

		assert.Equal(t, "urn:supercargo:hub:product:order-events-product", state.URN.ValueString())
		assert.Equal(t, "gcp-orders-prod", state.Project.ValueString())
		assert.Equal(t, "US", state.Location.ValueString())
		assert.Equal(t, "event_timestamp", state.PartitioningField.ValueString())

		var contracts map[string]supercargoContractModel
		resp.Diagnostics.Append(state.Contracts.ElementsAs(ctx, &contracts, false)...)
		require.False(t, resp.Diagnostics.HasError())
		require.Contains(t, contracts, "urn:supercargo:hub:contract:order-events")
	})

	t.Run("Hub client nil falls back to local manifest loading", func(t *testing.T) {
		manifestPath := writeTestDataProductManifest(t, sampleDataProductManifestYAML)

		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: nil}, &datasource.ConfigureResponse{})

		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel(manifestPath, "", "", "")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		var state supercargoDataProductDataSourceModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &state)...)
		require.False(t, resp.Diagnostics.HasError())

		assert.Equal(t, "urn:supercargo:hub:product:order-events-product", state.URN.ValueString())
		assert.Equal(t, "gcp-orders-prod", state.Project.ValueString())
	})

	t.Run("Hub NotFound without manifest_file returns diagnostic error", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		mockSrv.mu.Unlock()

		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})

		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel("", "urn:supercargo:hub:product:not-found-product", "", "")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Data Product")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "Remediation Hint: Ensure 'sc apply' has been executed")
	})

	t.Run("Hub PermissionDenied does NOT fall back to local manifest and fails closed", func(t *testing.T) {
		manifestPath := writeTestDataProductManifest(t, sampleDataProductManifestYAML)

		mockSrv, providerData, _ := startMockHubServer(t)
		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized access to product")
		}
		mockSrv.mu.Unlock()

		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})

		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		// Provide both ID and manifest_file
		cfgModel := newTestConfigModel(manifestPath, "urn:supercargo:hub:product:order-events-product", "", "")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError(), "AuthZ failure must fail closed without falling back to local manifest")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Data Product")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "Verify that your identity token is valid and has permissions")
	})

	t.Run("Hub nil without manifest_file returns diagnostic error", func(t *testing.T) {
		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: nil}, &datasource.ConfigureResponse{})

		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel("", "urn:supercargo:hub:product:some-product", "", "")

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		resp.State = newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)

		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})
}
