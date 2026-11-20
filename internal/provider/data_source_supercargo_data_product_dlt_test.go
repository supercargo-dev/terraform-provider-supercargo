package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSupercargoDataProductDataSource_DLT_SchemaAttributes(t *testing.T) {
	t.Parallel()
	d := NewSupercargoDataProductDataSource()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())

	engAttr := resp.Schema.Attributes["ingestion_engine"]
	require.NotNil(t, engAttr)
	assert.True(t, engAttr.IsOptional())
	assert.False(t, engAttr.IsComputed())

	metaAttr := resp.Schema.Attributes["include_dlt_metadata"]
	require.NotNil(t, metaAttr)
	assert.True(t, metaAttr.IsOptional())
	assert.False(t, metaAttr.IsComputed())
}

func TestSupercargoDataProductDataSource_DLT_HubAndLocalFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Hub GetProduct resolves DLT input port and surfaces schema_json JSON NULLABLE and metadata", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		productURN := "urn:supercargo:hub:product:github-dlt-hub"
		issuesContractURN := "urn:supercargo:hub:contract:github-issues"
		streamContractURN := "urn:supercargo:hub:contract:github-audit-stream"

		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return &hubv1.GetProductResponse{
				Manifest: &hubv1.ProductManifest{
					Meta: &hubv1.ProductMeta{
						Urn:     productURN,
						Version: "v1.0.0",
					},
					InputPorts: []*hubv1.InputPort{
						{
							Name:        "github_issues",
							Type:        "dlt",
							Source:      "dlt://custom",
							ContractUrn: issuesContractURN,
						},
						{
							Name:        "audit_stream",
							Source:      "pubsub://managed",
							ContractUrn: streamContractURN,
						},
					},
					OutputPorts: []*hubv1.OutputPort{
						{
							Name: "issues_out",
							Contract: &hubv1.ContractPointer{
								Urn: issuesContractURN,
							},
							Physical: &hubv1.PhysicalConfig{
								Bigquery: &hubv1.BigQueryConfig{
									Project:  "proj-dlt",
									Location: "US",
								},
							},
						},
						{
							Name: "audit_out",
							Contract: &hubv1.ContractPointer{
								Urn: streamContractURN,
							},
						},
					},
				},
				Contracts: []*hubv1.DataContract{
					{
						Meta: &hubv1.Meta{Urn: issuesContractURN, Version: "1.0.0"},
						Schema: []*hubv1.Field{
							{Name: "issue_id", Type: hubv1.DataType_DATA_TYPE_INT64, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
							{Name: "labels", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REPEATED, Description: "issue labels"},
							{Name: "embedding", Type: hubv1.DataType_DATA_TYPE_VECTOR, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
						},
					},
					{
						Meta: &hubv1.Meta{Urn: streamContractURN, Version: "1.0.0"},
						Schema: []*hubv1.Field{
							{Name: "tags", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REPEATED},
						},
					},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel("", productURN, "", "")
		cfgModel.IncludeDLTMetadata = types.BoolValue(true)

		req := datasource.ReadRequest{Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel)}
		resp := datasource.ReadResponse{State: newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)}
		d.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		var state supercargoDataProductDataSourceModel
		require.False(t, resp.State.Get(ctx, &state).HasError())
		var contracts map[string]supercargoContractModel
		require.False(t, state.Contracts.ElementsAs(ctx, &contracts, false).HasError())

		var issuesFields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(contracts[issuesContractURN].SchemaJSON.ValueString()), &issuesFields))
		require.Len(t, issuesFields, 5)
		assert.Equal(t, manifest.BQFieldJSON{Name: "labels", Type: "JSON", Mode: "NULLABLE", Description: "issue labels"}, issuesFields[1])
		assert.Equal(t, manifest.BQFieldJSON{Name: "embedding", Type: "FLOAT", Mode: "REPEATED"}, issuesFields[2])
		assert.Equal(t, "_dlt_load_id", issuesFields[3].Name)
		assert.Equal(t, "REQUIRED", issuesFields[3].Mode)
		assert.Equal(t, "_dlt_id", issuesFields[4].Name)

		var streamFields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(contracts[streamContractURN].SchemaJSON.ValueString()), &streamFields))
		require.Len(t, streamFields, 3)
		assert.Equal(t, manifest.BQFieldJSON{Name: "tags", Type: "STRING", Mode: "REPEATED"}, streamFields[0])
	})

	t.Run("local manifest_file fallback overrides stale static bigquery.json for DLT", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return nil, status.Error(codes.NotFound, "not found")
		}
		mockSrv.mu.Unlock()

		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		schemasDir := filepath.Join(dir, "schemas")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))
		require.NoError(t, os.MkdirAll(schemasDir, 0755))

		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:github-dlt-local
  version: v1.0.0
