package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Ensure the implementation satisfies the expected interfaces.
var _ resource.Resource = &supercargoDataProductResource{}
var _ resource.ResourceWithConfigure = &supercargoDataProductResource{}
var _ resource.ResourceWithModifyPlan = &supercargoDataProductResource{}

// NewSupercargoDataProductResource is a helper function to simplify the provider implementation.
func NewSupercargoDataProductResource() resource.Resource {
	return &supercargoDataProductResource{}
}

// supercargoDataProductResource is the resource implementation.
type supercargoDataProductResource struct {
	client *ProviderData
}

// ModifyPlan performs plan-time validation of overrides.
func (r *supercargoDataProductResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan supercargoDataProductResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 1. Load Manifest
	productManifest, err := manifest.Load(plan.ManifestFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Loading Manifest", err.Error())
		return
	}

	// 2. Calculate Service Identities (Deterministic Handshake)
	// We use the project from plan if available, otherwise fallback to manifest
	targetProject := ""
	if !plan.Project.IsNull() && !plan.Project.IsUnknown() {
		targetProject = plan.Project.ValueString()
	}

	if targetProject == "" {
		// Validate manifest project consistency if no override provided
		for i, port := range productManifest.OutputPorts {
			if port != nil && port.Physical != nil && port.Physical.Bigquery != nil && port.Physical.Bigquery.Project != "" {
				if targetProject == "" {
					targetProject = port.Physical.Bigquery.Project
				} else if targetProject != port.Physical.Bigquery.Project {
					resp.Diagnostics.AddError(
						"Inconsistent GCP Projects in Manifest",
						fmt.Sprintf("Output port %d uses project '%s', but previous ports used '%s'. Supercargo requires all ports in a manifest to share the same project for identity generation unless overridden in HCL.", i, port.Physical.Bigquery.Project, targetProject),
					)
					return
				}
			}
		}
	}

	if targetProject != "" && productManifest.Meta != nil && productManifest.Meta.Urn != "" {
		identitiesMap, diags := calculateServiceIdentities(ctx, targetProject, productManifest.Meta.Urn)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.ServiceIdentities = identitiesMap
	}

	// 2.1 Validate SLA Tier
	if plan.SLA != nil && !plan.SLA.Tier.IsNull() && !plan.SLA.Tier.IsUnknown() {
		tierStr := plan.SLA.Tier.ValueString()
		if _, ok := parseSLATier(tierStr); !ok {
			resp.Diagnostics.AddError(
				"Invalid SLA Tier",
				fmt.Sprintf("'%s' is not a valid SLA tier. Valid values include: SLA_TIER_1_MISSION_CRITICAL (GOLD, TIER_1, MISSION_CRITICAL), SLA_TIER_2_IMPORTANT (SILVER, TIER_2, IMPORTANT), SLA_TIER_3_BEST_EFFORT (BRONZE, TIER_3, BEST_EFFORT)", tierStr),
			)
		}
	}

	// 3. Validation Logic (Requires Hub Client)
	if r.client != nil && r.client.HubClient != nil {
		// Validate Team existence
		if productManifest.Meta == nil || productManifest.Meta.Owner == nil || productManifest.Meta.Owner.TeamName == "" {
			resp.Diagnostics.AddError(
				"Invalid Product Manifest",
				"The product manifest is missing metadata or owner information.",
			)
			return
		}
		teamName := productManifest.Meta.Owner.TeamName
		cacheKey := "team:" + teamName
		if _, ok := r.client.Cache.Load(cacheKey); !ok {
			_, err = r.client.HubClient.GetTeam(ctx, &hubv1.GetTeamRequest{
				Name: teamName,
			})
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.NotFound {
					resp.Diagnostics.AddWarning(
						"Team Not Found (Bootstrap Mode)",
						fmt.Sprintf("Team '%s' was not found in the Hub. Plan will proceed assuming it's being created in the current plan.", teamName),
					)
				} else if ok && st.Code() != codes.Unavailable {
					resp.Diagnostics.AddError(
						"Governance Handshake Failed",
						fmt.Sprintf("Could not verify team '%s' in Hub: %s", teamName, st.Message()),
					)
					return
				}
			} else {
				r.client.Cache.Store(cacheKey, true)
			}
		}

		// Validate Output Port Contracts
		contractsChecked := 0
		var loadedContracts []*hubv1.DataContract
		for _, port := range productManifest.OutputPorts {
			if port == nil || port.Contract == nil || port.Contract.Urn == "" {
				continue
			}

			// Fetch contract from Hub with caching
			contractCacheKey := fmt.Sprintf("contract:%s:%s", port.Contract.Urn, port.Contract.Version)
			var contract *hubv1.DataContract
			if val, ok := r.client.Cache.Load(contractCacheKey); ok {
				contract = val.(*hubv1.DataContract)
			} else {
				res, err := r.client.HubClient.GetContract(ctx, &hubv1.GetContractRequest{
					ContractUrn: port.Contract.Urn,
					Version:     port.Contract.Version,
				})
				if err != nil {
					st, ok := status.FromError(err)
					if ok && st.Code() == codes.NotFound {
						resp.Diagnostics.AddError(
							"Governing Contract Not Found",
							fmt.Sprintf("Contract '%s' version '%s' was not found in the Hub.", port.Contract.Urn, port.Contract.Version),
						)
						return
					} else if ok && st.Code() != codes.Unavailable {
						resp.Diagnostics.AddError(
							"Governance Handshake Failed",
							fmt.Sprintf("Could not verify contract '%s' in Hub: %s", port.Contract.Urn, st.Message()),
						)
						return
					}
				} else if res != nil && res.Contract != nil {
					contract = res.Contract
					r.client.Cache.Store(contractCacheKey, contract)
				}
			}

			if contract != nil {
				contractsChecked++
				loadedContracts = append(loadedContracts, contract)
			}
		}

		// Validate Partitioning Field against contracts if specified
		if !plan.PartitioningField.IsNull() && !plan.PartitioningField.IsUnknown() && plan.PartitioningField.ValueString() != "" {
			partitioningField := plan.PartitioningField.ValueString()
			fieldFound := false
			for _, contract := range loadedContracts {
				for _, f := range contract.Schema {
					if f != nil && f.Name == partitioningField {
						fieldFound = true
						break
					}
				}
				if fieldFound {
					break
				}
			}

			if contractsChecked > 0 && !fieldFound {
				resp.Diagnostics.AddError(
					"Invalid Partitioning Field",
					fmt.Sprintf("The partitioning field '%s' was not found in any of the contracts associated with this data product.", partitioningField),
				)
			}
		}
	}

	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// supercargoDataProductResourceModel maps the resource schema data.
