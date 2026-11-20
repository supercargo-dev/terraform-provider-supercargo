package manifest_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"github.com/supercargo-dev/terraform-provider-supercargo/internal/manifest"
)

func TestParseIngestionEngine_ScrubsBearerTokens(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		want    manifest.IngestionEngine
		wantErr bool
	}{
		{name: "empty", raw: "", want: manifest.IngestionEngineUnspecified},
		{name: "whitespace", raw: "   ", want: manifest.IngestionEngineUnspecified},
		{name: "unspecified", raw: "UNSPECIFIED", want: manifest.IngestionEngineUnspecified},
		{name: "streaming", raw: "Streaming", want: manifest.IngestionEngineStreaming},
		{name: "dlt", raw: " DLT ", want: manifest.IngestionEngineDLT},
		{name: "invalid_bearer", raw: "Bearer super-secret-tok-123", wantErr: true},
		{name: "invalid_unknown", raw: "spark", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := manifest.ParseIngestionEngine(tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "super-secret-tok-123")
				if tc.name == "invalid_bearer" {
					assert.Contains(t, err.Error(), "Bearer [REDACTED]")
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestProtoFieldsToBigQueryFields_CanonicalTypeMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		fields   []*hubv1.Field
		opts     manifest.SchemaOptions
		expected []manifest.BQFieldJSON
		wantErr  string
	}{
		{
			name:     "nil_fields_streaming_zero_value",
			fields:   nil,
			opts:     manifest.SchemaOptions{},
			expected: []manifest.BQFieldJSON{},
		},
		{
			name:   "nil_fields_with_include_dlt_metadata",
			fields: nil,
			opts:   manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngineDLT, IncludeDLTMetadata: true},
			expected: []manifest.BQFieldJSON{
				{Name: "_dlt_load_id", Type: "STRING", Mode: "REQUIRED", Description: "DLT load identifier"},
				{Name: "_dlt_id", Type: "STRING", Mode: "REQUIRED", Description: "DLT row identifier"},
			},
		},
		{
			name: "streaming_preserves_repeated_and_vector_float_repeated",
			fields: []*hubv1.Field{
				{Name: "labels", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REPEATED, Description: "issue labels"},
				{Name: "embedding", Type: hubv1.DataType_DATA_TYPE_VECTOR, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "text embedding"},
				{
					Name: "items",
					Type: hubv1.DataType_DATA_TYPE_STRUCT,
					Mode: hubv1.FieldMode_FIELD_MODE_REPEATED,
					Fields: []*hubv1.Field{
						{Name: "sku", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
					},
				},
			},
			opts: manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngineStreaming},
			expected: []manifest.BQFieldJSON{
				{Name: "labels", Type: "STRING", Mode: "REPEATED", Description: "issue labels"},
				{Name: "embedding", Type: "FLOAT", Mode: "REPEATED", Description: "text embedding"},
				{
					Name: "items",
					Type: "RECORD",
					Mode: "REPEATED",
					Fields: []manifest.BQFieldJSON{
						{Name: "sku", Type: "STRING", Mode: "REQUIRED"},
					},
				},
			},
		},
		{
			name: "dlt_collapses_repeated_primitives_and_structs_but_preserves_non_repeated_vector",
			fields: []*hubv1.Field{
				{Name: "issue_id", Type: hubv1.DataType_DATA_TYPE_INT64, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
				{Name: "labels", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REPEATED, Description: "issue labels"},
				{Name: "embedding", Type: hubv1.DataType_DATA_TYPE_VECTOR, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE, Description: "embedding vector"},
				{
					Name:        "reviewers",
					Type:        hubv1.DataType_DATA_TYPE_STRUCT,
					Mode:        hubv1.FieldMode_FIELD_MODE_REPEATED,
					Description: "repeated struct",
					Fields: []*hubv1.Field{
						{Name: "login", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
					},
				},
				{
					Name: "metadata",
					Type: hubv1.DataType_DATA_TYPE_STRUCT,
					Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE,
					Fields: []*hubv1.Field{
						{Name: "tags", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REPEATED, Description: "nested repeated"},
						{Name: "active", Type: hubv1.DataType_DATA_TYPE_BOOL, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
					},
				},
			},
			opts: manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngineDLT, IncludeDLTMetadata: false},
			expected: []manifest.BQFieldJSON{
				{Name: "issue_id", Type: "INT64", Mode: "REQUIRED"},
				{Name: "labels", Type: "JSON", Mode: "NULLABLE", Description: "issue labels"},
				{Name: "embedding", Type: "FLOAT", Mode: "REPEATED", Description: "embedding vector"},
				{Name: "reviewers", Type: "JSON", Mode: "NULLABLE", Description: "repeated struct"},
				{
					Name: "metadata",
					Type: "RECORD",
					Mode: "NULLABLE",
					Fields: []manifest.BQFieldJSON{
						{Name: "tags", Type: "JSON", Mode: "NULLABLE", Description: "nested repeated"},
						{Name: "active", Type: "BOOL", Mode: "REQUIRED"},
					},
				},
			},
		},
		{
			name: "dlt_metadata_dedup_case_insensitive",
			fields: []*hubv1.Field{
				{Name: "_DLT_LOAD_ID", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "topic", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REPEATED},
			},
			opts: manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngineDLT, IncludeDLTMetadata: true},
			expected: []manifest.BQFieldJSON{
				{Name: "_DLT_LOAD_ID", Type: "STRING", Mode: "NULLABLE"},
				{Name: "topic", Type: "JSON", Mode: "NULLABLE"},
				{Name: "_dlt_id", Type: "STRING", Mode: "REQUIRED", Description: "DLT row identifier"},
			},
		},
		{
			name:    "invalid_engine_scrubbed",
			fields:  []*hubv1.Field{{Name: "id", Type: hubv1.DataType_DATA_TYPE_STRING}},
			opts:    manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngine("Bearer tok-999")},
			wantErr: "Bearer [REDACTED]",
		},
		{
			name: "full_scalar_matrix_and_repeated_vector_in_dlt",
			fields: []*hubv1.Field{
				{Name: "f_str", Type: hubv1.DataType_DATA_TYPE_STRING, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
				{Name: "f_bytes", Type: hubv1.DataType_DATA_TYPE_BYTES, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_int", Type: hubv1.DataType_DATA_TYPE_INT64, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
				{Name: "f_float", Type: hubv1.DataType_DATA_TYPE_FLOAT64, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_bool", Type: hubv1.DataType_DATA_TYPE_BOOL, Mode: hubv1.FieldMode_FIELD_MODE_REQUIRED},
				{Name: "f_ts", Type: hubv1.DataType_DATA_TYPE_TIMESTAMP, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_date", Type: hubv1.DataType_DATA_TYPE_DATE, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_time", Type: hubv1.DataType_DATA_TYPE_TIME, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_dt", Type: hubv1.DataType_DATA_TYPE_DATETIME, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_geo", Type: hubv1.DataType_DATA_TYPE_GEOGRAPHY, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_num", Type: hubv1.DataType_DATA_TYPE_NUMERIC, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_bignum", Type: hubv1.DataType_DATA_TYPE_BIGNUMERIC, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_json", Type: hubv1.DataType_DATA_TYPE_JSON, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_vec", Type: hubv1.DataType_DATA_TYPE_VECTOR, Mode: hubv1.FieldMode_FIELD_MODE_NULLABLE},
				{Name: "f_rep_vec", Type: hubv1.DataType_DATA_TYPE_VECTOR, Mode: hubv1.FieldMode_FIELD_MODE_REPEATED},
			},
			opts: manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngineDLT},
			expected: []manifest.BQFieldJSON{
				{Name: "f_str", Type: "STRING", Mode: "REQUIRED"},
				{Name: "f_bytes", Type: "BYTES", Mode: "NULLABLE"},
				{Name: "f_int", Type: "INT64", Mode: "REQUIRED"},
				{Name: "f_float", Type: "FLOAT64", Mode: "NULLABLE"},
				{Name: "f_bool", Type: "BOOL", Mode: "REQUIRED"},
				{Name: "f_ts", Type: "TIMESTAMP", Mode: "NULLABLE"},
				{Name: "f_date", Type: "DATE", Mode: "NULLABLE"},
				{Name: "f_time", Type: "TIME", Mode: "NULLABLE"},
				{Name: "f_dt", Type: "DATETIME", Mode: "NULLABLE"},
				{Name: "f_geo", Type: "GEOGRAPHY", Mode: "NULLABLE"},
				{Name: "f_num", Type: "NUMERIC", Mode: "NULLABLE"},
				{Name: "f_bignum", Type: "BIGNUMERIC", Mode: "NULLABLE"},
				{Name: "f_json", Type: "JSON", Mode: "NULLABLE"},
				{Name: "f_vec", Type: "FLOAT", Mode: "REPEATED"},
				{Name: "f_rep_vec", Type: "JSON", Mode: "NULLABLE"},
			},
		},
		{
			name:    "unspecified_data_type_fails_closed",
			fields:  []*hubv1.Field{{Name: "bad", Type: hubv1.DataType_DATA_TYPE_UNSPECIFIED}},
			opts:    manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngineDLT},
			wantErr: "data type is unspecified",
		},
		{
			name:    "unknown_data_type_fails_closed",
			fields:  []*hubv1.Field{{Name: "bad", Type: hubv1.DataType(999)}},
			opts:    manifest.SchemaOptions{},
			wantErr: "unsupported or unknown hubv1.DataType",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := manifest.ProtoFieldsToBigQueryFields(tc.fields, tc.opts)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.NotContains(t, err.Error(), "tok-999")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)

			rawJSON, err := manifest.ProtoFieldsToBigQueryJSON(tc.fields, tc.opts)
			require.NoError(t, err)
			var decoded []manifest.BQFieldJSON
			require.NoError(t, json.Unmarshal([]byte(rawJSON), &decoded))
			assert.Equal(t, tc.expected, decoded)
		})
	}
}

