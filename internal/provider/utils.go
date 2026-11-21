package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
)

// ContractFieldModel represents a flattened field for Terraform state.
type ContractFieldModel struct {
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Mode        types.String `tfsdk:"mode"`
	Description types.String `tfsdk:"description"`
}

func maskBearerTokens(msg string) string {
	return manifest.ScrubBearerTokens(msg)
}

func parseProviderSchemaOptions(ingestionEngineAttr types.String, includeDLTMetadataAttr types.Bool) (manifest.SchemaOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	var opts manifest.SchemaOptions

	if !ingestionEngineAttr.IsNull() && !ingestionEngineAttr.IsUnknown() {
		raw := ingestionEngineAttr.ValueString()
		parsed, err := manifest.ParseIngestionEngine(raw)
		if err != nil {
			diags.AddError(
				"Invalid Ingestion Engine",
				fmt.Sprintf("%s\n\nRemediation Hint: Set ingestion_engine to 'streaming' (default) or 'dlt'.", maskBearerTokens(err.Error())),
			)
			return opts, diags
		}
		opts.IngestionEngine = parsed
	}
	if !includeDLTMetadataAttr.IsNull() && !includeDLTMetadataAttr.IsUnknown() {
		opts.IncludeDLTMetadata = includeDLTMetadataAttr.ValueBool()
	}
	return opts, diags
}

func flattenFieldsWithOptions(fields []*hubv1.Field, opts manifest.SchemaOptions) ([]ContractFieldModel, error) {
	bqFields, err := manifest.ProtoFieldsToBigQueryFields(fields, opts)
	if err != nil {
		return nil, err
	}
	return flattenBQFields(bqFields, ""), nil
}

func flattenBQFields(bqFields []manifest.BQFieldJSON, prefix string) []ContractFieldModel {
	result := make([]ContractFieldModel, 0, len(bqFields))
	for _, f := range bqFields {
		name := f.Name
		if prefix != "" {
			name = prefix + "." + f.Name
		}
		result = append(result, ContractFieldModel{
			Name:        types.StringValue(name),
			Type:        types.StringValue(f.Type),
			Mode:        types.StringValue(f.Mode),
			Description: types.StringValue(f.Description),
		})
		if len(f.Fields) > 0 {
			result = append(result, flattenBQFields(f.Fields, name)...)
		}
	}
	return result
}
