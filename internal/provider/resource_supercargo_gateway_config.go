package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
)

// Ensure the implementation satisfies the expected interfaces.
var _ resource.Resource = &supercargoGatewayConfigResource{}
var _ resource.ResourceWithConfigure = &supercargoGatewayConfigResource{}

// NewSupercargoGatewayConfigResource is a helper function to simplify the provider implementation.
func NewSupercargoGatewayConfigResource() resource.Resource {
	return &supercargoGatewayConfigResource{}
}

// supercargoGatewayConfigResource is the resource implementation.
type supercargoGatewayConfigResource struct {
	client *ProviderData
}

// supercargoGatewayConfigResourceModel maps the resource schema data.
type supercargoGatewayConfigResourceModel struct {
	ManifestFile types.String `tfsdk:"manifest_file"`
	HubAddress   types.String `tfsdk:"hub_address"`
	ConfigHash   types.String `tfsdk:"config_hash"`
	EnvVars      types.Map    `tfsdk:"env_vars"`
}

// Metadata returns the resource type name.
func (r *supercargoGatewayConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gateway_config"
}

// Schema defines the schema for the resource.
func (r *supercargoGatewayConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"manifest_file": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Path to the local product.yaml manifest.",
			},
			"hub_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The URL of the Supercargo Hub.",
			},
			"config_hash": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A SHA-256 hash of the manifest and contract versions.",
			},
			"env_vars": schema.MapAttribute{
				Computed:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
				MarkdownDescription: "Map of environment variables for the Gateway.",
			},
		},
	}
}

// Configure adds the provider-level data to the resource.
func (r *supercargoGatewayConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Data Type",
			fmt.Sprintf("Expected *ProviderData, got: %T.", req.ProviderData),
		)
		return
	}

	r.client = data
}

// Create creates the resource and sets the initial Terraform state.
func (r *supercargoGatewayConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan supercargoGatewayConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}

	// 1. Load and Hash Manifest
	manifestData, err := os.ReadFile(plan.ManifestFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Manifest", err.Error())
		return
	}

	productManifest, err := manifest.Load(plan.ManifestFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Parsing Manifest", err.Error())
		return
	}

	// 2. Resolve Contract Versions and Hash
	h := sha256.New()
	h.Write(manifestData)

	var contractURNs []string
	for _, o := range productManifest.OutputPorts {
		if o != nil && o.Contract != nil {
			urn := fmt.Sprintf("%s:%s", o.Contract.Urn, o.Contract.Version)
			contractURNs = append(contractURNs, urn)
		}
	}
	sort.Strings(contractURNs)
	for _, urn := range contractURNs {
		h.Write([]byte(urn))
		h.Write([]byte("\n"))
	}

	configHash := hex.EncodeToString(h.Sum(nil))
	plan.ConfigHash = types.StringValue(configHash)

	// 3. Generate Env Vars
	env := map[string]string{
		"HUB_ADDRESS":              plan.HubAddress.ValueString(),
		"SUPERCARGO_CONFIG_HASH":   configHash,
		"SUPERCARGO_MANIFEST_PATH": plan.ManifestFile.ValueString(),
	}

	envVars, diags := types.MapValueFrom(ctx, types.StringType, env)
	resp.Diagnostics.Append(diags...)
	plan.EnvVars = envVars

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *supercargoGatewayConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state supercargoGatewayConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}

	// Read is similar to Create logic to check for drift if manifest changed on disk
	manifestData, err := os.ReadFile(state.ManifestFile.ValueString())
	if err != nil {
		if os.IsNotExist(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Manifest", err.Error())
		return
	}

	productManifest, err := manifest.Load(state.ManifestFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Parsing Manifest", err.Error())
		return
	}

	h := sha256.New()
	h.Write(manifestData)

	var contractURNs []string
	for _, o := range productManifest.OutputPorts {
		if o != nil && o.Contract != nil {
			urn := fmt.Sprintf("%s:%s", o.Contract.Urn, o.Contract.Version)
			contractURNs = append(contractURNs, urn)
		}
	}
	sort.Strings(contractURNs)
	for _, urn := range contractURNs {
		h.Write([]byte(urn))
		h.Write([]byte("\n"))
	}

	configHash := hex.EncodeToString(h.Sum(nil))
	state.ConfigHash = types.StringValue(configHash)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *supercargoGatewayConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}
	createResp := resource.CreateResponse{State: resp.State}
	r.Create(ctx, resource.CreateRequest{Plan: req.Plan, Config: req.Config}, &createResp)
	resp.Diagnostics.Append(createResp.Diagnostics...)
	resp.State = createResp.State
}

func (r *supercargoGatewayConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}
	// Gateway configuration represents a logical entity tracking drift from a local manifest.
	// Deletion via Terraform simply removes it from state; actual gateway resource cleanup
	// would typically be managed by modifying the surrounding Terraform module usage.
}
