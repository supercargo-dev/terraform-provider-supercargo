package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleValidManifestYAML = `meta:
  urn: urn:supercargo:hub:product:gateway-product
  version: v1.0.0
  owner:
    team_name: test-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:test-contract
      version: v1.0.0
`

const sampleUpdatedManifestYAML = `meta:
  urn: urn:supercargo:hub:product:gateway-product
  version: v2.0.0
  owner:
    team_name: test-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:test-contract
      version: v2.0.0
`

func TestSupercargoGatewayConfigResource_Metadata(t *testing.T) {
	r := NewSupercargoGatewayConfigResource()
	req := resource.MetadataRequest{ProviderTypeName: "supercargo"}
	var resp resource.MetadataResponse

	r.Metadata(context.Background(), req, &resp)
	assert.Equal(t, "supercargo_gateway_config", resp.TypeName)
}

func TestSupercargoGatewayConfigResource_Schema(t *testing.T) {
	r := NewSupercargoGatewayConfigResource()
	req := resource.SchemaRequest{}
	var resp resource.SchemaResponse

	r.Schema(context.Background(), req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "schema errors: %v", resp.Diagnostics)

	assert.NotNil(t, resp.Schema.Attributes["manifest_file"])
	assert.True(t, resp.Schema.Attributes["manifest_file"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["hub_address"])
	assert.True(t, resp.Schema.Attributes["hub_address"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["config_hash"])
	assert.True(t, resp.Schema.Attributes["config_hash"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["env_vars"])
	assert.True(t, resp.Schema.Attributes["env_vars"].IsComputed())
	assert.True(t, resp.Schema.Attributes["env_vars"].IsSensitive())
}

func TestSupercargoGatewayConfigResource_Configure(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider data", func(t *testing.T) {
		r := &supercargoGatewayConfigResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: nil}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Nil(t, r.client)
	})

	t.Run("invalid provider data type", func(t *testing.T) {
		r := &supercargoGatewayConfigResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: 12345}, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Unexpected Resource Configure Data Type")
		assert.Nil(t, r.client)
	})

	t.Run("valid provider data", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoGatewayConfigResource{}
		var resp resource.ConfigureResponse
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: providerData}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, providerData, r.client)
	})
}

func TestSupercargoGatewayConfigResource_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("success reads manifest and computes config_hash and env_vars", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoGatewayConfigResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "product.yaml")
		err := os.WriteFile(manifestPath, []byte(sampleValidManifestYAML), 0644)
		require.NoError(t, err)

		planModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue(manifestPath),
			HubAddress:   types.StringValue("https://hub.prod.example.com"),
			ConfigHash:   types.StringNull(),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, planModel)

		r.Create(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected create error: %v", resp.Diagnostics)

		var stateModel supercargoGatewayConfigResourceModel
		diags := resp.State.Get(ctx, &stateModel)
		require.False(t, diags.HasError())

		assert.NotEmpty(t, stateModel.ConfigHash.ValueString())
		assert.Equal(t, 64, len(stateModel.ConfigHash.ValueString())) // SHA-256 hex string

		var envVars map[string]string
		diags = stateModel.EnvVars.ElementsAs(ctx, &envVars, false)
		require.False(t, diags.HasError())
		assert.Equal(t, "https://hub.prod.example.com", envVars["HUB_ADDRESS"])
		assert.Equal(t, stateModel.ConfigHash.ValueString(), envVars["SUPERCARGO_CONFIG_HASH"])
		assert.Equal(t, manifestPath, envVars["SUPERCARGO_MANIFEST_PATH"])
	})

	t.Run("missing manifest file error", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoGatewayConfigResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue(filepath.Join(t.TempDir(), "non-existent.yaml")),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringNull(),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Reading Manifest")
	})

	t.Run("invalid YAML manifest error", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoGatewayConfigResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "invalid.yaml")
		err := os.WriteFile(manifestPath, []byte("invalid: yaml: :\n  - ::\n"), 0644)
		require.NoError(t, err)

		planModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue(manifestPath),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringNull(),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Parsing Manifest")
	})

	t.Run("unconfigured provider error", func(t *testing.T) {
		r := &supercargoGatewayConfigResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue("/path/to/manifest.yaml"),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringNull(),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.CreateResponse

		r.Create(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})
}

