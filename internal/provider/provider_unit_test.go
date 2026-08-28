package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/hub"
)

func newTestProviderConfig(ctx context.Context, t *testing.T, s pschema.Schema, model any) tfsdk.Config {
	t.Helper()
	state := tfsdk.State{
		Schema: s,
	}
	diags := state.Set(ctx, model)
	require.False(t, diags.HasError(), "diags setting test provider config: %v", diags)
	return tfsdk.Config{
		Schema: s,
		Raw:    state.Raw,
	}
}

func TestSupercargoProvider_Metadata(t *testing.T) {
	p := New("1.0.0")()
	var resp provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &resp)

	assert.Equal(t, "supercargo", resp.TypeName)
	assert.Equal(t, "1.0.0", resp.Version)
}

func TestSupercargoProvider_Schema(t *testing.T) {
	p := New("dev")()
	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)

	require.False(t, resp.Diagnostics.HasError(), "schema errors: %v", resp.Diagnostics)
	assert.NotNil(t, resp.Schema.Attributes["hub_address"])
	assert.True(t, resp.Schema.Attributes["hub_address"].IsOptional())

	assert.NotNil(t, resp.Schema.Attributes["token"])
	assert.True(t, resp.Schema.Attributes["token"].IsOptional())
	assert.True(t, resp.Schema.Attributes["token"].IsSensitive())

	assert.NotNil(t, resp.Schema.Attributes["audience"])
	assert.True(t, resp.Schema.Attributes["audience"].IsOptional())
}