func TestResolveContractsWithOptions_GitHubDLTShapesAndPrecedence(t *testing.T) {
	t.Parallel()

	t.Run("dlt input ports github-issues github-pulls github-repositories override stale static bigquery.json", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		schemasDir := filepath.Join(dir, "schemas")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))
		require.NoError(t, os.MkdirAll(schemasDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:github-analytics
  version: v1.0.0
input_ports:
  - name: github_issues
    source: dlt://custom
    contract:
      urn: urn:supercargo:hub:contract:github-issues
      version: 1.0.0
  - name: github_pulls
    type: dlt
    source: custom_github
    contract:
      urn: urn:supercargo:hub:contract:github-pulls
      version: 1.0.0
  - name: github_repositories
    source: dlt://github
    contract:
      urn: urn:supercargo:hub:contract:github-repositories
      version: 1.0.0
output_ports:
  - name: issues_passthrough
    source_ref: github_issues
    contract:
      urn: urn:supercargo:hub:contract:github-issues
      version: 1.0.0
  - name: pulls_analytics_dbt
    source_ref: dbt_pulls
    contract:
      urn: urn:supercargo:hub:contract:github-pulls-view
      version: 1.0.0
transforms:
  - name: dbt_pulls
    target_outputs:
      - pulls_analytics_dbt
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		issuesContract := `meta:
  urn: urn:supercargo:hub:contract:github-issues
  version: 1.0.0
schema:
  - name: issue_id
    type: DATA_TYPE_INT64
    mode: FIELD_MODE_REQUIRED
  - name: labels
    description: Repeated list of labels applied to the issue
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REPEATED
  - name: title_embedding
    type: DATA_TYPE_VECTOR
    mode: FIELD_MODE_NULLABLE
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "github-issues.1.0.0.contract.yaml"), []byte(issuesContract), 0644))

		// Stale static .bigquery.json with REPEATED labels: must be ignored for DLT, but honored if streaming
		staleIssuesJSON := `[{"name":"issue_id","type":"INT64","mode":"REQUIRED"},{"name":"labels","type":"STRING","mode":"REPEATED","description":"stale"}]`
		staleIssuesPath := filepath.Join(schemasDir, "github-issues.1.0.0.bigquery.json")
		require.NoError(t, os.WriteFile(staleIssuesPath, []byte(staleIssuesJSON), 0644))

		pullsContract := `meta:
  urn: urn:supercargo:hub:contract:github-pulls
  version: 1.0.0
schema:
  - name: pull_id
    type: DATA_TYPE_INT64
    mode: FIELD_MODE_REQUIRED
  - name: touched_files
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REPEATED
  - name: context
    type: DATA_TYPE_STRUCT
    mode: FIELD_MODE_NULLABLE
    fields:
      - name: reviewers
        type: DATA_TYPE_STRING
        mode: FIELD_MODE_REPEATED
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "github-pulls.1.0.0.contract.yaml"), []byte(pullsContract), 0644))

		reposContract := `meta:
  urn: urn:supercargo:hub:contract:github-repositories
  version: 1.0.0
schema:
  - name: repository_id
    type: DATA_TYPE_INT64
    mode: FIELD_MODE_REQUIRED
  - name: topics
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REPEATED
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "github-repositories.1.0.0.contract.yaml"), []byte(reposContract), 0644))

		pullsViewContract := `meta:
  urn: urn:supercargo:hub:contract:github-pulls-view
  version: 1.0.0