input_ports:
  - name: github_issues
    source: dlt://custom
    contract:
      urn: urn:supercargo:hub:contract:github-issues
      version: 1.0.0
output_ports:
  - name: issues_table
    source_ref: github_issues
    contract:
      urn: urn:supercargo:hub:contract:github-issues
      version: 1.0.0
    physical:
      bigquery:
        project: dlt-local-proj
        location: US
`), 0644))

		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "github-issues.1.0.0.contract.yaml"), []byte(`meta:
  urn: urn:supercargo:hub:contract:github-issues
  version: 1.0.0
schema:
  - name: issue_id
    type: DATA_TYPE_INT64
    mode: FIELD_MODE_REQUIRED
  - name: labels
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REPEATED
`), 0644))

		// Stale static schema must be bypassed
		require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "github-issues.1.0.0.bigquery.json"), []byte(`[{"name":"labels","type":"STRING","mode":"REPEATED"}]`), 0644))

		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		cfgModel := newTestConfigModel(manifestPath, "", "", "")
		req := datasource.ReadRequest{Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, cfgModel)}
		resp := datasource.ReadResponse{State: newTestDataSourceState(ctx, t, schemaResp.Schema, cfgModel)}
		d.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		var state supercargoDataProductDataSourceModel
		require.False(t, resp.State.Get(ctx, &state).HasError())
		var contracts map[string]supercargoContractModel
		require.False(t, state.Contracts.ElementsAs(ctx, &contracts, false).HasError())

		var fields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(contracts["urn:supercargo:hub:contract:github-issues"].SchemaJSON.ValueString()), &fields))
		require.Len(t, fields, 2)
		assert.Equal(t, manifest.BQFieldJSON{Name: "labels", Type: "JSON", Mode: "NULLABLE"}, fields[1])
	})

	t.Run("Unauthenticated Hub rejection and invalid ingestion_engine mask Bearer tokens", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		var hubCalls atomic.Int64
		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			hubCalls.Add(1)
			return nil, status.Error(codes.Unauthenticated, "invalid credential Bearer raw-secret-jwt-999")
		}
		mockSrv.mu.Unlock()

		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		// 1. Invalid ingestion_engine fails closed before Hub RPC
		badEngineCfg := newTestConfigModel("", "urn:supercargo:hub:product:test", "", "")
		badEngineCfg.IngestionEngine = types.StringValue("Bearer secret-engine-token")
		badResp := datasource.ReadResponse{State: newTestDataSourceState(ctx, t, schemaResp.Schema, badEngineCfg)}
		d.Read(ctx, datasource.ReadRequest{Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, badEngineCfg)}, &badResp)
		require.True(t, badResp.Diagnostics.HasError())
		assert.Equal(t, int64(0), hubCalls.Load(), "invalid ingestion_engine must fail before Hub RPC")
		assert.Equal(t, "Invalid Ingestion Engine", badResp.Diagnostics.Errors()[0].Summary())
		assert.Contains(t, badResp.Diagnostics.Errors()[0].Detail(), "Bearer [REDACTED]")
		assert.NotContains(t, badResp.Diagnostics.Errors()[0].Detail(), "secret-engine-token")

		// 2. Unauthenticated Hub error scrubs Bearer token
		unauthCfg := newTestConfigModel("", "urn:supercargo:hub:product:test", "", "")
		unauthResp := datasource.ReadResponse{State: newTestDataSourceState(ctx, t, schemaResp.Schema, unauthCfg)}
		d.Read(ctx, datasource.ReadRequest{Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, unauthCfg)}, &unauthResp)
		require.True(t, unauthResp.Diagnostics.HasError())
		assert.Equal(t, int64(1), hubCalls.Load())
		assert.Contains(t, unauthResp.Diagnostics.Errors()[0].Detail(), "Bearer [REDACTED]")
		assert.NotContains(t, unauthResp.Diagnostics.Errors()[0].Detail(), "raw-secret-jwt-999")
	})

	t.Run("Hub contract translation error and local invalid label error are surfaced instead of swallowed", func(t *testing.T) {
		mockSrv, providerData, _ := startMockHubServer(t)
		mockSrv.mu.Lock()
		mockSrv.GetProductHook = func(ctx context.Context, req *hubv1.GetProductRequest) (*hubv1.GetProductResponse, error) {
			return &hubv1.GetProductResponse{
				Manifest: &hubv1.ProductManifest{Meta: &hubv1.ProductMeta{Urn: req.ProductUrn, Version: "v1.0.0"}},
				Contracts: []*hubv1.DataContract{
					{
						Meta:   &hubv1.Meta{Urn: "urn:supercargo:hub:contract:bad", Version: "1.0.0"},
						Schema: []*hubv1.Field{{Name: "broken", Type: hubv1.DataType_DATA_TYPE_UNSPECIFIED}},
					},
				},
			}, nil
		}
		mockSrv.mu.Unlock()

		d := &supercargoDataProductDataSource{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: providerData}, &datasource.ConfigureResponse{})
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

		hubErrCfg := newTestConfigModel("", "urn:supercargo:hub:product:bad-schema", "", "")
		hubErrResp := datasource.ReadResponse{State: newTestDataSourceState(ctx, t, schemaResp.Schema, hubErrCfg)}
		d.Read(ctx, datasource.ReadRequest{Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, hubErrCfg)}, &hubErrResp)
		require.True(t, hubErrResp.Diagnostics.HasError())
		assert.Contains(t, hubErrResp.Diagnostics.Errors()[0].Summary(), "Error Translating Contract Schema")

		// Local fallback invalid label in contract YAML is surfaced with Bearer [REDACTED]
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))
		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:local-bad
  version: v1.0.0
output_ports:
  - name: out
    contract:
      urn: urn:supercargo:hub:contract:local-bad
      version: 1.0.0
`), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "local-bad.1.0.0.contract.yaml"), []byte(`meta:
  urn: urn:supercargo:hub:contract:local-bad
  version: 1.0.0
  labels:
    supercargo.ingestion_engine: Bearer leak-tok-42
schema:
  - name: id
    type: DATA_TYPE_STRING
`), 0644))

		dLocal := &supercargoDataProductDataSource{}
		localCfg := newTestConfigModel(manifestPath, "", "", "")
		localResp := datasource.ReadResponse{State: newTestDataSourceState(ctx, t, schemaResp.Schema, localCfg)}
		dLocal.Read(ctx, datasource.ReadRequest{Config: newTestDataSourceConfig(ctx, t, schemaResp.Schema, localCfg)}, &localResp)
		require.True(t, localResp.Diagnostics.HasError())
		assert.Contains(t, localResp.Diagnostics.Errors()[0].Summary(), "Error Resolving Local Contracts")
		assert.Contains(t, localResp.Diagnostics.Errors()[0].Detail(), "Bearer [REDACTED]")
		assert.NotContains(t, localResp.Diagnostics.Errors()[0].Detail(), "leak-tok-42")
	})
}
