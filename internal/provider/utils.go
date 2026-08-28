package provider

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
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

type schemaFieldJSON struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Mode        string            `json:"mode"`
	Description string            `json:"description,omitempty"`
	Constraints map[string]string `json:"constraints,omitempty"`
	Fields      []schemaFieldJSON `json:"fields,omitempty"`
}

func parseSchemaJSONToFields(schemaJSON string) ([]*hubv1.Field, error) {
	if strings.TrimSpace(schemaJSON) == "" {
		return nil, nil
	}
	var fields []schemaFieldJSON
	if err := json.Unmarshal([]byte(schemaJSON), &fields); err != nil {
		return nil, fmt.Errorf("failed to parse schema JSON: %w", err)
	}
	return mapSchemaJSONFieldsToProto(fields, 0, []string{})
}

func mapSchemaJSONFieldsToProto(fs []schemaFieldJSON, depth int, path []string) ([]*hubv1.Field, error) {
	if err := checkRecursionLimit(path); err != nil {
		return nil, err
	}
	protoFields := make([]*hubv1.Field, len(fs))
	for i, f := range fs {
		var dataType hubv1.DataType
		switch strings.ToUpper(strings.TrimSpace(f.Type)) {
		case "STRING":
			dataType = hubv1.DataType_DATA_TYPE_STRING
		case "INT64", "INTEGER", "INT":
			dataType = hubv1.DataType_DATA_TYPE_INT64
		case "FLOAT64", "FLOAT":
			dataType = hubv1.DataType_DATA_TYPE_FLOAT64
		case "BOOL", "BOOLEAN":
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
		switch strings.ToUpper(strings.TrimSpace(f.Mode)) {
		case "REQUIRED":
			fieldMode = hubv1.FieldMode_FIELD_MODE_REQUIRED
		case "REPEATED":
			fieldMode = hubv1.FieldMode_FIELD_MODE_REPEATED
		}

		subFields, err := mapSchemaJSONFieldsToProto(f.Fields, depth+1, append(path, f.Name))
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

func resolvedContractToProto(rc *manifest.ResolvedContract) (*hubv1.DataContract, error) {
	if rc == nil {
		return nil, nil
	}
	fields, err := parseSchemaJSONToFields(rc.SchemaJSON)
	if err != nil {
		return nil, err
	}
	return &hubv1.DataContract{
		Meta: &hubv1.Meta{
			Urn:         rc.URN,
			Version:     rc.Version,
			ContentHash: rc.ContentHash,
			CommitSha:   rc.CommitSha,
			DataAsset:   rc.DataAsset,
		},
		Schema: fields,
	}, nil
}