schema:
  - name: pull_id
    type: DATA_TYPE_INT64
    mode: FIELD_MODE_REQUIRED
  - name: touched_files
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REPEATED
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "github-pulls-view.1.0.0.contract.yaml"), []byte(pullsViewContract), 0644))

		m, err := manifest.Load(manifestPath)
		require.NoError(t, err)

		// 1. Default options: DLT ports infer DLT (dynamic generation overrides stale .bigquery.json), dbt transform stays streaming!
		resolved, err := manifest.ResolveContractsWithOptions(manifestPath, m, manifest.SchemaOptions{})
		require.NoError(t, err)
		require.Len(t, resolved, 4)

		issuesRC := resolved["urn:supercargo:hub:contract:github-issues"]
		require.NotNil(t, issuesRC)
		assert.Empty(t, issuesRC.SchemaPath, "DLT must dynamically generate from contract YAML instead of stale static .bigquery.json")
		var issuesFields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(issuesRC.SchemaJSON), &issuesFields))
		require.Len(t, issuesFields, 3)
		assert.Equal(t, manifest.BQFieldJSON{Name: "labels", Type: "JSON", Mode: "NULLABLE", Description: "Repeated list of labels applied to the issue"}, issuesFields[1])
		assert.Equal(t, manifest.BQFieldJSON{Name: "title_embedding", Type: "FLOAT", Mode: "REPEATED"}, issuesFields[2])

		pullsRC := resolved["urn:supercargo:hub:contract:github-pulls"]
		require.NotNil(t, pullsRC)
		var pullsFields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(pullsRC.SchemaJSON), &pullsFields))
		assert.Equal(t, "JSON", pullsFields[1].Type)
		assert.Equal(t, "NULLABLE", pullsFields[1].Mode)
		require.Len(t, pullsFields[2].Fields, 1)
		assert.Equal(t, "JSON", pullsFields[2].Fields[0].Type)
		assert.Equal(t, "NULLABLE", pullsFields[2].Fields[0].Mode)

		reposRC := resolved["urn:supercargo:hub:contract:github-repositories"]
		require.NotNil(t, reposRC)
		var reposFields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(reposRC.SchemaJSON), &reposFields))
		assert.Equal(t, "JSON", reposFields[1].Type)
		assert.Equal(t, "NULLABLE", reposFields[1].Mode)

		// Transform Isolation: dbt_pulls output stays streaming REPEATED
		pullsViewRC := resolved["urn:supercargo:hub:contract:github-pulls-view"]
		require.NotNil(t, pullsViewRC)
		var viewFields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(pullsViewRC.SchemaJSON), &viewFields))
		assert.Equal(t, "STRING", viewFields[1].Type)
		assert.Equal(t, "REPEATED", viewFields[1].Mode)

		// 2. IncludeDLTMetadata appends _dlt_load_id and _dlt_id
		resolvedMeta, err := manifest.ResolveContractsWithOptions(manifestPath, m, manifest.SchemaOptions{IncludeDLTMetadata: true})
		require.NoError(t, err)
		var issuesWithMeta []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(resolvedMeta["urn:supercargo:hub:contract:github-issues"].SchemaJSON), &issuesWithMeta))
		require.Len(t, issuesWithMeta, 5)
		assert.Equal(t, "_dlt_load_id", issuesWithMeta[3].Name)
		assert.Equal(t, "REQUIRED", issuesWithMeta[3].Mode)
		assert.Equal(t, "_dlt_id", issuesWithMeta[4].Name)
		assert.Equal(t, "REQUIRED", issuesWithMeta[4].Mode)

		// 3. Explicit streaming overrides dlt:// and dynamically generates REPEATED (bypassing stale static file too)
		resolvedStream, err := manifest.ResolveContractsWithOptions(manifestPath, m, manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngineStreaming})
		require.NoError(t, err)
		var streamIssues []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(resolvedStream["urn:supercargo:hub:contract:github-issues"].SchemaJSON), &streamIssues))
		assert.Equal(t, "STRING", streamIssues[1].Type)
		assert.Equal(t, "REPEATED", streamIssues[1].Mode)
		assert.Equal(t, "Repeated list of labels applied to the issue", streamIssues[1].Description)
	})

	t.Run("contract meta labels and evolution_profile trigger DLT and fallback transforms static JSON when schema absent", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		schemasDir := filepath.Join(dir, "schemas")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))
		require.NoError(t, os.MkdirAll(schemasDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:labeled-dlt
  version: v1.0.0
output_ports:
  - name: labeled_out
    contract:
      urn: urn:supercargo:hub:contract:labeled-out
      version: 1.0.0
  - name: profile_out
    evolution_profile: BIGQUERY_DLT
    contract:
      urn: urn:supercargo:hub:contract:profile-out
      version: 1.0.0
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		// Contract 1: has meta.labels supercargo.ingestion_engine: dlt and schema
		labeledContract := `meta:
  urn: urn:supercargo:hub:contract:labeled-out
  version: 1.0.0
  labels:
    supercargo.ingestion_engine: dlt
schema:
  - name: tags
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REPEATED
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "labeled-out.1.0.0.contract.yaml"), []byte(labeledContract), 0644))

		// Contract 2: has no schema in YAML, falls back to static bigquery.json and transforms it
		profileContract := `meta:
  urn: urn:supercargo:hub:contract:profile-out
  version: 1.0.0
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "profile-out.1.0.0.contract.yaml"), []byte(profileContract), 0644))
		staticProfileJSON := `[
  {"name":"topics","type":"STRING","mode":"REPEATED"},
  {"name":"vec","type":"FLOAT","mode":"REPEATED"}
]`
		staticProfilePath := filepath.Join(schemasDir, "profile-out.1.0.0.bigquery.json")
		require.NoError(t, os.WriteFile(staticProfilePath, []byte(staticProfileJSON), 0644))

		m, err := manifest.Load(manifestPath)
		require.NoError(t, err)

		resolved, err := manifest.ResolveContractsWithOptions(manifestPath, m, manifest.SchemaOptions{})
		require.NoError(t, err)

		var labeledFields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(resolved["urn:supercargo:hub:contract:labeled-out"].SchemaJSON), &labeledFields))
		assert.Equal(t, manifest.BQFieldJSON{Name: "tags", Type: "JSON", Mode: "NULLABLE"}, labeledFields[0])

		profileRC := resolved["urn:supercargo:hub:contract:profile-out"]
		assert.Equal(t, filepath.Clean(staticProfilePath), profileRC.SchemaPath)
		var profileFields []manifest.BQFieldJSON
		require.NoError(t, json.Unmarshal([]byte(profileRC.SchemaJSON), &profileFields))
		assert.Equal(t, manifest.BQFieldJSON{Name: "topics", Type: "JSON", Mode: "NULLABLE"}, profileFields[0])
		assert.Equal(t, manifest.BQFieldJSON{Name: "vec", Type: "FLOAT", Mode: "REPEATED"}, profileFields[1])
	})

	t.Run("invalid label engine fails closed with Bearer redacted", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:bad-label
  version: v1.0.0
output_ports:
  - name: out
    contract:
      urn: urn:supercargo:hub:contract:bad-label
      version: 1.0.0
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))
		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:bad-label
  version: 1.0.0
  labels:
    ingestion_engine: Bearer super-secret-label-token
