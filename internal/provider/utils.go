package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
)

const maxRecursionDepth = 100

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

func mapProtoType(t hubv1.DataType) (string, error) {
	switch t {
	case hubv1.DataType_DATA_TYPE_STRING:
		return "STRING", nil
	case hubv1.DataType_DATA_TYPE_BYTES:
		return "BYTES", nil
	case hubv1.DataType_DATA_TYPE_INT64:
		return "INT64", nil
	case hubv1.DataType_DATA_TYPE_FLOAT64:
		return "FLOAT64", nil
	case hubv1.DataType_DATA_TYPE_BOOL:
		return "BOOL", nil
	case hubv1.DataType_DATA_TYPE_TIMESTAMP:
		return "TIMESTAMP", nil
	case hubv1.DataType_DATA_TYPE_DATE:
		return "DATE", nil
	case hubv1.DataType_DATA_TYPE_TIME:
		return "TIME", nil
	case hubv1.DataType_DATA_TYPE_DATETIME:
		return "DATETIME", nil
	case hubv1.DataType_DATA_TYPE_GEOGRAPHY:
		return "GEOGRAPHY", nil
	case hubv1.DataType_DATA_TYPE_NUMERIC:
		return "NUMERIC", nil
	case hubv1.DataType_DATA_TYPE_BIGNUMERIC:
		return "BIGNUMERIC", nil
	case hubv1.DataType_DATA_TYPE_STRUCT:
		return "RECORD", nil
	case hubv1.DataType_DATA_TYPE_JSON:
		return "JSON", nil
	case hubv1.DataType_DATA_TYPE_VECTOR:
		return "FLOAT", nil
	case hubv1.DataType_DATA_TYPE_UNSPECIFIED:
		return "", fmt.Errorf("data type is unspecified")
	default:
		return "", fmt.Errorf("unsupported or unknown hubv1.DataType: %d", t)
	}
}

func mapProtoMode(m hubv1.FieldMode) string {
	switch m {
	case hubv1.FieldMode_FIELD_MODE_REQUIRED:
		return "REQUIRED"
	case hubv1.FieldMode_FIELD_MODE_REPEATED:
		return "REPEATED"
	case hubv1.FieldMode_FIELD_MODE_NULLABLE:
		return "NULLABLE"
	default:
		return "NULLABLE"
	}
}

func checkRecursionLimit(path []string) error {
	if len(path) > maxRecursionDepth {
		return fmt.Errorf("max recursion depth (%d) reached: %s", maxRecursionDepth, strings.Join(path, " -> "))
	}
	return nil
}

func flattenFields(fields []*hubv1.Field, prefix string, path []string) ([]ContractFieldModel, error) {
	return flattenFieldsWithOptions(fields, prefix, path, manifest.SchemaOptions{})
}

func flattenFieldsWithOptions(fields []*hubv1.Field, prefix string, path []string, opts manifest.SchemaOptions) ([]ContractFieldModel, error) {
	if len(path) == 0 {
		parsedEngine, err := manifest.ParseIngestionEngine(string(opts.IngestionEngine))
		if err != nil {
			return nil, err
		}
		opts.IngestionEngine = parsedEngine
	}
	if err := checkRecursionLimit(path); err != nil {
		return nil, err
	}
	result := make([]ContractFieldModel, 0, len(fields)+2)
	for _, f := range fields {
		if f == nil {
			continue
		}
		name := f.Name
		if prefix != "" {
			name = prefix + "." + f.Name
		}

		origMode := mapProtoMode(f.Mode)
		if opts.IngestionEngine == manifest.IngestionEngineDLT && origMode == "REPEATED" {
			result = append(result, ContractFieldModel{
				Name:        types.StringValue(name),
				Type:        types.StringValue("JSON"),
				Mode:        types.StringValue("NULLABLE"),
				Description: types.StringValue(f.Description),
			})
			continue
		}
		if f.Type == hubv1.DataType_DATA_TYPE_VECTOR {
			result = append(result, ContractFieldModel{
				Name:        types.StringValue(name),
				Type:        types.StringValue("FLOAT"),
				Mode:        types.StringValue("REPEATED"),
				Description: types.StringValue(f.Description),
			})
			continue
		}

		bqType, err := mapProtoType(f.Type)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}

		result = append(result, ContractFieldModel{
			Name:        types.StringValue(name),
			Type:        types.StringValue(bqType),
			Mode:        types.StringValue(origMode),
			Description: types.StringValue(f.Description),
		})
		if len(f.Fields) > 0 {
			subFields, err := flattenFieldsWithOptions(f.Fields, name, append(path, f.Name), opts)
			if err != nil {
				return nil, err
			}
			result = append(result, subFields...)
		}
	}

	if prefix == "" && len(path) == 0 && opts.IncludeDLTMetadata {
		hasLoadID := false
		hasID := false
		for _, f := range fields {
			if f == nil {
				continue
			}
			if strings.EqualFold(f.Name, "_dlt_load_id") {
				hasLoadID = true
			} else if strings.EqualFold(f.Name, "_dlt_id") {
				hasID = true
			}
		}
		if !hasLoadID {
			result = append(result, ContractFieldModel{
				Name:        types.StringValue("_dlt_load_id"),
				Type:        types.StringValue("STRING"),
				Mode:        types.StringValue("REQUIRED"),
				Description: types.StringValue("DLT load identifier"),
			})
		}
		if !hasID {
			result = append(result, ContractFieldModel{
				Name:        types.StringValue("_dlt_id"),
				Type:        types.StringValue("STRING"),
				Mode:        types.StringValue("REQUIRED"),
				Description: types.StringValue("DLT row identifier"),
			})
		}
	}
	return result, nil
}
