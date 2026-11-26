package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	IngestionEngine       types.String        `tfsdk:"ingestion_engine"`
	IncludeDLTMetadata    types.Bool          `tfsdk:"include_dlt_metadata"`
	Project               types.String        `tfsdk:"project"`
	URN                   types.String        `tfsdk:"urn"`
	Location              types.String        `tfsdk:"location"`
	PartitioningField     types.String        `tfsdk:"partitioning_field"`
	PartitionExpirationMs types.Int64         `tfsdk:"partition_expiration_ms"`
	ServiceIdentities     types.Map           `tfsdk:"service_identities"`
	Contracts             types.Map           `tfsdk:"contracts"`
	SLA                   *supercargoSLAModel `tfsdk:"sla"`
	LockDir               types.String        `tfsdk:"lock_dir"`
	InternalDatasetID     types.String        `tfsdk:"internal_dataset_id"`
	PublicDatasetID       types.String        `tfsdk:"public_dataset_id"`
	Outputs               types.Map           `tfsdk:"outputs"`
	Inputs                types.Map           `tfsdk:"inputs"`
	PAMEntitlements       types.Map           `tfsdk:"pam_entitlements"`
}

type supercargoOutputPortModel struct {
	Name                types.String `tfsdk:"name"`
	URN                 types.String `tfsdk:"urn"`
	Lens                types.String `tfsdk:"lens"`
	Materialization     types.String `tfsdk:"materialization"`
	ViewQuery           types.String `tfsdk:"view_query"`
	ViewSQL             types.String `tfsdk:"view_sql"`
	Sensitivity         types.String `tfsdk:"sensitivity"`
	AuthorizedRoles     types.List   `tfsdk:"authorized_roles"`
	FirstLevelApprovers types.List   `tfsdk:"first_level_approvers"`
}

var outputPortAttrTypes = map[string]attr.Type{
	"name":                  types.StringType,
	"urn":                   types.StringType,
	"lens":                  types.StringType,
	"materialization":       types.StringType,
	"view_query":            types.StringType,
	"view_sql":              types.StringType,
	"sensitivity":           types.StringType,
	"authorized_roles":      types.ListType{ElemType: types.StringType},
	"first_level_approvers": types.ListType{ElemType: types.StringType},
}

var outputPortObjectType = types.ObjectType{
	AttrTypes: outputPortAttrTypes,
}

type supercargoPAMEntitlementModel struct {
	EntitlementID       types.String `tfsdk:"entitlement_id"`
	PortName            types.String `tfsdk:"port_name"`
	PortURN             types.String `tfsdk:"port_urn"`
	Lens                types.String `tfsdk:"lens"`
	Sensitivity         types.String `tfsdk:"sensitivity"`
	AuthorizedRoles     types.List   `tfsdk:"authorized_roles"`
	FirstLevelApprovers types.List   `tfsdk:"first_level_approvers"`
	RequiresPAM         types.Bool   `tfsdk:"requires_pam"`
	RequiresKMSDecrypt  types.Bool   `tfsdk:"requires_kms_decrypt"`
	TermsPurpose        types.String `tfsdk:"terms_purpose"`
	TermsUsage          types.String `tfsdk:"terms_usage"`
	TermsLimitations    types.String `tfsdk:"terms_limitations"`
}

var pamEntitlementAttrTypes = map[string]attr.Type{
	"entitlement_id":        types.StringType,
	"port_name":             types.StringType,
	"port_urn":              types.StringType,
	"lens":                  types.StringType,
	"sensitivity":           types.StringType,
	"authorized_roles":      types.ListType{ElemType: types.StringType},
	"first_level_approvers": types.ListType{ElemType: types.StringType},
	"requires_pam":          types.BoolType,
	"requires_kms_decrypt":  types.BoolType,
	"terms_purpose":         types.StringType,
	"terms_usage":           types.StringType,
	"terms_limitations":     types.StringType,
}

