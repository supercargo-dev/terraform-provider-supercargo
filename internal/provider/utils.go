package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
)

const maxRecursionDepth = 100

// bqField represents a field in a BigQuery JSON schema.
type bqField struct {
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Mode        string     `json:"mode"`
	Description string     `json:"description,omitempty"`
	Fields      []*bqField `json:"fields,omitempty"`
}

// ContractFieldModel represents a flattened field for Terraform state.
type ContractFieldModel struct {
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Mode        types.String `tfsdk:"mode"`
	Description types.String `tfsdk:"description"`
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

func protoToBQFields(fields []*hubv1.Field, path []string) ([]*bqField, error) {
	if err := checkRecursionLimit(path); err != nil {
		return nil, err
	}
	out := make([]*bqField, 0, len(fields))
	for _, f := range fields {
		if f == nil {
			continue
		}
		bqType, err := mapProtoType(f.Type)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name, err)
		}

		bq := &bqField{
			Name:        f.Name,
			Type:        bqType,
			Mode:        mapProtoMode(f.Mode),
			Description: f.Description,
		}
		if len(f.Fields) > 0 {
			subFields, err := protoToBQFields(f.Fields, append(path, f.Name))
			if err != nil {
				return nil, err
			}
			bq.Fields = subFields
		}
		out = append(out, bq)
	}
	return out, nil
}

func flattenFields(fields []*hubv1.Field, prefix string, path []string) ([]ContractFieldModel, error) {
	if err := checkRecursionLimit(path); err != nil {
		return nil, err
	}
	result := make([]ContractFieldModel, 0, len(fields))
	for _, f := range fields {
		if f == nil {
			continue
		}
		name := f.Name
		if prefix != "" {
			name = prefix + "." + f.Name
		}

		bqType, err := mapProtoType(f.Type)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}

		result = append(result, ContractFieldModel{
			Name:        types.StringValue(name),
			Type:        types.StringValue(bqType),
			Mode:        types.StringValue(mapProtoMode(f.Mode)),
			Description: types.StringValue(f.Description),
		})
		if len(f.Fields) > 0 {
			subFields, err := flattenFields(f.Fields, name, append(path, f.Name))
			if err != nil {
				return nil, err
			}
			result = append(result, subFields...)
		}
	}
	return result, nil
}

func protoToSchemaJSON(fields []*hubv1.Field) (string, error) {
	bqFields, err := protoToBQFields(fields, []string{})
	if err != nil {
		return "", err
	}
	bytes, err := json.MarshalIndent(bqFields, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
