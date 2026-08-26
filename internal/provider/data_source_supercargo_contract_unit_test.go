package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSupercargoContractDataSource_Metadata(t *testing.T) {
	d := NewSupercargoContractDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "supercargo"}
	var resp datasource.MetadataResponse

	d.Metadata(context.Background(), req, &resp)
	assert.Equal(t, "supercargo_contract", resp.TypeName)
}

func TestSupercargoContractDataSource_Schema(t *testing.T) {
	d := NewSupercargoContractDataSource()
	req := datasource.SchemaRequest{}
	var resp datasource.SchemaResponse

	d.Schema(context.Background(), req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "schema errors: %v", resp.Diagnostics)

	assert.NotNil(t, resp.Schema.Attributes["id"])
	assert.True(t, resp.Schema.Attributes["id"].IsRequired())

	assert.NotNil(t, resp.Schema.Attributes["version"])
	assert.True(t, resp.Schema.Attributes["version"].IsOptional())
	assert.True(t, resp.Schema.Attributes["version"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["urn"])
	assert.True(t, resp.Schema.Attributes["urn"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["owner_team"])
	assert.True(t, resp.Schema.Attributes["owner_team"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["schema_json"])
	assert.True(t, resp.Schema.Attributes["schema_json"].IsComputed())

	assert.NotNil(t, resp.Schema.Attributes["fields"])
	assert.True(t, resp.Schema.Attributes["fields"].IsComputed())
}

func TestSupercargoContractDataSource_Configure(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider data", func(t *testing.T) {
		d := &supercargoContractDataSource{}
		var resp datasource.ConfigureResponse
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: nil}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Nil(t, d.client)
	})

	t.Run("invalid provider data type", func(t *testing.T) {
		d := &supercargoContractDataSource{}
		var resp datasource.ConfigureResponse
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: 12345}, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Unexpected Data Source Configure Data Type")
		assert.Nil(t, d.client)
	})

	t.Run("valid provider data", func(t *testing.T) {
		_, providerData, _ := startMockHubServer(t)
		d := &supercargoContractDataSource{}
		var resp datasource.ConfigureResponse
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &resp)
		assert.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, providerData, d.client)
	})
}

