package provider

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/protobuf/proto"
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
	out := make([]*bqField, 0)
	for _, f := range fields {
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
	result := make([]ContractFieldModel, 0)
	for _, f := range fields {
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

func mapConstraints(cs map[string]string) (*hubv1.Constraints, error) {
	if cs == nil {
		return nil, nil
	}
	out := &hubv1.Constraints{}
	for k, v := range cs {
		switch k {
		case "greater_than":
			out.GreaterThan = proto.String(v)
		case "greater_than_or_equal_to":
			out.GreaterThanOrEqualTo = proto.String(v)
		case "less_than":
			out.LessThan = proto.String(v)
		case "less_than_or_equal_to":
			out.LessThanOrEqualTo = proto.String(v)
		case "is_null":
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("invalid boolean for is_null: %w", err)
			}
			out.IsNull = b
		case "is_not_empty":
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("invalid boolean for is_not_empty: %w", err)
			}
			out.IsNotEmpty = b
		case "length":
			iv, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid integer for length: %w", err)
			}
			out.Length = int32(iv)
		case "min_value":
			iv, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid integer for min_value: %w", err)
			}
			out.MinValue = proto.Int32(int32(iv))
		case "max_value":
			iv, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid integer for max_value: %w", err)
			}
			out.MaxValue = proto.Int32(int32(iv))
		case "min_length":
			iv, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid integer for min_length: %w", err)
			}
			out.MinLength = proto.Int32(int32(iv))
		case "max_length":
			iv, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("invalid integer for max_length: %w", err)
			}
			out.MaxLength = proto.Int32(int32(iv))
		case "enum":
			if v != "" {
				parts := strings.Split(v, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				out.Enum = parts
			}
		case "pattern":
			out.Pattern = v
		case "cel_expression":
			out.CelExpression = v
		}
	}
	return out, nil
}