type supercargoDataProductResourceModel struct {
	ManifestFile          types.String        `tfsdk:"manifest_file"`
	URN                   types.String        `tfsdk:"urn"`
	Version               types.String        `tfsdk:"version"`
	Project               types.String        `tfsdk:"project"`
	Location              types.String        `tfsdk:"location"`
	PartitioningField     types.String        `tfsdk:"partitioning_field"`
	PartitionExpirationMs types.Int64         `tfsdk:"partition_expiration_ms"`
	ServiceIdentities     types.Map           `tfsdk:"service_identities"`
	SLA                   *supercargoSLAModel `tfsdk:"sla"`
}

type supercargoSLAModel struct {
	Tier      types.String `tfsdk:"tier"`
	Rto       types.String `tfsdk:"rto"`
	Freshness types.String `tfsdk:"freshness"`
	Latency   types.String `tfsdk:"latency"`
}

// Metadata returns the resource type name.
func (r *supercargoDataProductResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_data_product"
}

// Schema defines the schema for the resource.
func (r *supercargoDataProductResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"manifest_file": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Path to the local product.yaml manifest.",
			},
			"urn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Globally unique identifier (URN).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Version of the data product.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "GCP Project ID (Overrides manifest).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"location": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "BigQuery Location (Overrides manifest).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"partitioning_field": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "BigQuery Partitioning Field (Overrides manifest).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"partition_expiration_ms": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "BigQuery Partition Expiration in milliseconds (Overrides manifest).",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"service_identities": schema.MapAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Map of component identities (e.g. gateway, shovel) as IAM member strings.",
			},
			"sla": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Service Level Agreement (SLA) overrides for the data product.",
				Attributes: map[string]schema.Attribute{
					"tier": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Criticality tier (e.g., Tier 1 - Mission Critical).",
					},
					"rto": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Recovery Time Objective (e.g., '4h', '24h').",
					},
					"freshness": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Target data freshness window (e.g., '15m', '1h').",
					},
					"latency": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Maximum ingestion latency.",
					},
				},
			},
		},
	}
}

