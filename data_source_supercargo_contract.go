package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
)

var _ datasource.DataSource = &SupercargoContractDataSource{}
var _ datasource.DataSourceWithConfigure = &SupercargoContractDataSource{}

func NewSupercargoContractDataSource() datasource.DataSource {
	return &SupercargoContractDataSource{}
}

type SupercargoContractDataSource struct {
	client *ProviderData
}

type SupercargoContractDataSourceModel struct {
	ID         types.String         `tfsdk:"id"`
	Version    types.String         `tfsdk:"version"`
	URN        types.String         `tfsdk:"urn"`
	OwnerTeam  types.String         `tfsdk:"owner_team"`
	SchemaJSON types.String         `tfsdk:"schema_json"`
	Fields     []ContractFieldModel `tfsdk:"fields"`
}

func (d *SupercargoContractDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contract"
}

func (d *SupercargoContractDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The URN of the Data Contract.",
				Required:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "The version of the Data Contract. If omitted, the latest active version will be retrieved.",
				Optional:            true,
				Computed:            true,
			},
			"urn": schema.StringAttribute{
				MarkdownDescription: "The exact URN of the contract.",
				Computed:            true,
			},
			"owner_team": schema.StringAttribute{
				MarkdownDescription: "The team that owns this contract.",
				Computed:            true,
			},
			"schema_json": schema.StringAttribute{
				MarkdownDescription: "The generated BigQuery JSON schema definition.",
				Computed:            true,
			},
			"fields": schema.ListNestedAttribute{
				MarkdownDescription: "The flattened schema fields of the contract.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed: true,
						},
						"type": schema.StringAttribute{
							Computed: true,
						},
						"mode": schema.StringAttribute{
							Computed: true,
						},
						"description": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *SupercargoContractDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SupercargoContractDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The provider must be configured before the data source can be used.",
		)
		return
	}

	var data SupercargoContractDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Fetch from Hub using gRPC client
	version := ""
	if !data.Version.IsNull() && !data.Version.IsUnknown() {
		version = data.Version.ValueString()
	}

	hubResp, err := d.client.HubClient.GetContract(ctx, &hubv1.GetContractRequest{
		ContractUrn: data.ID.ValueString(),
		Version:     version,
	})

	if err != nil {
		v := version
		if v == "" {
			v = "latest"
		}
		resp.Diagnostics.AddError(
			"Error fetching contract from Hub",
			fmt.Sprintf("Could not retrieve contract %s (version: %s): %s", data.ID.ValueString(), v, err),
		)
		return
	}

	if hubResp == nil || hubResp.Contract == nil {
		resp.Diagnostics.AddError(
			"Contract not found",
			fmt.Sprintf("Hub returned an empty response for %s", data.ID.ValueString()),
		)
		return
	}

	contract := hubResp.Contract

	// Populate metadata
	if contract.Meta == nil {
		resp.Diagnostics.AddError(
			"Invalid Contract Metadata",
			"The contract returned from the Hub is missing metadata.",
		)
		return
	}
	data.URN = types.StringValue(contract.Meta.Urn)
	data.Version = types.StringValue(contract.Meta.Version)
	data.OwnerTeam = types.StringValue(contract.Meta.OwnerTeam)

	// Translate to BigQuery JSON
	bqFields, err := protoToBQFields(contract.Schema, []string{})
	if err != nil {
		resp.Diagnostics.AddError("Error translating schema to BigQuery format", err.Error())
		return
	}
	schemaBytes, err := json.MarshalIndent(bqFields, "", "  ")
	if err != nil {
		resp.Diagnostics.AddError("Error marshaling BigQuery schema", err.Error())
		return
	}
	data.SchemaJSON = types.StringValue(string(schemaBytes))

	// Populate flattened fields list
	fields, err := flattenFields(contract.Schema, "", []string{})
	if err != nil {
		resp.Diagnostics.AddError("Error flattening schema fields", err.Error())
		return
	}
	data.Fields = fields

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
