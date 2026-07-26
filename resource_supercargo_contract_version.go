package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Ensure the implementation satisfies the expected interfaces.
var _ resource.Resource = &supercargoContractVersionResource{}
var _ resource.ResourceWithConfigure = &supercargoContractVersionResource{}
var _ resource.ResourceWithModifyPlan = &supercargoContractVersionResource{}

// NewSupercargoContractVersionResource is a helper function to simplify the provider implementation.
func NewSupercargoContractVersionResource() resource.Resource {
	return &supercargoContractVersionResource{}
}

// supercargoContractVersionResource is the resource implementation.
type supercargoContractVersionResource struct {
	client *ProviderData
}

// ModifyPlan performs the plan-time governance handshake.
func (r *supercargoContractVersionResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If the entire resource is being deleted, skip
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan supercargoContractVersionResourceModel
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

	if r.client.HubClient == nil {
		// Provider was configured with an unknown Hub Address (likely being created in this plan).
		// Skip plan-time validation since we can't reach the Hub yet.
		return
	}

	// Handshake: Check compatibility during the plan phase with caching
	contract, err := r.mapToProto(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Schema JSON", err.Error())
		return
	}

	impactCacheKey := fmt.Sprintf("impact:%s:%s:%s", contract.Meta.Urn, contract.Meta.Version, contract.Meta.ContentHash)
	var impact *hubv1.CheckDownstreamImpactResponse
	if val, ok := r.client.Cache.Load(impactCacheKey); ok {
		impact = val.(*hubv1.CheckDownstreamImpactResponse)
	} else {
		res, err := r.client.HubClient.CheckDownstreamImpact(ctx, &hubv1.CheckDownstreamImpactRequest{
			Contract: contract,
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Governance Handshake Failed",
				fmt.Sprintf("Could not check compatibility for contract '%s': %s", plan.URN.ValueString(), status.Convert(err).Message()),
			)
			return
		}
		impact = res
		r.client.Cache.Store(impactCacheKey, impact)
	}

	if impact.Severity == hubv1.ImpactSeverity_IMPACT_SEVERITY_BREAKING {
		resp.Diagnostics.AddError(
			"Breaking Change Detected (Plan-Time Governance)",
			fmt.Sprintf("Proposed changes to %s are incompatible with the existing contract version. Breaking changes: %s",
				plan.URN.ValueString(), strings.Join(impact.BreakingChanges, ", ")),
		)
		return
	}
}

// supercargoContractVersionResourceModel maps the resource schema data.
type supercargoContractVersionResourceModel struct {
	URN         types.String `tfsdk:"urn"`
	Version     types.String `tfsdk:"version"`
	ContentHash types.String `tfsdk:"content_hash"`
	CommitSha   types.String `tfsdk:"commit_sha"`
	SchemaJSON  types.String `tfsdk:"schema_json"`
	DataAsset   types.String `tfsdk:"data_asset"`
}

// Metadata returns the resource type name.
func (r *supercargoContractVersionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contract_version"
}

// Schema defines the schema for the resource.
func (r *supercargoContractVersionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"urn": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Globally unique identifier (URN).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Semantic Versioning (SemVer).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"content_hash": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "SHA-256 of the contract content.",
			},
			"commit_sha": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable Git Commit SHA.",
			},
			"schema_json": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The JSON representation of the schema fields.",
			},
			"data_asset": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Reference to the authoritative source (e.g., 'go://...').",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure adds the provider-level data to the resource.
