package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	platformv1 "github.com/supercargo-dev/core/gen/go/platform/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Ensure the implementation satisfies the expected interfaces.
var _ resource.Resource = &supercargoTeamResource{}
var _ resource.ResourceWithConfigure = &supercargoTeamResource{}

// NewSupercargoTeamResource is a helper function to simplify the provider implementation.
func NewSupercargoTeamResource() resource.Resource {
	return &supercargoTeamResource{}
}

// supercargoTeamResource is the resource implementation.
type supercargoTeamResource struct {
	client *ProviderData
}

// supercargoTeamResourceModel maps the resource schema data.
type supercargoTeamResourceModel struct {
	Name      types.String `tfsdk:"name"`
	URN       types.String `tfsdk:"urn"`
	DataAsset types.String `tfsdk:"data_asset"`
	Members   types.List   `tfsdk:"members"`
	Metadata  types.Map    `tfsdk:"metadata"`
}

// Metadata returns the resource type name.
func (r *supercargoTeamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

// Schema defines the schema for the resource.
func (r *supercargoTeamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique slug for the team (e.g., 'mobile-analytics').",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"urn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Globally unique identifier (URN).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"data_asset": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Reference to the authoritative source (e.g., 'go://...').",
			},
			"members": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "List of User Principals (emails) or Group Principals belonging to this team.",
			},
			"metadata": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Arbitrary metadata for the team.",
			},
		},
	}
}

// Configure adds the provider-level data to the resource.
func (r *supercargoTeamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Data Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = data
}

// Create creates the resource and sets the initial Terraform state.
func (r *supercargoTeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan supercargoTeamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil || r.client.HubClient == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}

	// Map plan to proto
	team := &platformv1.Team{
		Name:      plan.Name.ValueString(),
		DataAsset: plan.DataAsset.ValueString(),
	}

	if !plan.Members.IsNull() && !plan.Members.IsUnknown() {
		resp.Diagnostics.Append(plan.Members.ElementsAs(ctx, &team.Members, false)...)
	}

	if !plan.Metadata.IsNull() && !plan.Metadata.IsUnknown() {
		resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &team.Metadata, false)...)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Call Hub
	_, err := r.client.HubClient.RegisterTeam(ctx, &hubv1.RegisterTeamRequest{
		Team: team,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Team",
			fmt.Sprintf("Could not create team '%s' in Supercargo Hub: %s", team.Name, status.Convert(err).Message()),
		)
		return
	}

	// Update state with URN (deterministic)
	plan.URN = types.StringValue(fmt.Sprintf("urn:supercargo:hub:team:%s", team.Name))

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data from the Hub.
func (r *supercargoTeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state supercargoTeamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil || r.client.HubClient == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}

	// Call Hub
	res, err := r.client.HubClient.GetTeam(ctx, &hubv1.GetTeamRequest{
		Name: state.Name.ValueString(),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Team",
			fmt.Sprintf("Could not read team '%s' from Supercargo Hub: %s", state.Name.ValueString(), status.Convert(err).Message()),
		)
		return
	}

	if res == nil || res.Team == nil {
		resp.Diagnostics.AddError(
			"Error Reading Team",
			"Hub returned an empty response.",
		)
		return
	}

	// Update state
	state.DataAsset = types.StringValue(res.Team.DataAsset)
	state.URN = types.StringValue(res.Team.Urn)

	members, diags := types.ListValueFrom(ctx, types.StringType, res.Team.Members)
	resp.Diagnostics.Append(diags...)
	state.Members = members

	metadata, diags := types.MapValueFrom(ctx, types.StringType, res.Team.Metadata)
	resp.Diagnostics.Append(diags...)
	state.Metadata = metadata

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update updates the resource and sets the updated Terraform state.
func (r *supercargoTeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan supercargoTeamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil || r.client.HubClient == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}

	team := &platformv1.Team{
		Name:      plan.Name.ValueString(),
		DataAsset: plan.DataAsset.ValueString(),
	}

	if !plan.Members.IsNull() && !plan.Members.IsUnknown() {
		resp.Diagnostics.Append(plan.Members.ElementsAs(ctx, &team.Members, false)...)
	}

	if !plan.Metadata.IsNull() && !plan.Metadata.IsUnknown() {
		resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &team.Metadata, false)...)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Call Hub (RegisterTeam is idempotent and acts as Update)
	_, err := r.client.HubClient.RegisterTeam(ctx, &hubv1.RegisterTeamRequest{
		Team: team,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Team",
			fmt.Sprintf("Could not update team '%s' in Supercargo Hub: %s", team.Name, status.Convert(err).Message()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the resource and removes the Terraform state.
func (r *supercargoTeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil || r.client.HubClient == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}
	// Hub does not currently support team deletion via API for safety reasons,
	// but we could implement a 'deactivate' or similar if needed.
	// For now, we just remove from Terraform state.
}