var pamEntitlementObjectType = types.ObjectType{
	AttrTypes: pamEntitlementAttrTypes,
}

type supercargoInputPortModel struct {
	Name       types.String `tfsdk:"name"`
	URN        types.String `tfsdk:"urn"`
	RecordName types.String `tfsdk:"record_name"`
	TableName  types.String `tfsdk:"table_name"`
	Source     types.String `tfsdk:"source"`
	SchemaJSON types.String `tfsdk:"schema_json"`
}

var inputPortAttrTypes = map[string]attr.Type{
	"name":        types.StringType,
	"urn":         types.StringType,
	"record_name": types.StringType,
	"table_name":  types.StringType,
	"source":      types.StringType,
	"schema_json": types.StringType,
}

var inputPortObjectType = types.ObjectType{
	AttrTypes: inputPortAttrTypes,
}

type supercargoSLAModel struct {
	Tier      types.String `tfsdk:"tier"`
	Rto       types.String `tfsdk:"rto"`
	Freshness types.String `tfsdk:"freshness"`
	Latency   types.String `tfsdk:"latency"`
}

type supercargoContractModel struct {
	ID          types.String `tfsdk:"id"`
	SchemaJSON  types.String `tfsdk:"schema_json"`
	Version     types.String `tfsdk:"version"`
	ContentHash types.String `tfsdk:"content_hash"`
	CommitSha   types.String `tfsdk:"commit_sha"`
}