schema:
  - name: id
    type: DATA_TYPE_STRING
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "bad-label.1.0.0.contract.yaml"), []byte(contractYAML), 0644))

		m, err := manifest.Load(manifestPath)
		require.NoError(t, err)

		_, err = manifest.ResolveContractsWithOptions(manifestPath, m, manifest.SchemaOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Bearer [REDACTED]")
		assert.NotContains(t, err.Error(), "super-secret-label-token")
	})
}

func TestResolvePortSchemaOptions_MatrixAndTransformIsolation(t *testing.T) {
	t.Parallel()

	dltInput := &hubv1.InputPort{Name: "gh_in", Source: "dlt://custom", ContractUrn: "urn:c:in"}
	streamInput := &hubv1.InputPort{Name: "ps_in", Source: "pubsub://managed", ContractUrn: "urn:c:ps"}
	multiManifest := &hubv1.ProductManifest{
		Meta:       &hubv1.ProductMeta{Urn: "urn:p:1"},
		InputPorts: []*hubv1.InputPort{dltInput, streamInput},
	}

	t.Run("explicit override beats contract and manifest labels", func(t *testing.T) {
		opts, err := manifest.ResolvePortSchemaOptions(
			&hubv1.ProductManifest{Meta: &hubv1.ProductMeta{Labels: map[string]string{"supercargo.ingestion_engine": "dlt"}}},
			nil,
			dltInput,
			map[string]string{"supercargo.ingestion_engine": "dlt"},
			"urn:c:in",
			manifest.SchemaOptions{IngestionEngine: manifest.IngestionEngineStreaming, IncludeDLTMetadata: true},
		)
		require.NoError(t, err)
		assert.Equal(t, manifest.IngestionEngineStreaming, opts.IngestionEngine)
		assert.True(t, opts.IncludeDLTMetadata)
	})

	t.Run("contract label overrides manifest label and port", func(t *testing.T) {
		opts, err := manifest.ResolvePortSchemaOptions(
			&hubv1.ProductManifest{Meta: &hubv1.ProductMeta{Labels: map[string]string{"ingestion_engine": "streaming"}}},
			&hubv1.OutputPort{Name: "out"},
			nil,
			map[string]string{"supercargo.ingestion_engine": "dlt"},
			"urn:c:out",
			manifest.SchemaOptions{},
		)
		require.NoError(t, err)
		assert.Equal(t, manifest.IngestionEngineDLT, opts.IngestionEngine)
	})

	t.Run("manifest label applies when port has no override", func(t *testing.T) {
		opts, err := manifest.ResolvePortSchemaOptions(
			&hubv1.ProductManifest{Meta: &hubv1.ProductMeta{Labels: map[string]string{"ingestion_engine": "dlt"}}},
			&hubv1.OutputPort{Name: "out"},
			nil,
			nil,
			"urn:c:out",
			manifest.SchemaOptions{},
		)
		require.NoError(t, err)
		assert.Equal(t, manifest.IngestionEngineDLT, opts.IngestionEngine)
	})

	t.Run("output port links via SourceRef and Contract.Urn and single input fallback", func(t *testing.T) {
		// SourceRef -> dltInput
		opts, err := manifest.ResolvePortSchemaOptions(
			multiManifest,
			&hubv1.OutputPort{Name: "out_sr"},
			nil,
			nil,
			"",
			manifest.SchemaOptions{},
			manifest.ManifestExtras{OutputSourceRefs: map[string]string{"out_sr": "gh_in"}},
		)
		require.NoError(t, err)
		assert.Equal(t, manifest.IngestionEngineDLT, opts.IngestionEngine)

		// Contract.Urn -> dltInput
		opts, err = manifest.ResolvePortSchemaOptions(
			multiManifest,
			&hubv1.OutputPort{Name: "out_urn", Contract: &hubv1.ContractPointer{Urn: "urn:c:in"}},
			nil,
			nil,
			"urn:c:in",
			manifest.SchemaOptions{},
		)
		require.NoError(t, err)
		assert.Equal(t, manifest.IngestionEngineDLT, opts.IngestionEngine)

		// Single DLT input port fallback when SourceRef is empty
		singleDLTManifest := &hubv1.ProductManifest{InputPorts: []*hubv1.InputPort{dltInput}}
		opts, err = manifest.ResolvePortSchemaOptions(
			singleDLTManifest,
			&hubv1.OutputPort{Name: "out_single"},
			nil,
			nil,
			"",
			manifest.SchemaOptions{},
		)
		require.NoError(t, err)
		assert.Equal(t, manifest.IngestionEngineDLT, opts.IngestionEngine)
	})

	t.Run("Transform Isolation keeps dbt targets streaming even with single DLT input", func(t *testing.T) {
		singleDLTManifest := &hubv1.ProductManifest{InputPorts: []*hubv1.InputPort{dltInput}}
		opts, err := manifest.ResolvePortSchemaOptions(
			singleDLTManifest,
			&hubv1.OutputPort{Name: "dbt_summary"},
			nil,
			nil,
			"",
			manifest.SchemaOptions{},
			manifest.ManifestExtras{TransformTargets: map[string]bool{"dbt_summary": true}},
		)
		require.NoError(t, err)
		assert.Equal(t, manifest.IngestionEngineStreaming, opts.IngestionEngine)
	})

	t.Run("invalid manifest label fails closed with Bearer redacted", func(t *testing.T) {
		_, err := manifest.ResolvePortSchemaOptions(
			&hubv1.ProductManifest{Meta: &hubv1.ProductMeta{Labels: map[string]string{"supercargo.ingestion_engine": "Bearer m-secret"}}},
			nil,
			nil,
			nil,
			"",
			manifest.SchemaOptions{},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Bearer [REDACTED]")
		assert.NotContains(t, err.Error(), "m-secret")
	})
}