func TestSupercargoContractDataSource_Read_Success_LosslessTypes(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var dsSchemaResp datasource.SchemaResponse
	d := &supercargoContractDataSource{client: providerData}
	d.Schema(ctx, datasource.SchemaRequest{}, &dsSchemaResp)

	allTypesSchema := []*hubv1.Field{
		{Name: "field_string", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED, Description: "string field"},
		{Name: "field_bytes", Type: hubv1.DataType_DATA_TYPE_BYTES, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "bytes field"},
		{Name: "field_int64", Type: hubv1.DataType_DATA_TYPE_INT64, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED, Description: "int64 field"},
		{Name: "field_float64", Type: hubv1.DataType_DATA_TYPE_FLOAT64, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "float64 field"},
		{Name: "field_bool", Type: hubv1.DataType_DATA_TYPE_BOOL, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED, Description: "bool field"},
		{Name: "field_timestamp", Type: hubv1.DataType_DATA_TYPE_TIMESTAMP, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "timestamp field"},
		{Name: "field_date", Type: hubv1.DataType_DATA_TYPE_DATE, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "date field"},
		{Name: "field_time", Type: hubv1.DataType_DATA_TYPE_TIME, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "time field"},
		{Name: "field_datetime", Type: hubv1.DataType_DATA_TYPE_DATETIME, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "datetime field"},
		{Name: "field_geography", Type: hubv1.DataType_DATA_TYPE_GEOGRAPHY, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "geography field"},
		{Name: "field_numeric", Type: hubv1.DataType_DATA_TYPE_NUMERIC, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "numeric field"},
		{Name: "field_bignumeric", Type: hubv1.DataType_DATA_TYPE_BIGNUMERIC, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "bignumeric field"},
		{Name: "field_json", Type: hubv1.DataType_DATA_TYPE_JSON, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "json field"},
		{
			Name:        "field_record",
			Type:        hubv1.DataType_DATA_TYPE_STRUCT,
			Mode:        hubv1.FieldMode_FIELD_MODE_REPEATED,
			Description: "nested record field",
			Fields: []*hubv1.Field{
				{Name: "child_str", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED, Description: "child string"},
				{Name: "child_int", Type: hubv1.DataType_DATA_TYPE_INT64, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "child int64"},
			},
		},
	}

	mockSrv.mu.Lock()
	mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
		assert.Equal(t, "urn:supercargo:hub:contract:com.example.events", req.ContractUrn)
		assert.Equal(t, "v1.2.0", req.Version)
		return &hubv1.GetContractResponse{
			Contract: &hubv1.DataContract{
				Meta: &hubv1.Meta{
					Urn:         "urn:supercargo:hub:contract:com.example.events",
					Version:     "v1.2.0",
					OwnerTeam:   "analytics",
					DataAsset:   "go://github.com/supercargo-dev/core/events",
					CommitSha:   "abcdef0123456789abcdef0123456789abcdef01",
					ContentHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				},
				Schema: allTypesSchema,
			},
		}, nil
	}
	mockSrv.mu.Unlock()

	cfgModel := supercargoContractDataSourceModel{
		ID:      types.StringValue("urn:supercargo:hub:contract:com.example.events"),
		Version: types.StringValue("v1.2.0"),
	}

	req := datasource.ReadRequest{
		Config: newTestDataSourceConfig(ctx, t, dsSchemaResp.Schema, cfgModel),
	}
	var resp datasource.ReadResponse
	resp.State = newTestDataSourceState(ctx, t, dsSchemaResp.Schema, cfgModel)

	d.Read(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError(), "read diagnostics error: %v", resp.Diagnostics)

	var stateModel supercargoContractDataSourceModel
	diags := resp.State.Get(ctx, &stateModel)
	require.False(t, diags.HasError())

	assert.Equal(t, "urn:supercargo:hub:contract:com.example.events", stateModel.URN.ValueString())
	assert.Equal(t, "v1.2.0", stateModel.Version.ValueString())
	assert.Equal(t, "analytics", stateModel.OwnerTeam.ValueString())

	// Validate SchemaJSON
	require.NotEmpty(t, stateModel.SchemaJSON.ValueString())
	var bqFieldsParsed []map[string]any
	err := json.Unmarshal([]byte(stateModel.SchemaJSON.ValueString()), &bqFieldsParsed)
	require.NoError(t, err)
	assert.Len(t, bqFieldsParsed, 14)

	// Validate flattened fields list
	require.Len(t, stateModel.Fields, 16) // 13 primitives + 1 record + 2 child fields

	expectedFieldMap := map[string]struct {
		fType string
		fMode string
	}{
		"field_string":             {"STRING", "REQUIRED"},
		"field_bytes":              {"BYTES", "NULLABLE"},
		"field_int64":              {"INT64", "REQUIRED"},
		"field_float64":            {"FLOAT64", "NULLABLE"},
		"field_bool":               {"BOOL", "REQUIRED"},
		"field_timestamp":          {"TIMESTAMP", "NULLABLE"},
		"field_date":               {"DATE", "NULLABLE"},
		"field_time":               {"TIME", "NULLABLE"},
		"field_datetime":           {"DATETIME", "NULLABLE"},
		"field_geography":          {"GEOGRAPHY", "NULLABLE"},
		"field_numeric":            {"NUMERIC", "NULLABLE"},
		"field_bignumeric":         {"BIGNUMERIC", "NULLABLE"},
		"field_json":               {"JSON", "NULLABLE"},
		"field_record":             {"RECORD", "REPEATED"},
		"field_record.child_str":   {"STRING", "REQUIRED"},
		"field_record.child_int":   {"INT64", "NULLABLE"},
	}

	for _, f := range stateModel.Fields {
		exp, ok := expectedFieldMap[f.Name.ValueString()]
		require.True(t, ok, "unexpected field name in state: %s", f.Name.ValueString())
		assert.Equal(t, exp.fType, f.Type.ValueString(), "field type mismatch for %s", f.Name.ValueString())
		assert.Equal(t, exp.fMode, f.Mode.ValueString(), "field mode mismatch for %s", f.Name.ValueString())
	}
}

func TestSupercargoContractDataSource_Read_DefaultLatestVersion(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var dsSchemaResp datasource.SchemaResponse
	d := &supercargoContractDataSource{client: providerData}
	d.Schema(ctx, datasource.SchemaRequest{}, &dsSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
		assert.Equal(t, "urn:supercargo:hub:contract:com.example.events", req.ContractUrn)
		assert.Equal(t, "", req.Version) // Default requested version is empty string
		return &hubv1.GetContractResponse{
			Contract: &hubv1.DataContract{
				Meta: &hubv1.Meta{
					Urn:       "urn:supercargo:hub:contract:com.example.events",
					Version:   "v2.5.0",
					OwnerTeam: "platform",
				},
				Schema: []*hubv1.Field{
					{Name: "id", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
				},
			},
		}, nil
	}
	mockSrv.mu.Unlock()

	cfgModel := supercargoContractDataSourceModel{
		ID:      types.StringValue("urn:supercargo:hub:contract:com.example.events"),
		Version: types.StringNull(),
	}

	req := datasource.ReadRequest{
		Config: newTestDataSourceConfig(ctx, t, dsSchemaResp.Schema, cfgModel),
	}
	var resp datasource.ReadResponse
	resp.State = newTestDataSourceState(ctx, t, dsSchemaResp.Schema, cfgModel)

	d.Read(ctx, req, &resp)
	require.False(t, resp.Diagnostics.HasError())

	var stateModel supercargoContractDataSourceModel
	diags := resp.State.Get(ctx, &stateModel)
	require.False(t, diags.HasError())
	assert.Equal(t, "v2.5.0", stateModel.Version.ValueString())
	assert.Equal(t, "platform", stateModel.OwnerTeam.ValueString())
}

func TestSupercargoContractDataSource_Read_NotFound(t *testing.T) {
	ctx := context.Background()
	mockSrv, providerData, _ := startMockHubServer(t)

	var dsSchemaResp datasource.SchemaResponse
	d := &supercargoContractDataSource{client: providerData}
	d.Schema(ctx, datasource.SchemaRequest{}, &dsSchemaResp)

	mockSrv.mu.Lock()
	mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
		return nil, status.Error(codes.NotFound, "contract not found")
	}
	mockSrv.mu.Unlock()

	cfgModel := supercargoContractDataSourceModel{
		ID:      types.StringValue("urn:supercargo:hub:contract:missing"),
		Version: types.StringValue("v1.0.0"),
	}

	req := datasource.ReadRequest{
		Config: newTestDataSourceConfig(ctx, t, dsSchemaResp.Schema, cfgModel),
	}
	var resp datasource.ReadResponse

	d.Read(ctx, req, &resp)
	require.True(t, resp.Diagnostics.HasError())
	assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error fetching contract from Hub")
	assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "contract not found")
}