func TestSupercargoGatewayConfigResource_Read(t *testing.T) {
	ctx := context.Background()

	t.Run("success reads and updates config_hash", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoGatewayConfigResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "product.yaml")
		err := os.WriteFile(manifestPath, []byte(sampleValidManifestYAML), 0644)
		require.NoError(t, err)

		stateModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue(manifestPath),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringValue("old-hash"),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, stateModel)

		r.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected read error: %v", resp.Diagnostics)

		var updatedState supercargoGatewayConfigResourceModel
		diags := resp.State.Get(ctx, &updatedState)
		require.False(t, diags.HasError())
		assert.NotEmpty(t, updatedState.ConfigHash.ValueString())
		assert.NotEqual(t, "old-hash", updatedState.ConfigHash.ValueString())
	})

	t.Run("deleted manifest triggers RemoveResource", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoGatewayConfigResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		missingManifestPath := filepath.Join(t.TempDir(), "deleted-product.yaml")

		stateModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue(missingManifestPath),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringValue("some-hash"),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse
		resp.State = newTestState(ctx, t, schemaResp.Schema, stateModel)

		r.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError())
		assert.True(t, resp.State.Raw.IsNull(), "state should be null after file deletion")
	})

	t.Run("manifest parsing error on read", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoGatewayConfigResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "corrupted.yaml")
		err := os.WriteFile(manifestPath, []byte("corrupted: yaml: ::"), 0644)
		require.NoError(t, err)

		stateModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue(manifestPath),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringValue("some-hash"),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.ReadRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.ReadResponse

		r.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error Parsing Manifest")
	})

	t.Run("unconfigured provider error", func(t *testing.T) {
		r := &supercargoGatewayConfigResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue("/path/to/manifest.yaml"),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringValue("some-hash"),
			EnvVars:      types.MapNull(types.StringType),
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

func TestSupercargoGatewayConfigResource_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("success updates hash when manifest changes", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		r := &supercargoGatewayConfigResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		tmpDir := t.TempDir()
		manifestPath := filepath.Join(tmpDir, "product.yaml")
		err := os.WriteFile(manifestPath, []byte(sampleValidManifestYAML), 0644)
		require.NoError(t, err)

		initialPlan := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue(manifestPath),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringNull(),
			EnvVars:      types.MapNull(types.StringType),
		}

		createReq := resource.CreateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, initialPlan),
		}
		var createResp resource.CreateResponse
		createResp.State = newTestState(ctx, t, schemaResp.Schema, initialPlan)
		r.Create(ctx, createReq, &createResp)
		require.False(t, createResp.Diagnostics.HasError())

		var initialState supercargoGatewayConfigResourceModel
		diags := createResp.State.Get(ctx, &initialState)
		require.False(t, diags.HasError())
		initialHash := initialState.ConfigHash.ValueString()

		// Update manifest file on disk
		err = os.WriteFile(manifestPath, []byte(sampleUpdatedManifestYAML), 0644)
		require.NoError(t, err)

		updatePlan := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue(manifestPath),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringNull(),
			EnvVars:      types.MapNull(types.StringType),
		}

		updateReq := resource.UpdateRequest{
			Plan:  newTestPlan(ctx, t, schemaResp.Schema, updatePlan),
			State: createResp.State,
		}
		var updateResp resource.UpdateResponse
		updateResp.State = createResp.State

		r.Update(ctx, updateReq, &updateResp)
		require.False(t, updateResp.Diagnostics.HasError(), "unexpected update error: %v", updateResp.Diagnostics)

		var updatedState supercargoGatewayConfigResourceModel
		diags = updateResp.State.Get(ctx, &updatedState)
		require.False(t, diags.HasError())

		assert.NotEmpty(t, updatedState.ConfigHash.ValueString())
		assert.NotEqual(t, initialHash, updatedState.ConfigHash.ValueString())
	})

	t.Run("unconfigured provider error", func(t *testing.T) {
		r := &supercargoGatewayConfigResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		planModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue("/path/to/manifest.yaml"),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringNull(),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.UpdateRequest{
			Plan: newTestPlan(ctx, t, schemaResp.Schema, planModel),
		}
		var resp resource.UpdateResponse

		r.Update(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})
}

func TestSupercargoGatewayConfigResource_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("unconfigured provider error", func(t *testing.T) {
		r := &supercargoGatewayConfigResource{client: nil}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue("/path/to/manifest.yaml"),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringValue("hash"),
			EnvVars:      types.MapNull(types.StringType),
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
		r := &supercargoGatewayConfigResource{client: providerData}
		var schemaResp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

		stateModel := supercargoGatewayConfigResourceModel{
			ManifestFile: types.StringValue("/path/to/manifest.yaml"),
			HubAddress:   types.StringValue("https://hub.example.com"),
			ConfigHash:   types.StringValue("hash"),
			EnvVars:      types.MapNull(types.StringType),
		}

		req := resource.DeleteRequest{
			State: newTestState(ctx, t, schemaResp.Schema, stateModel),
		}
		var resp resource.DeleteResponse

		r.Delete(ctx, req, &resp)
		assert.False(t, resp.Diagnostics.HasError())
	})
}
