package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

var _ datasource.DataSource = &supercargoDataProductDataSource{}
var _ datasource.DataSourceWithConfigure = &supercargoDataProductDataSource{}

// NewSupercargoDataProductDataSource is a constructor for the supercargo_data_product data source.
func NewSupercargoDataProductDataSource() datasource.DataSource {
	return &supercargoDataProductDataSource{}
}

type supercargoDataProductDataSource struct {
	client *ProviderData
}

type supercargoDataProductDataSourceModel struct {
	ID                    types.String        `tfsdk:"id"`
	Version               types.String        `tfsdk:"version"`
	ManifestFile          types.String        `tfsdk:"manifest_file"`
	Project               types.String        `tfsdk:"project"`
	URN                   types.String        `tfsdk:"urn"`
	Location              types.String        `tfsdk:"location"`
	PartitioningField     types.String        `tfsdk:"partitioning_field"`
	PartitionExpirationMs types.Int64         `tfsdk:"partition_expiration_ms"`
	ServiceIdentities     types.Map           `tfsdk:"service_identities"`
	Contracts             types.Map           `tfsdk:"contracts"`
	SLA                   *supercargoSLAModel `tfsdk:"sla"`
}

type supercargoSLAModel struct {
	Tier      types.String `tfsdk:"tier"`
	Rto       types.String `tfsdk:"rto"`
	Freshness types.String `tfsdk:"freshness"`
	Latency   types.String `tfsdk:"latency"`
}

type supercargoContractModel struct {
	ID          types.String `tfsdk:"id"`
	Schema      types.String `tfsdk:"schema"`
	Version     types.String `tfsdk:"version"`
	ContentHash types.String `tfsdk:"content_hash"`
	CommitSha   types.String `tfsdk:"commit_sha"`
}

var contractAttrTypes = map[string]attr.Type{
	"id":           types.StringType,
	"schema":       types.StringType,
	"version":      types.StringType,
	"content_hash": types.StringType,
	"commit_sha":   types.StringType,
}

var contractObjectType = types.ObjectType{
	AttrTypes: contractAttrTypes,
}

func (d *supercargoDataProductDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_data_product"
}

func (d *supercargoDataProductDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves metadata, BigQuery physical configuration, service identities, SLA, and data contracts for a Supercargo Data Product.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The URN of the Data Product. Optional if manifest_file is provided.",
				Optional:            true,
				Computed:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The version of the Data Product. If omitted, latest active version will be retrieved.",
				Optional:            true,
				Computed:            true,
			},
			"manifest_file": schema.StringAttribute{
				MarkdownDescription: "Path to the local product.yaml manifest file.",
				Optional:            true,
			},
			"project": schema.StringAttribute{
				MarkdownDescription: "GCP Project ID override.",
				Optional:            true,
				Computed:            true,
			},
			"urn": schema.StringAttribute{
				MarkdownDescription: "Globally unique identifier (URN) of the Data Product.",
				Computed:            true,
			},
			"location": schema.StringAttribute{
				MarkdownDescription: "BigQuery dataset location resolved from the Data Product output ports.",
				Computed:            true,
			},
			"partitioning_field": schema.StringAttribute{
				MarkdownDescription: "BigQuery partitioning field resolved from the Data Product output ports.",
				Computed:            true,
			},
			"partition_expiration_ms": schema.Int64Attribute{
				MarkdownDescription: "BigQuery partition expiration in milliseconds.",
				Computed:            true,
			},
			"service_identities": schema.MapAttribute{
				MarkdownDescription: "Map of component identities (e.g. gateway, shovel) as IAM member strings.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"contracts": schema.MapNestedAttribute{
				MarkdownDescription: "Map of resolved data contracts keyed by contract URN or port name.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Contract URN.",
							Computed:            true,
						},
						"schema": schema.StringAttribute{
							MarkdownDescription: "BigQuery JSON schema.",
							Computed:            true,
						},
						"version": schema.StringAttribute{
							MarkdownDescription: "Contract semantic version.",
							Computed:            true,
						},
						"content_hash": schema.StringAttribute{
							MarkdownDescription: "SHA256 content hash of the contract.",
							Computed:            true,
						},
						"commit_sha": schema.StringAttribute{
							MarkdownDescription: "Git commit SHA of the contract.",
							Computed:            true,
						},
					},
				},
			},
			"sla": schema.SingleNestedAttribute{
				MarkdownDescription: "Service Level Agreement (SLA) for the data product.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"tier": schema.StringAttribute{
						MarkdownDescription: "Criticality tier (e.g., SLA_TIER_1_MISSION_CRITICAL).",
						Computed:            true,
					},
					"rto": schema.StringAttribute{
						MarkdownDescription: "Recovery Time Objective (e.g., '4h', '24h').",
						Computed:            true,
					},
					"freshness": schema.StringAttribute{
						MarkdownDescription: "Target data freshness window (e.g., '15m', '1h').",
						Computed:            true,
					},
					"latency": schema.StringAttribute{
						MarkdownDescription: "Maximum ingestion latency.",
						Computed:            true,
					},
				},
			},
		},
	}
}