var contractAttrTypes = map[string]attr.Type{
	"id":           types.StringType,
	"schema_json":  types.StringType,
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
			"ingestion_engine": schema.StringAttribute{
				MarkdownDescription: "BigQuery ingestion engine override: 'streaming' (default) or 'dlt' (maps repeated fields to JSON NULLABLE). If omitted, inferred per port from manifest and contract metadata.",
				Optional:            true,
			},
			"include_dlt_metadata": schema.BoolAttribute{
				MarkdownDescription: "Whether to append _dlt_load_id and _dlt_id STRING REQUIRED columns to generated contract schemas. Defaults to false.",
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
						"schema_json": schema.StringAttribute{
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
			"lock_dir": schema.StringAttribute{
				MarkdownDescription: "Optional explicit path to the compiled .supercargo/lock directory. If omitted, auto-discovered.",
				Optional:            true,
				Computed:            true,
			},
			"internal_dataset_id": schema.StringAttribute{
				MarkdownDescription: "BigQuery dataset ID for private internal landing tables.",
				Computed:            true,
			},
			"public_dataset_id": schema.StringAttribute{
				MarkdownDescription: "BigQuery dataset ID for public authorized views.",
				Computed:            true,
			},
			"outputs": schema.MapNestedAttribute{
				MarkdownDescription: "Map of compiled output views keyed by contract URN or port name.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Output port name.",
							Computed:            true,
						},
						"urn": schema.StringAttribute{
							MarkdownDescription: "Output contract URN.",
							Computed:            true,
						},
						"lens": schema.StringAttribute{
							MarkdownDescription: "Relational lens applied (e.g. replica, event_log, scd2, cdc, reidentified).",
							Computed:            true,
						},
						"materialization": schema.StringAttribute{
							MarkdownDescription: "Materialization type (view or table).",
							Computed:            true,
						},
						"view_query": schema.StringAttribute{
							MarkdownDescription: "Unwrapped GoogleSQL query body for authorized view definition.",
							Computed:            true,
						},
						"view_sql": schema.StringAttribute{
							MarkdownDescription: "Full GoogleSQL CREATE VIEW DDL statement.",
							Computed:            true,
						},
						"sensitivity": schema.StringAttribute{
							MarkdownDescription: "Resolved maximum sensitivity tier (PUBLIC, INTERNAL, CONFIDENTIAL, RESTRICTED).",
							Computed:            true,
						},
						"authorized_roles": schema.ListAttribute{
							MarkdownDescription: "Producer-declared governance roles authorized to access this output port.",
							ElementType:         types.StringType,
							Computed:            true,
						},
						"first_level_approvers": schema.ListAttribute{
							MarkdownDescription: "Producer governance approver team names from ODCS roles[].firstLevelApprovers.",
							ElementType:         types.StringType,
							Computed:            true,
						},
					},
				},
			},
			"pam_entitlements": schema.MapNestedAttribute{
				MarkdownDescription: "Deterministic plan-time GCP PAM entitlement metadata keyed by output port slug (includes both view-backed and transform-backed outputs).",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"entitlement_id": schema.StringAttribute{
							MarkdownDescription: "Sanitized GCP PAM entitlement ID matching core/pkg/validation.SanitizePAMIdentifier.",
							Computed:            true,
						},
						"port_name": schema.StringAttribute{
							MarkdownDescription: "Output port slug.",
							Computed:            true,
						},
						"port_urn": schema.StringAttribute{
							MarkdownDescription: "Output contract URN.",
							Computed:            true,
						},
						"lens": schema.StringAttribute{
							MarkdownDescription: "Relational lens applied (if any).",
							Computed:            true,
						},
						"sensitivity": schema.StringAttribute{
							MarkdownDescription: "Resolved maximum sensitivity tier.",
							Computed:            true,
						},
						"authorized_roles": schema.ListAttribute{
							MarkdownDescription: "Producer-declared governance roles.",
							ElementType:         types.StringType,
							Computed:            true,
						},
						"first_level_approvers": schema.ListAttribute{
							MarkdownDescription: "Producer governance approver team names.",
							ElementType:         types.StringType,
							Computed:            true,
						},
						"requires_pam": schema.BoolAttribute{
							MarkdownDescription: "True when lens == 'reidentified', sensitivity is CONFIDENTIAL/RESTRICTED, or authorized_roles is non-empty.",
							Computed:            true,
						},
						"requires_kms_decrypt": schema.BoolAttribute{
							MarkdownDescription: "True when lens == 'reidentified'.",
							Computed:            true,
						},
						"terms_purpose": schema.StringAttribute{
							MarkdownDescription: "ODCS description.purpose / terms_of_use.purpose.",
							Computed:            true,
						},
						"terms_usage": schema.StringAttribute{
							MarkdownDescription: "ODCS description.usage / terms_of_use.usage.",
							Computed:            true,
						},
						"terms_limitations": schema.StringAttribute{
							MarkdownDescription: "ODCS description.limitations / terms_of_use.limitations.",
							Computed:            true,
						},
					},
				},
			},
			"inputs": schema.MapNestedAttribute{
				MarkdownDescription: "Map of compiled boundary input landing tables keyed by contract URN or port name.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Input port name.",
							Computed:            true,
						},
						"urn": schema.StringAttribute{
							MarkdownDescription: "Internal ingress contract URN.",
							Computed:            true,
						},
						"record_name": schema.StringAttribute{
							MarkdownDescription: "Input record name.",
							Computed:            true,
						},
						"table_name": schema.StringAttribute{
							MarkdownDescription: "BigQuery landing table name in the internal dataset.",
							Computed:            true,
						},
						"source": schema.StringAttribute{
							MarkdownDescription: "Ingress data source URI (e.g. pubsub://managed, dlt://...).",
							Computed:            true,
						},
						"schema_json": schema.StringAttribute{
							MarkdownDescription: "BigQuery physical schema JSON for base landing table with policy tags.",
							Computed:            true,
						},
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

	explicitOpts, optDiags := parseProviderSchemaOptions(data.IngestionEngine, data.IncludeDLTMetadata)
	resp.Diagnostics.Append(optDiags...)
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
		var err error
		localManifest, err = manifest.Load(manifestPath)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "invalid sla tier") {
				resp.Diagnostics.AddError("Invalid SLA Tier", err.Error())
				return
			}
			resp.Diagnostics.AddError("Error Loading Manifest", fmt.Sprintf("Failed to load manifest file %q: %s", manifestPath, err))
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
			st, ok := status.FromError(err)
			if ok && (st.Code() == codes.PermissionDenied || st.Code() == codes.Unauthenticated) {
				resp.Diagnostics.AddError(
					"Error Reading Data Product",
					fmt.Sprintf("Could not retrieve product %s: %s\n\nRemediation Hint: Verify that your identity token is valid and has permissions to access this data product.", productUrn, maskBearerTokens(err.Error())),
				)
				return
			}
			if !hasManifest {
				resp.Diagnostics.AddError(
					"Error Reading Data Product",
					fmt.Sprintf("Could not retrieve product %s: %s\n\nRemediation Hint: Ensure 'sc apply' has been executed to publish the product to Supercargo Hub, or specify 'manifest_file' in the data source to allow local schema resolution.", productUrn, maskBearerTokens(err.Error())),
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

	var manifestExtras manifest.ManifestExtras
	if hasManifest {
		manifestExtras = manifest.LoadManifestExtras(manifestPath)
	}

	contractsMap := make(map[string]supercargoContractModel, len(activeContracts))
	if len(activeContracts) > 0 {
		outPortByContractURN := make(map[string]*hubv1.OutputPort, len(activeManifest.OutputPorts))
		for _, op := range activeManifest.OutputPorts {
			if op != nil && op.Contract != nil && op.Contract.Urn != "" {
				if _, exists := outPortByContractURN[op.Contract.Urn]; !exists {
					outPortByContractURN[op.Contract.Urn] = op
				}
			}
		}
		inPortByContractURN := make(map[string]*hubv1.InputPort, len(activeManifest.InputPorts))
		for _, ip := range activeManifest.InputPorts {
			if ip == nil {
				continue
			}
			inURN := ip.ContractUrn
			if ip.Contract != nil && ip.Contract.Urn != "" {
				inURN = ip.Contract.Urn
			}
			if inURN != "" {
				if _, exists := inPortByContractURN[inURN]; !exists {
					inPortByContractURN[inURN] = ip
				}
			}
		}

		for _, c := range activeContracts {
			if c == nil || c.Meta == nil {
				continue
			}
			outPort := outPortByContractURN[c.Meta.Urn]
			inPort := inPortByContractURN[c.Meta.Urn]
			portOpts, err := manifest.ResolvePortSchemaOptions(activeManifest, outPort, inPort, c.Meta.Labels, c.Meta.Urn, explicitOpts, manifestExtras)
			if err != nil {
				resp.Diagnostics.AddError(
					"Invalid Ingestion Engine",
					fmt.Sprintf("Contract %s: %s\n\nRemediation Hint: Set ingestion_engine (or supercargo.ingestion_engine label) to 'streaming' or 'dlt'.", c.Meta.Urn, maskBearerTokens(err.Error())),
				)
				return
			}
			schemaJSON, err := manifest.ProtoFieldsToBigQueryJSON(c.Schema, portOpts)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error Translating Contract Schema",
					fmt.Sprintf("Contract %s: %s", c.Meta.Urn, maskBearerTokens(err.Error())),
				)
				return
			}
			contractsMap[c.Meta.Urn] = supercargoContractModel{
				ID:          types.StringValue(c.Meta.Urn),
				SchemaJSON:  types.StringValue(schemaJSON),
				Version:     types.StringValue(c.Meta.Version),
				ContentHash: types.StringValue(c.Meta.ContentHash),
				CommitSha:   types.StringValue(c.Meta.CommitSha),
			}
		}
	}

	// If Hub didn't return contracts and manifest_file is provided, resolve locally
	if len(contractsMap) == 0 && hasManifest && localManifest != nil {
		resolved, err := manifest.ResolveContractsWithOptions(manifestPath, localManifest, explicitOpts)
		if err != nil {
			if !errors.Is(err, manifest.ErrMissingContractAndSchema) {
				resp.Diagnostics.AddError(
					"Error Resolving Local Contracts",
					maskBearerTokens(err.Error()),
				)
				return
			}
		} else if len(resolved) > 0 {
			for k, rc := range resolved {
				contractsMap[k] = supercargoContractModel{
					ID:          types.StringValue(rc.URN),
					SchemaJSON:  types.StringValue(rc.SchemaJSON),
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

	// Derive product slug / ID
	productSlug := productUrn
	if idx := strings.LastIndex(productUrn, ":"); idx >= 0 && idx < len(productUrn)-1 {
		productSlug = productUrn[idx+1:]
	}
	productSlug = strings.ReplaceAll(productSlug, "-", "_")
	data.PublicDatasetID = types.StringValue(productSlug)
	data.InternalDatasetID = types.StringValue(productSlug + "_internal")

	// Lock Directory handling
	explicitLockDir := ""
	if !data.LockDir.IsNull() && !data.LockDir.IsUnknown() && data.LockDir.ValueString() != "" {
		explicitLockDir = data.LockDir.ValueString()
	}

	discoveredLockDir, foundLock, lockErr := manifest.DiscoverLockDir(manifestPath, productSlug, explicitLockDir)
	if explicitLockDir != "" && (!foundLock || lockErr != nil) {
		resp.Diagnostics.AddError(
			"Explicitly Configured Lock Directory Missing",
			fmt.Sprintf("Explicitly configured lock_dir '%s' does not exist\n\nRemediation Hint: Run 'sc build --output-dir %s' to compile relational lenses, schemas, and views before planning infrastructure, or verify the directory path.", explicitLockDir, explicitLockDir),
		)
		return
	}

	outputsMap := make(map[string]supercargoOutputPortModel)
	inputsMap := make(map[string]supercargoInputPortModel)
	pamEntitlementsMap := make(map[string]supercargoPAMEntitlementModel)

	if foundLock && discoveredLockDir != "" {
		data.LockDir = types.StringValue(discoveredLockDir)

		compiledOutputs, err := manifest.LoadCompiledOutputs(discoveredLockDir, productSlug)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate collision-resistant collection key") {
				resp.Diagnostics.AddError(
					"Duplicate Collection Key",
					fmt.Sprintf("Error: %s\n\nRemediation Hint: Output and input port names must be unique within the data product manifest.", err),
				)
				return
			}
			resp.Diagnostics.AddError("Error Loading Compiled Outputs", fmt.Sprintf("Failed to load compiled outputs from lock directory %q: %s", discoveredLockDir, maskBearerTokens(err.Error())))
			return
		}

		for k, out := range compiledOutputs {
			mat := strings.ToLower(strings.TrimSpace(out.Materialization))
			if mat == "" {
				mat = "view"
			}
			if mat != "view" && mat != "table" {
				resp.Diagnostics.AddError(
					"Invalid Materialization",
					fmt.Sprintf("Error: Unsupported materialization '%s' on output port '%s'\n\nRemediation Hint: Valid materialization types are 'view' (default) or 'table'. Check 'materialization:' in supercargo.yaml.", out.Materialization, out.Name),
				)
				return
			}
			rolesList, listDiags := stringSliceToList(ctx, out.AuthorizedRoles)
			resp.Diagnostics.Append(listDiags...)
			if resp.Diagnostics.HasError() {
				return
			}
			approversList, listDiags := stringSliceToList(ctx, out.FirstLevelApprovers)
			resp.Diagnostics.Append(listDiags...)
			if resp.Diagnostics.HasError() {
				return
			}
			outputsMap[k] = supercargoOutputPortModel{
				Name:                types.StringValue(out.Name),
				URN:                 types.StringValue(out.URN),
				Lens:                types.StringValue(out.Lens),
				Materialization:     types.StringValue(mat),
				ViewQuery:           types.StringValue(out.ViewQuery),
				ViewSQL:             types.StringValue(out.ViewSQL),
				Sensitivity:         types.StringValue(out.Sensitivity),
				AuthorizedRoles:     rolesList,
				FirstLevelApprovers: approversList,
			}
		}

		compiledPAM, err := manifest.LoadCompiledPAMEntitlements(discoveredLockDir, productSlug)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Loading Compiled PAM Entitlements",
				fmt.Sprintf("Failed to load compiled PAM entitlements from lock directory %q: %s", discoveredLockDir, maskBearerTokens(err.Error())),
			)
			return
		}

		for k, ent := range compiledPAM {
			rolesList, listDiags := stringSliceToList(ctx, ent.AuthorizedRoles)
			resp.Diagnostics.Append(listDiags...)
			if resp.Diagnostics.HasError() {
				return
			}
			approversList, listDiags := stringSliceToList(ctx, ent.FirstLevelApprovers)
			resp.Diagnostics.Append(listDiags...)
			if resp.Diagnostics.HasError() {
				return
			}
			pamEntitlementsMap[k] = supercargoPAMEntitlementModel{
				EntitlementID:       types.StringValue(ent.EntitlementID),
				PortName:            types.StringValue(ent.PortName),
				PortURN:             types.StringValue(ent.PortURN),
				Lens:                types.StringValue(ent.Lens),
				Sensitivity:         types.StringValue(ent.Sensitivity),
				AuthorizedRoles:     rolesList,
				FirstLevelApprovers: approversList,
				RequiresPAM:         types.BoolValue(ent.RequiresPAM),
				RequiresKMSDecrypt:  types.BoolValue(ent.RequiresKMSDecrypt),
				TermsPurpose:        types.StringValue(ent.TermsPurpose),
				TermsUsage:          types.StringValue(ent.TermsUsage),
				TermsLimitations:    types.StringValue(ent.TermsLimitations),
			}
		}

		compiledInputs, err := manifest.LoadCompiledInputs(discoveredLockDir, productSlug)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate collision-resistant collection key") {
				resp.Diagnostics.AddError(
					"Duplicate Collection Key",
					fmt.Sprintf("Error: %s\n\nRemediation Hint: Output and input port names must be unique within the data product manifest.", err),
				)
				return
			}
			resp.Diagnostics.AddError("Error Loading Compiled Inputs", fmt.Sprintf("Failed to load compiled inputs from lock directory %q: %s", discoveredLockDir, maskBearerTokens(err.Error())))
			return
		}

		for k, in := range compiledInputs {
			inputsMap[k] = supercargoInputPortModel{
				Name:       types.StringValue(in.Name),
				URN:        types.StringValue(in.URN),
				RecordName: types.StringValue(in.RecordName),
				TableName:  types.StringValue(in.TableName),
				Source:     types.StringValue(in.Source),
				SchemaJSON: types.StringValue(in.SchemaJSON),
			}
		}
	} else {
		data.LockDir = types.StringNull()
	}

	outputsVal, diags := types.MapValueFrom(ctx, outputPortObjectType, outputsMap)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Outputs = outputsVal

	inputsVal, diags := types.MapValueFrom(ctx, inputPortObjectType, inputsMap)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Inputs = inputsVal

	pamVal, diags := types.MapValueFrom(ctx, pamEntitlementObjectType, pamEntitlementsMap)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.PAMEntitlements = pamVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func stringSliceToList(ctx context.Context, items []string) (types.List, diag.Diagnostics) {
	if len(items) == 0 {
		return types.ListValueFrom(ctx, types.StringType, []string{})
	}
	return types.ListValueFrom(ctx, types.StringType, items)
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
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
