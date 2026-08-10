package main

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
		},
	}
}

func (p *SupercargoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data SupercargoProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.HubAddress.IsUnknown() || data.Token.IsUnknown() {
		// HubAddress or Token is not known yet (e.g. creating the Hub in this run).
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

	client, err := p.factory.GetClient(ctx, hubAddress, data.Token.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to connect to Supercargo Hub",
			fmt.Sprintf("Unable to connect to Hub at %s: %s", hubAddress, err),
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
		NewSupercargoIngestionGatewayResource,
		NewSupercargoTeamResource,
		NewSupercargoContractVersionResource,
		NewSupercargoDataProductResource,
		NewSupercargoGatewayConfigResource,
	}
}

func (p *SupercargoProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSupercargoContractDataSource,
	}
}

func New() provider.Provider {
	return &SupercargoProvider{
		version: "dev",
		factory: hub.NewFactory(),
	}
}