func TestSupercargoContractDataSource_Read_InvalidResponse(t *testing.T) {
	ctx := context.Background()

	t.Run("empty contract in response", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		var dsSchemaResp datasource.SchemaResponse
		d := &supercargoContractDataSource{client: providerData}
		d.Schema(ctx, datasource.SchemaRequest{}, &dsSchemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return &hubv1.GetContractResponse{Contract: nil}, nil
		}
		mockSrv.mu.Unlock()

		cfgModel := supercargoContractDataSourceModel{
			ID:      types.StringValue("urn:supercargo:hub:contract:empty"),
			Version: types.StringValue("v1.0.0"),
		}

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, dsSchemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Contract not found")
	})

	t.Run("nil metadata in contract", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		var dsSchemaResp datasource.SchemaResponse
		d := &supercargoContractDataSource{client: providerData}
		d.Schema(ctx, datasource.SchemaRequest{}, &dsSchemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return &hubv1.GetContractResponse{
				Contract: &hubv1.DataContract{
					Meta:   nil,
					Schema: []*hubv1.Field{},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		cfgModel := supercargoContractDataSourceModel{
			ID:      types.StringValue("urn:supercargo:hub:contract:nilmeta"),
			Version: types.StringValue("v1.0.0"),
		}

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, dsSchemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Invalid Contract Metadata")
	})

	t.Run("invalid schema data type", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		var dsSchemaResp datasource.SchemaResponse
		d := &supercargoContractDataSource{client: providerData}
		d.Schema(ctx, datasource.SchemaRequest{}, &dsSchemaResp)

		mockSrv.mu.Lock()
		mockSrv.GetContractHook = func(ctx context.Context, req *hubv1.GetContractRequest) (*hubv1.GetContractResponse, error) {
			return &hubv1.GetContractResponse{
				Contract: &hubv1.DataContract{
					Meta: &hubv1.Meta{
						Urn:       "urn:supercargo:hub:contract:badschema",
						Version:   "v1.0.0",
						OwnerTeam: "platform",
					},
					Schema: []*hubv1.Field{
						{Name: "unspecified_field", Type: hubv1.DataType_DATA_TYPE_UNSPECIFIED},
					},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		cfgModel := supercargoContractDataSourceModel{
			ID:      types.StringValue("urn:supercargo:hub:contract:badschema"),
			Version: types.StringValue("v1.0.0"),
		}

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, dsSchemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Error translating schema to BigQuery format")
	})
}

func TestSupercargoContractDataSource_Read_Unconfigured(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider client", func(t *testing.T) {
		d := &supercargoContractDataSource{client: nil}
		var dsSchemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &dsSchemaResp)

		cfgModel := supercargoContractDataSourceModel{
			ID:      types.StringValue("urn:supercargo:hub:contract:test"),
			Version: types.StringValue("v1.0.0"),
		}

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, dsSchemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})

	t.Run("nil hub client in provider data", func(t *testing.T) {
		d := &supercargoContractDataSource{client: &ProviderData{HubClient: nil}}
		var dsSchemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &dsSchemaResp)

		cfgModel := supercargoContractDataSourceModel{
			ID:      types.StringValue("urn:supercargo:hub:contract:test"),
			Version: types.StringValue("v1.0.0"),
		}

		req := datasource.ReadRequest{
			Config: newTestDataSourceConfig(ctx, t, dsSchemaResp.Schema, cfgModel),
		}
		var resp datasource.ReadResponse
		d.Read(ctx, req, &resp)
		require.True(t, resp.Diagnostics.HasError())
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Provider Not Configured")
	})
}
