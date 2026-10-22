package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/hub"
)

// Ensure SupercargoProvider satisfies various provider interfaces.
var _ provider.Provider = &SupercargoProvider{}

// SupercargoProvider defines the provider implementation.
type SupercargoProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
	factory *hub.Factory
}

// SupercargoProviderModel describes the provider data model.
type SupercargoProviderModel struct {
	HubAddress types.String `tfsdk:"hub_address"`
	Token      types.String `tfsdk:"token"`
	Audience   types.String `tfsdk:"audience"`
	UseTLS     types.Bool   `tfsdk:"use_tls"`
}

// ProviderData is the data passed to data sources and resources.
type ProviderData struct {
	HubAddress string
	HubClient  *hub.Client
	Cache      *sync.Map
}

func (p *SupercargoProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "supercargo"
	resp.Version = p.version
}

func (p *SupercargoProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"hub_address": schema.StringAttribute{
				MarkdownDescription: "Address of the Supercargo Hub (HubService).",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Explicit OIDC ID token for authenticating with the Hub. If omitted, the provider will attempt to generate one automatically using Application Default Credentials.",
				Optional:            true,
				Sensitive:           true,
			},
			"audience": schema.StringAttribute{
				MarkdownDescription: "Target OIDC audience for Hub authentication (defaults to https://<hub-host>).",
				Optional:            true,
			},
			"use_tls": schema.BoolAttribute{
				MarkdownDescription: "Whether to connect to the Hub using TLS. If omitted, defaults to false for localhost/loopback addresses and true for all other hosts.",
				Optional:            true,
			},
		},
	}
}

func (p *SupercargoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data SupercargoProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.HubAddress.IsUnknown() || data.Token.IsUnknown() || data.Audience.IsUnknown() || data.UseTLS.IsUnknown() {
		// HubAddress, Token, Audience, or UseTLS is not known yet (e.g. creating the Hub in this run).
		// We can't configure the client, but we must pass ProviderData down
		// so ModifyPlan can detect the client is nil and skip validation.
		providerData := &ProviderData{
			HubAddress: "unknown",
			HubClient:  nil,
			Cache:      &sync.Map{},
		}
		resp.DataSourceData = providerData
		resp.ResourceData = providerData
		return
	}

	hubAddress := data.HubAddress.ValueString()
	if hubAddress == "" {
		// Default to localhost for development if not provided
		hubAddress = "localhost:50051"
	}

	var useTLS *bool
	if !data.UseTLS.IsNull() && !data.UseTLS.IsUnknown() {
		val := data.UseTLS.ValueBool()
		useTLS = &val
	}

	client, err := p.factory.GetClientWithTLS(ctx, hubAddress, data.Token.ValueString(), data.Audience.ValueString(), useTLS)
	if err != nil {
		detail := fmt.Sprintf("failed to dial hub at %s: %s\n\nHint: Remote Hub connections default to TLS (use_tls = true). If connecting to a plaintext internal or test Hub, explicitly set use_tls = false in provider \"supercargo\".", hubAddress, err)
		resp.Diagnostics.AddError(
			"Error Creating Hub Client",
			detail,
		)
		return
	}

	providerData := &ProviderData{
		HubAddress: hubAddress,
		HubClient:  client,
		Cache:      &sync.Map{},
	}

	resp.DataSourceData = providerData
	resp.ResourceData = providerData
}

func (p *SupercargoProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSupercargoTeamResource,
	}
}

func (p *SupercargoProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSupercargoContractDataSource,
		NewSupercargoDataProductDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SupercargoProvider{
			version: version,
			factory: hub.NewFactory(),
		}
	}
}