// Configure adds the provider-level data to the resource.
func (r *supercargoDataProductResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *supercargoDataProductResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan supercargoDataProductResourceModel
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

	// 1. Read and Parse Manifest
	productManifest, err := manifest.Load(plan.ManifestFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Loading Manifest", err.Error())
		return
	}

	// 2. Apply Overrides (Deep Merge)
	r.applyOverrides(productManifest, &plan)

	// 3. Register Product in Hub
	res, err := r.client.HubClient.RegisterProduct(ctx, &hubv1.RegisterProductRequest{
		Manifest: productManifest,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Registering Product", err.Error())
		return
	}

	// 4. Update State
	plan.URN = types.StringValue(res.ProductUrn)
	plan.Version = types.StringValue(res.Version)

	targetProject := ""
	if !plan.Project.IsNull() && !plan.Project.IsUnknown() {
		targetProject = plan.Project.ValueString()
	}

	// Ensure overrides that were applied are reflected in the computed fields if they weren't in HCL
	if plan.Project.IsUnknown() || plan.Project.IsNull() {
		if len(productManifest.OutputPorts) > 0 && productManifest.OutputPorts[0] != nil && productManifest.OutputPorts[0].Physical != nil && productManifest.OutputPorts[0].Physical.Bigquery != nil && productManifest.OutputPorts[0].Physical.Bigquery.Project != "" {
			targetProject = productManifest.OutputPorts[0].Physical.Bigquery.Project
			plan.Project = types.StringValue(targetProject)
		} else {
			plan.Project = types.StringNull()
		}
	}
	if plan.Location.IsUnknown() || plan.Location.IsNull() {
		if len(productManifest.OutputPorts) > 0 && productManifest.OutputPorts[0] != nil && productManifest.OutputPorts[0].Physical != nil && productManifest.OutputPorts[0].Physical.Bigquery != nil && productManifest.OutputPorts[0].Physical.Bigquery.Location != "" {
			plan.Location = types.StringValue(productManifest.OutputPorts[0].Physical.Bigquery.Location)
		} else {
			plan.Location = types.StringNull()
		}
	}
	if plan.PartitioningField.IsUnknown() || plan.PartitioningField.IsNull() {
		if len(productManifest.OutputPorts) > 0 && productManifest.OutputPorts[0] != nil && productManifest.OutputPorts[0].Physical != nil && productManifest.OutputPorts[0].Physical.Bigquery != nil && productManifest.OutputPorts[0].Physical.Bigquery.PartitionBy != "" {
			plan.PartitioningField = types.StringValue(productManifest.OutputPorts[0].Physical.Bigquery.PartitionBy)
		} else {
			plan.PartitioningField = types.StringNull()
		}
	}
	if plan.PartitionExpirationMs.IsUnknown() || plan.PartitionExpirationMs.IsNull() {
		if len(productManifest.OutputPorts) > 0 && productManifest.OutputPorts[0] != nil {
			plan.PartitionExpirationMs = durationToMs(productManifest.OutputPorts[0].Physical)
		} else {
			plan.PartitionExpirationMs = types.Int64Null()
		}
	}

	productUrn := res.ProductUrn
	if productUrn == "" && productManifest.Meta != nil {
		productUrn = productManifest.Meta.Urn
	}
	identitiesMap, diags := calculateServiceIdentities(ctx, targetProject, productUrn)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ServiceIdentities = identitiesMap

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *supercargoDataProductResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state supercargoDataProductResourceModel
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

	res, err := r.client.HubClient.GetProduct(ctx, &hubv1.GetProductRequest{
		ProductUrn: state.URN.ValueString(),
		Version:    state.Version.ValueString(),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Product", err.Error())
		return
	}

	if res == nil || res.Manifest == nil || res.Manifest.Meta == nil {
		resp.Diagnostics.AddError(
			"Invalid Product Manifest",
			"The product returned from the Hub is missing manifest or metadata.",
		)
		return
	}
	state.Version = types.StringValue(res.Manifest.Meta.Version)

	// Find first project ID in manifest
	manifestProject := ""
	for _, port := range res.Manifest.OutputPorts {
		if port != nil && port.Physical != nil && port.Physical.Bigquery != nil && port.Physical.Bigquery.Project != "" {
			manifestProject = port.Physical.Bigquery.Project
			state.Project = types.StringValue(manifestProject)
			state.Location = types.StringValue(port.Physical.Bigquery.Location)
			state.PartitioningField = types.StringValue(port.Physical.Bigquery.PartitionBy)
			state.PartitionExpirationMs = durationToMs(port.Physical)
			break
		}
	}

	if manifestProject != "" && res.Manifest.Meta != nil {
		identitiesMap, diags := calculateServiceIdentities(ctx, manifestProject, res.Manifest.Meta.Urn)
		resp.Diagnostics.Append(diags...)
		state.ServiceIdentities = identitiesMap
	} else {
		state.Project = types.StringNull()
		state.Location = types.StringNull()
		state.PartitioningField = types.StringNull()
		state.PartitionExpirationMs = types.Int64Null()
		state.ServiceIdentities = types.MapNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *supercargoDataProductResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil || r.client.HubClient == nil {
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

func (r *supercargoDataProductResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil || r.client.HubClient == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the resource can be managed.",
		)
		return
	}
	// The Hub API does not currently support deletion of data products for safety reasons.
	// This is a no-op; Terraform will just safely remove the resource from the state.
}

func (r *supercargoDataProductResource) applyOverrides(m *hubv1.ProductManifest, plan *supercargoDataProductResourceModel) {
	if !plan.Project.IsNull() && !plan.Project.IsUnknown() {
		for _, p := range m.OutputPorts {
			if bq := ensureBqConfig(p); bq != nil {
				bq.Project = plan.Project.ValueString()
			}
		}
	}
	if !plan.Location.IsNull() && !plan.Location.IsUnknown() {
		for _, p := range m.OutputPorts {
			if bq := ensureBqConfig(p); bq != nil {
				bq.Location = plan.Location.ValueString()
			}
		}
	}
	if !plan.PartitioningField.IsNull() && !plan.PartitioningField.IsUnknown() {
		for _, p := range m.OutputPorts {
			if bq := ensureBqConfig(p); bq != nil {
				bq.PartitionBy = plan.PartitioningField.ValueString()
			}
		}
	}
	if !plan.PartitionExpirationMs.IsNull() && !plan.PartitionExpirationMs.IsUnknown() {
		for _, p := range m.OutputPorts {
			if bq := ensureBqConfig(p); bq != nil {
				bq.PartitionExpiration = durationpb.New(time.Duration(plan.PartitionExpirationMs.ValueInt64()) * time.Millisecond)
			}
		}
	}

	if plan.SLA != nil {
		if m.Sla == nil {
			m.Sla = &hubv1.SLA{}
		}
		if !plan.SLA.Tier.IsNull() && !plan.SLA.Tier.IsUnknown() {
			if tier, ok := parseSLATier(plan.SLA.Tier.ValueString()); ok {
				m.Sla.Tier = tier
			}
		}
		if !plan.SLA.Rto.IsNull() && !plan.SLA.Rto.IsUnknown() {
			m.Sla.Rto = plan.SLA.Rto.ValueString()
		}
		if !plan.SLA.Freshness.IsNull() && !plan.SLA.Freshness.IsUnknown() {
			m.Sla.Freshness = plan.SLA.Freshness.ValueString()
		}
		if !plan.SLA.Latency.IsNull() && !plan.SLA.Latency.IsUnknown() {
			m.Sla.Latency = plan.SLA.Latency.ValueString()
		}
	}
}

func parseSLATier(tierStr string) (hubv1.SLATier, bool) {
	s := strings.ToUpper(strings.TrimSpace(tierStr))
	// Check direct enum map
	if val, ok := hubv1.SLATier_value[s]; ok {
		return hubv1.SLATier(val), true
	}
	if val, ok := hubv1.SLATier_value["SLA_TIER_"+s]; ok {
		return hubv1.SLATier(val), true
	}

	// Friendly alias mapping
	switch s {
	case "GOLD", "TIER_1", "TIER1", "1", "MISSION_CRITICAL", "MISSION-CRITICAL":
		return hubv1.SLATier_SLA_TIER_1_MISSION_CRITICAL, true
	case "SILVER", "TIER_2", "TIER2", "2", "IMPORTANT":
		return hubv1.SLATier_SLA_TIER_2_IMPORTANT, true
	case "BRONZE", "TIER_3", "TIER3", "3", "BEST_EFFORT", "BEST-EFFORT":
		return hubv1.SLATier_SLA_TIER_3_BEST_EFFORT, true
	case "UNSPECIFIED", "0":
		return hubv1.SLATier_SLA_TIER_UNSPECIFIED, true
	default:
		return hubv1.SLATier_SLA_TIER_UNSPECIFIED, false
	}
}

func ensureBqConfig(p *hubv1.OutputPort) *hubv1.BigQueryConfig {
	if p == nil {
		return nil
	}
	if p.Physical == nil {
		p.Physical = &hubv1.PhysicalConfig{Bigquery: &hubv1.BigQueryConfig{}}
	}
	if p.Physical.Bigquery == nil {
		p.Physical.Bigquery = &hubv1.BigQueryConfig{}
	}
	return p.Physical.Bigquery
}

func durationToMs(p *hubv1.PhysicalConfig) types.Int64 {
	if p != nil && p.Bigquery != nil && p.Bigquery.PartitionExpiration != nil {
		return types.Int64Value(p.Bigquery.PartitionExpiration.AsDuration().Milliseconds())
	}
	return types.Int64Null()
}

func calculateServiceIdentities(ctx context.Context, targetProject, productUrn string) (types.Map, diag.Diagnostics) {
	if targetProject == "" || productUrn == "" {
		return types.MapNull(types.StringType), nil
	}
	urnParts := strings.Split(productUrn, ":")
	productName := urnParts[len(urnParts)-1]
	components := []string{"gateway", "shovel"}
	identities := make(map[string]types.String, len(components))
	for _, comp := range components {
		email := fmt.Sprintf("%s-%s@%s.iam.gserviceaccount.com", comp, productName, targetProject)
		identities[comp] = types.StringValue("serviceAccount:" + email)
	}
	return types.MapValueFrom(ctx, types.StringType, identities)
}