func (r *supercargoContractVersionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *supercargoContractVersionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan supercargoContractVersionResourceModel
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

	contract, err := r.mapToProto(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Schema JSON", err.Error())
		return
	}

	impact, err := r.client.HubClient.CheckDownstreamImpact(ctx, &hubv1.CheckDownstreamImpactRequest{
		Contract: contract,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Governance Handshake Failed",
			fmt.Sprintf("Could not check compatibility for contract '%s': %s", plan.URN.ValueString(), status.Convert(err).Message()),
		)
		return
	}

	if impact.Severity == hubv1.ImpactSeverity_IMPACT_SEVERITY_BREAKING {
		resp.Diagnostics.AddError(
			"Breaking Change Detected",
			fmt.Sprintf("Proposed changes to %s are incompatible with the existing contract version. Breaking changes: %s",
				plan.URN.ValueString(), strings.Join(impact.BreakingChanges, ", ")),
		)
		return
	}

	// Register the contract
	_, err = r.client.HubClient.RegisterContract(ctx, &hubv1.RegisterContractRequest{
		Contract: contract,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Registering Contract",
			fmt.Sprintf("Could not register contract '%s' in Supercargo Hub: %s", plan.URN.ValueString(), status.Convert(err).Message()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *supercargoContractVersionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state supercargoContractVersionResourceModel
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

	res, err := r.client.HubClient.GetContract(ctx, &hubv1.GetContractRequest{
		ContractUrn: state.URN.ValueString(),
		Version:     state.Version.ValueString(),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Contract",
			fmt.Sprintf("Could not read contract '%s' (v%s) from Supercargo Hub: %s", state.URN.ValueString(), state.Version.ValueString(), status.Convert(err).Message()),
		)
		return
	}

	if res == nil || res.Contract == nil || res.Contract.Meta == nil {
		resp.Diagnostics.AddError(
			"Error Reading Contract",
			"Hub returned an empty or invalid contract response.",
		)
		return
	}

	state.ContentHash = types.StringValue(res.Contract.Meta.ContentHash)
	state.CommitSha = types.StringValue(res.Contract.Meta.CommitSha)
	state.DataAsset = types.StringValue(res.Contract.Meta.DataAsset)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *supercargoContractVersionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}
	// Contracts are immutable by URN+Version. Update is mostly for metadata if allowed.
	// But our resource requires replace for version.
}

func (r *supercargoContractVersionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}
}

func (r *supercargoContractVersionResource) mapToProto(m supercargoContractVersionResourceModel) (*hubv1.DataContract, error) {
	type fieldJSON struct {
		Name        string            `json:"name"`
		Type        string            `json:"type"`
		Mode        string            `json:"mode"`
		Description string            `json:"description,omitempty"`
		Constraints map[string]string `json:"constraints,omitempty"`
		Fields      []fieldJSON       `json:"fields,omitempty"`
	}

	var fields []fieldJSON
	if err := json.Unmarshal([]byte(m.SchemaJSON.ValueString()), &fields); err != nil {
		return nil, fmt.Errorf("failed to parse schema_json: %w", err)
	}

	var mapFields func([]fieldJSON, int, []string) ([]*hubv1.Field, error)
	mapFields = func(fs []fieldJSON, depth int, path []string) ([]*hubv1.Field, error) {
		if err := checkRecursionLimit(path); err != nil {
			return nil, err
		}
		protoFields := make([]*hubv1.Field, len(fs))
		for i, f := range fs {
			var dataType hubv1.DataType
			switch f.Type {
			case "STRING":
				dataType = hubv1.DataType_DATA_TYPE_STRING
			case "INT64":
				dataType = hubv1.DataType_DATA_TYPE_INT64
			case "FLOAT64":
				dataType = hubv1.DataType_DATA_TYPE_FLOAT64
			case "BOOL":
				dataType = hubv1.DataType_DATA_TYPE_BOOL
			case "TIMESTAMP":
				dataType = hubv1.DataType_DATA_TYPE_TIMESTAMP
			case "DATE":
				dataType = hubv1.DataType_DATA_TYPE_DATE
			case "TIME":
				dataType = hubv1.DataType_DATA_TYPE_TIME
			case "DATETIME":
				dataType = hubv1.DataType_DATA_TYPE_DATETIME
			case "GEOGRAPHY":
				dataType = hubv1.DataType_DATA_TYPE_GEOGRAPHY
			case "NUMERIC":
				dataType = hubv1.DataType_DATA_TYPE_NUMERIC
			case "BIGNUMERIC":
				dataType = hubv1.DataType_DATA_TYPE_BIGNUMERIC
			case "BYTES":
				dataType = hubv1.DataType_DATA_TYPE_BYTES
			case "JSON":
				dataType = hubv1.DataType_DATA_TYPE_JSON
			case "STRUCT", "RECORD":
				dataType = hubv1.DataType_DATA_TYPE_STRUCT
			default:
				return nil, fmt.Errorf("unsupported or missing data type for field %q: %q", f.Name, f.Type)
			}

			fieldMode := hubv1.FieldMode_FIELD_MODE_NULLABLE
			switch f.Mode {
			case "REQUIRED":
				fieldMode = hubv1.FieldMode_FIELD_MODE_REQUIRED
			case "REPEATED":
				fieldMode = hubv1.FieldMode_FIELD_MODE_REPEATED
			}

			subFields, err := mapFields(f.Fields, depth+1, append(path, f.Name))
			if err != nil {
				return nil, err
			}

			constraints, err := mapConstraints(f.Constraints)
			if err != nil {
				return nil, err
			}

			protoFields[i] = &hubv1.Field{
				Name:        f.Name,
				Description: f.Description,
				Type:        dataType,
				Mode:        fieldMode,
				Fields:      subFields,
				Constraints: constraints,
			}
		}
		return protoFields, nil
	}

	protoSchema, err := mapFields(fields, 0, []string{})
	if err != nil {
		return nil, err
	}

	return &hubv1.DataContract{
		Meta: &hubv1.Meta{
			Urn:         m.URN.ValueString(),
			Version:     m.Version.ValueString(),
			ContentHash: m.ContentHash.ValueString(),
			CommitSha:   m.CommitSha.ValueString(),
			DataAsset:   m.DataAsset.ValueString(),
		},
		Schema: protoSchema,
	}, nil
}
