package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ resource.Resource = &supercargoIngestionGatewayResource{}
var _ resource.ResourceWithConfigure = &supercargoIngestionGatewayResource{}

func NewSupercargoIngestionGatewayResource() resource.Resource {
	return &supercargoIngestionGatewayResource{}
}

type supercargoIngestionGatewayResource struct {
	client *ProviderData
}

type supercargoIngestionGatewayResourceModel struct {
	ID              types.String `tfsdk:"id"`
	ContractID      types.String `tfsdk:"contract_id"`
	ContractVersion types.String `tfsdk:"contract_version"`
}

func (r *supercargoIngestionGatewayResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ingestion_gateway"
}

func (r *supercargoIngestionGatewayResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"contract_id": schema.StringAttribute{
				Required: true,
			},
			"contract_version": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func (r *supercargoIngestionGatewayResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *supercargoIngestionGatewayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan supercargoIngestionGatewayResourceModel
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

	// Governance Check: Verify contract existence in Hub.
	// This ensures that we don't even attempt to provision infrastructure if the
	// governing contract doesn't exist.
	if err := r.validateContract(ctx, plan.ContractID.ValueString(), plan.ContractVersion.ValueString()); err != nil {
		resp.Diagnostics.AddError("Governance Validation Failed", err.Error())
		return
	}

	plan.ID = types.StringValue("gateway-" + plan.ContractID.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *supercargoIngestionGatewayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state supercargoIngestionGatewayResourceModel
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

	if err := r.validateContract(ctx, state.ContractID.ValueString(), state.ContractVersion.ValueString()); err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Contract for Gateway", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *supercargoIngestionGatewayResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan supercargoIngestionGatewayResourceModel
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
	plan.ID = types.StringValue("gateway-" + plan.ContractID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete handles the de-provisioning of the resource.
// Note: In this 'light' provider implementation, this is currently a no-op as the actual
// cloud resources are managed via the Terraform module 'supercargo-ingestion-gateway'.
// A full implementation would use GCP APIs here to manage the lifecycle of Cloud Run, Pub/Sub, etc.
func (r *supercargoIngestionGatewayResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil || r.client.HubClient == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}
	// No-op for now as infrastructure is module-managed.
}

func (r *supercargoIngestionGatewayResource) validateContract(ctx context.Context, contractID string, contractVersion string) error {
	if r.client == nil || r.client.HubClient == nil {
		// Cannot validate if client is not fully configured (e.g., plan phase)
		return nil
	}

	_, err := r.client.HubClient.GetContract(ctx, &hubv1.GetContractRequest{
		ContractUrn: contractID,
		Version:     contractVersion,
	})
	if err != nil {
		return fmt.Errorf("contract %s version %s not found or inaccessible: %w", contractID, contractVersion, err)
	}

	return nil
}