func (d *supercargoDataProductDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Data Type",
			fmt.Sprintf("Expected *ProviderData, got: %T.", req.ProviderData),
		)
		return
	}

	d.client = data
}

func (d *supercargoDataProductDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data supercargoDataProductDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !data.ID.IsNull() && !data.ID.IsUnknown() && data.ID.ValueString() != ""
	hasManifest := !data.ManifestFile.IsNull() && !data.ManifestFile.IsUnknown() && data.ManifestFile.ValueString() != ""

	if !hasID && !hasManifest {
		resp.Diagnostics.AddError(
			"Missing Required Attribute",
			"Either 'id' or 'manifest_file' must be specified.",
		)
		return
	}

	var localManifest *hubv1.ProductManifest
	manifestPath := ""
	if hasManifest {
		manifestPath = data.ManifestFile.ValueString()
		manifestBytes, err := os.ReadFile(manifestPath)
		if err != nil {
			resp.Diagnostics.AddError("Error Loading Manifest", fmt.Sprintf("Failed to read manifest file %q: %s", manifestPath, err))
			return
		}
		var rawYaml map[string]any
		if err := yaml.Unmarshal(manifestBytes, &rawYaml); err != nil {
			resp.Diagnostics.AddError("Error Loading Manifest", fmt.Sprintf("Failed to parse YAML from %q: %s", manifestPath, err))
			return
		}

		if slaRaw, ok := rawYaml["sla"].(map[string]any); ok {
			if tierVal, ok := slaRaw["tier"].(string); ok && tierVal != "" {
				parsedTier, ok := parseSLATier(tierVal)
				if !ok {
					resp.Diagnostics.AddError(
						"Invalid SLA Tier",
						fmt.Sprintf("'%s' is not a valid SLA tier. Valid values include: SLA_TIER_1_MISSION_CRITICAL (GOLD, TIER_1, MISSION_CRITICAL), SLA_TIER_2_IMPORTANT (SILVER, TIER_2, IMPORTANT), SLA_TIER_3_BEST_EFFORT (BRONZE, TIER_3, BEST_EFFORT)", tierVal),
					)
					return
				}
				slaRaw["tier"] = parsedTier.String()
			}
		}

		jsonBytes, err := json.Marshal(rawYaml)
		if err != nil {
			resp.Diagnostics.AddError("Error Loading Manifest", fmt.Sprintf("Failed to convert YAML to JSON: %s", err))
			return
		}

		localManifest = &hubv1.ProductManifest{}
		unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
		if err := unmarshaler.Unmarshal(jsonBytes, localManifest); err != nil {
			resp.Diagnostics.AddError("Error Loading Manifest", fmt.Sprintf("Failed to unmarshal JSON to Proto: %s", err))
			return
		}

		// Validate project consistency if project override not specified in HCL
		targetProject := ""
		if !data.Project.IsNull() && !data.Project.IsUnknown() && data.Project.ValueString() != "" {
			targetProject = data.Project.ValueString()
		}
		if targetProject == "" {
			for i, port := range localManifest.OutputPorts {
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
	}

	productUrn := ""
	if hasID {
		productUrn = data.ID.ValueString()
	} else if localManifest != nil && localManifest.Meta != nil {
		productUrn = localManifest.Meta.Urn
	}

	versionStr := ""
	if !data.Version.IsNull() && !data.Version.IsUnknown() {
		versionStr = data.Version.ValueString()
	}

	var activeManifest *hubv1.ProductManifest
	var activeContracts []*hubv1.DataContract

	if d.client != nil && d.client.HubClient != nil && productUrn != "" {
		res, err := d.client.HubClient.GetProduct(ctx, &hubv1.GetProductRequest{
			ProductUrn: productUrn,
			Version:    versionStr,
		})
		if err == nil && res != nil && res.Manifest != nil {
			activeManifest = res.Manifest
			activeContracts = res.Contracts
		} else if err != nil {
			if !hasManifest {
				resp.Diagnostics.AddError(
					"Error fetching product from Hub",
					fmt.Sprintf("Could not retrieve product %s: %s", productUrn, err),
				)
				return
			}
			// Fall back to localManifest when Hub is unreachable or NotFound
		}
	} else if !hasManifest {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the data source can be read, or provide a manifest_file.",
		)
		return
	}

	if activeManifest == nil {
		activeManifest = localManifest
	}

	if activeManifest == nil {
		resp.Diagnostics.AddError(
			"Product Manifest Not Available",
			"Could not resolve product manifest from Supercargo Hub or local manifest file.",
		)
		return
	}

	contractsMap := make(map[string]supercargoContractModel)
	if len(activeContracts) > 0 {
		for _, c := range activeContracts {
			if c == nil || c.Meta == nil {
				continue
			}
			schemaJSON, err := protoToSchemaJSON(c.Schema)
			if err != nil {
				schemaJSON = ""
			}
			contractsMap[c.Meta.Urn] = supercargoContractModel{
				ID:          types.StringValue(c.Meta.Urn),
				Schema:      types.StringValue(schemaJSON),
				Version:     types.StringValue(c.Meta.Version),
				ContentHash: types.StringValue(c.Meta.ContentHash),
				CommitSha:   types.StringValue(c.Meta.CommitSha),
			}
		}
	}

	// If Hub didn't return contracts and manifest_file is provided, resolve locally
	if len(contractsMap) == 0 && hasManifest && localManifest != nil {
		resolved, err := manifest.ResolveContracts(manifestPath, localManifest)
		if err == nil && len(resolved) > 0 {
			for k, rc := range resolved {
				contractsMap[k] = supercargoContractModel{
					ID:          types.StringValue(rc.URN),
					Schema:      types.StringValue(rc.SchemaJSON),
					Version:     types.StringValue(rc.Version),
					ContentHash: types.StringValue(rc.ContentHash),
					CommitSha:   types.StringValue(rc.CommitSha),
				}
			}
		}
	}

	if activeManifest.Meta != nil {
		if activeManifest.Meta.Urn != "" {
			productUrn = activeManifest.Meta.Urn
		}
		if activeManifest.Meta.Version != "" {
			data.Version = types.StringValue(activeManifest.Meta.Version)
		}
	}
	data.ID = types.StringValue(productUrn)
	data.URN = types.StringValue(productUrn)

	// Determine project, location, partitioning
	targetProject := ""
	if !data.Project.IsNull() && !data.Project.IsUnknown() && data.Project.ValueString() != "" {
		targetProject = data.Project.ValueString()
	}
	location := ""
	partitioningField := ""
	var partitionExpirationMs types.Int64 = types.Int64Null()

	for _, port := range activeManifest.OutputPorts {
		if port != nil && port.Physical != nil && port.Physical.Bigquery != nil {
			if targetProject == "" && port.Physical.Bigquery.Project != "" {
				targetProject = port.Physical.Bigquery.Project
			}
			if location == "" && port.Physical.Bigquery.Location != "" {
				location = port.Physical.Bigquery.Location
			}
			if partitioningField == "" && port.Physical.Bigquery.PartitionBy != "" {
				partitioningField = port.Physical.Bigquery.PartitionBy
			}
			if partitionExpirationMs.IsNull() && port.Physical.Bigquery.PartitionExpiration != nil {
				partitionExpirationMs = types.Int64Value(port.Physical.Bigquery.PartitionExpiration.AsDuration().Milliseconds())
			}
		}
	}

	if targetProject != "" {
		data.Project = types.StringValue(targetProject)
	} else {
		data.Project = types.StringNull()
	}

	if location != "" {
		data.Location = types.StringValue(location)
	} else {
		data.Location = types.StringNull()
	}

	if partitioningField != "" {
		data.PartitioningField = types.StringValue(partitioningField)
	} else {
		data.PartitioningField = types.StringNull()
	}

	data.PartitionExpirationMs = partitionExpirationMs

	// Service identities
	if targetProject != "" && productUrn != "" {
		identitiesMap, diags := calculateServiceIdentities(ctx, targetProject, productUrn)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.ServiceIdentities = identitiesMap
	} else {
		data.ServiceIdentities = types.MapNull(types.StringType)
	}

	// SLA
	if activeManifest.Sla != nil && (activeManifest.Sla.Tier != hubv1.SLATier_SLA_TIER_UNSPECIFIED || activeManifest.Sla.Rto != "" || activeManifest.Sla.Freshness != "" || activeManifest.Sla.Latency != "") {
		tierStr := activeManifest.Sla.Tier.String()
		data.SLA = &supercargoSLAModel{
			Tier:      types.StringValue(tierStr),
			Rto:       stringOrNull(activeManifest.Sla.Rto),
			Freshness: stringOrNull(activeManifest.Sla.Freshness),
			Latency:   stringOrNull(activeManifest.Sla.Latency),
		}
	} else {
		data.SLA = nil
	}

	// Contracts Map
	if len(contractsMap) > 0 {
		mapVal, diags := types.MapValueFrom(ctx, contractObjectType, contractsMap)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Contracts = mapVal
	} else {
		data.Contracts = types.MapNull(contractObjectType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func parseSLATier(tierStr string) (hubv1.SLATier, bool) {
	s := strings.ToUpper(strings.TrimSpace(tierStr))
	if val, ok := hubv1.SLATier_value[s]; ok {
		return hubv1.SLATier(val), true
	}
	if val, ok := hubv1.SLATier_value["SLA_TIER_"+s]; ok {
		return hubv1.SLATier(val), true
	}

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