func TestSupercargoProvider_Configure(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown hub_address sets unknown provider data and nil client", func(t *testing.T) {
		p := New("dev")()
		var schemaResp provider.SchemaResponse
		p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

		configModel := SupercargoProviderModel{
			HubAddress: types.StringUnknown(),
			Token:      types.StringValue("some-token"),
			Audience:   types.StringNull(),
		}

		req := provider.ConfigureRequest{
			Config: newTestProviderConfig(ctx, t, schemaResp.Schema, configModel),
		}
		var resp provider.ConfigureResponse

		p.Configure(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		require.NotNil(t, resp.ResourceData)
		data, ok := resp.ResourceData.(*ProviderData)
		require.True(t, ok)
		assert.Equal(t, "unknown", data.HubAddress)
		assert.Nil(t, data.HubClient)
		assert.NotNil(t, data.Cache)

		require.NotNil(t, resp.DataSourceData)
		dsData, ok := resp.DataSourceData.(*ProviderData)
		require.True(t, ok)
		assert.Equal(t, "unknown", dsData.HubAddress)
		assert.Nil(t, dsData.HubClient)
	})

	t.Run("unknown token sets unknown provider data and nil client", func(t *testing.T) {
		p := New("dev")()
		var schemaResp provider.SchemaResponse
		p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

		configModel := SupercargoProviderModel{
			HubAddress: types.StringValue("localhost:50051"),
			Token:      types.StringUnknown(),
			Audience:   types.StringNull(),
		}

		req := provider.ConfigureRequest{
			Config: newTestProviderConfig(ctx, t, schemaResp.Schema, configModel),
		}
		var resp provider.ConfigureResponse

		p.Configure(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		require.NotNil(t, resp.ResourceData)
		data, ok := resp.ResourceData.(*ProviderData)
		require.True(t, ok)
		assert.Equal(t, "unknown", data.HubAddress)
		assert.Nil(t, data.HubClient)
	})

	t.Run("unknown audience sets unknown provider data and nil client", func(t *testing.T) {
		p := New("dev")()
		var schemaResp provider.SchemaResponse
		p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

		configModel := SupercargoProviderModel{
			HubAddress: types.StringValue("localhost:50051"),
			Token:      types.StringValue("some-token"),
			Audience:   types.StringUnknown(),
		}

		req := provider.ConfigureRequest{
			Config: newTestProviderConfig(ctx, t, schemaResp.Schema, configModel),
		}
		var resp provider.ConfigureResponse

		p.Configure(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		require.NotNil(t, resp.ResourceData)
		data, ok := resp.ResourceData.(*ProviderData)
		require.True(t, ok)
		assert.Equal(t, "unknown", data.HubAddress)
		assert.Nil(t, data.HubClient)
	})

	t.Run("default hub_address fallback to localhost:50051", func(t *testing.T) {
		p := New("dev")()
		var schemaResp provider.SchemaResponse
		p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

		configModel := SupercargoProviderModel{
			HubAddress: types.StringNull(),
			Token:      types.StringValue("mock-local-token"),
			Audience:   types.StringNull(),
		}

		req := provider.ConfigureRequest{
			Config: newTestProviderConfig(ctx, t, schemaResp.Schema, configModel),
		}
		var resp provider.ConfigureResponse

		p.Configure(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		require.NotNil(t, resp.ResourceData)
		data, ok := resp.ResourceData.(*ProviderData)
		require.True(t, ok)
		assert.Equal(t, "localhost:50051", data.HubAddress)
		assert.NotNil(t, data.HubClient)
	})

	t.Run("successful configuration with mock server", func(t *testing.T) {
		_, _, mockAddr := startMockHubServer(t)

		p := New("dev")()
		var schemaResp provider.SchemaResponse
		p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

		configModel := SupercargoProviderModel{
			HubAddress: types.StringValue(mockAddr),
			Token:      types.StringValue("test-token"),
			Audience:   types.StringNull(),
		}

		req := provider.ConfigureRequest{
			Config: newTestProviderConfig(ctx, t, schemaResp.Schema, configModel),
		}
		var resp provider.ConfigureResponse

		p.Configure(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		require.NotNil(t, resp.ResourceData)
		data, ok := resp.ResourceData.(*ProviderData)
		require.True(t, ok)
		assert.Equal(t, mockAddr, data.HubAddress)
		assert.NotNil(t, data.HubClient)
	})

	t.Run("successful configuration with custom audience", func(t *testing.T) {
		_, _, mockAddr := startMockHubServer(t)

		p := New("dev")()
		var schemaResp provider.SchemaResponse
		p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

		configModel := SupercargoProviderModel{
			HubAddress: types.StringValue(mockAddr),
			Token:      types.StringValue("test-token"),
			Audience:   types.StringValue("https://custom-audience.google.com"),
		}

		req := provider.ConfigureRequest{
			Config: newTestProviderConfig(ctx, t, schemaResp.Schema, configModel),
		}
		var resp provider.ConfigureResponse

		p.Configure(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		require.NotNil(t, resp.ResourceData)
		data, ok := resp.ResourceData.(*ProviderData)
		require.True(t, ok)
		assert.Equal(t, mockAddr, data.HubAddress)
		assert.NotNil(t, data.HubClient)
	})

	t.Run("dial connection failure returns diagnostic error", func(t *testing.T) {
		sp := &SupercargoProvider{
			version: "dev",
			factory: hub.NewFactory(),
		}
		// Close the factory to force GetClient to fail with ErrFactoryClosed
		err := sp.factory.Close()
		require.NoError(t, err)

		var schemaResp provider.SchemaResponse
		sp.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

		configModel := SupercargoProviderModel{
			HubAddress: types.StringValue("localhost:50051"),
			Token:      types.StringValue("test-token"),
			Audience:   types.StringNull(),
		}

		req := provider.ConfigureRequest{
			Config: newTestProviderConfig(ctx, t, schemaResp.Schema, configModel),
		}
		var resp provider.ConfigureResponse

		sp.Configure(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Failed to connect to Supercargo Hub")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "hub factory is closed")
	})

	t.Run("config get error handled gracefully", func(t *testing.T) {
		p := New("dev")()
		var schemaResp provider.SchemaResponse
		p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

		req := provider.ConfigureRequest{
			Config: tfsdk.Config{
				Schema: schemaResp.Schema,
				Raw:    tftypes.NewValue(tftypes.String, "invalid-type"),
			},
		}
		var resp provider.ConfigureResponse

		p.Configure(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
	})
}
