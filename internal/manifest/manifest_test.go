package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
)

func TestLoad(t *testing.T) {
	t.Run("success loading valid YAML manifest", func(t *testing.T) {
		content := `meta:
  urn: urn:supercargo:hub:product:test-prod
  version: v1.0.0
  owner:
    team_name: core-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
`
		tmpPath := filepath.Join(t.TempDir(), "product.yaml")
		err := os.WriteFile(tmpPath, []byte(content), 0644)
		require.NoError(t, err)

		m, err := Load(tmpPath)
		require.NoError(t, err)
		require.NotNil(t, m)
		require.NotNil(t, m.Meta)
		assert.Equal(t, "urn:supercargo:hub:product:test-prod", m.Meta.Urn)
		assert.Equal(t, "v1.0.0", m.Meta.Version)
		require.NotNil(t, m.Meta.Owner)
		assert.Equal(t, "core-team", m.Meta.Owner.TeamName)
		require.Len(t, m.OutputPorts, 1)
		assert.Equal(t, "main", m.OutputPorts[0].Name)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		m, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
		require.Error(t, err)
		assert.Nil(t, m)
		assert.Contains(t, err.Error(), "failed to read manifest file")
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		tmpPath := filepath.Join(t.TempDir(), "invalid.yaml")
		err := os.WriteFile(tmpPath, []byte("invalid: yaml: : :"), 0644)
		require.NoError(t, err)

		m, err := Load(tmpPath)
		require.Error(t, err)
		assert.Nil(t, m)
		assert.Contains(t, err.Error(), "failed to unmarshal YAML")
	})
}

func TestResolveContracts(t *testing.T) {
	t.Run("auto-discovers contract YAML and schema JSON in schemas dir with snake_case and kebab-case", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		schemasDir := filepath.Join(dir, "schemas")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))
		require.NoError(t, os.MkdirAll(schemasDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:customer-telemetry
  version: v1.0.0
  owner:
    team_name: team-alpha
output_ports:
  - name: page_views
    urn: urn:supercargo:hub:port:page-views
    contract:
      urn: urn:supercargo:hub:contract:page-views
      version: "1.0.0"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:page-views
  version: "1.0.0"
  data_asset: go://github.com/supercargo-dev/core/examples/data-producer
  commit_sha: example-commit-sha-123456
schema:
  - name: user_id
    description: The unique identifier for the user
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REQUIRED
  - name: url
    description: The URL of the page viewed
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REQUIRED
`
		contractPath := filepath.Join(contractsDir, "page_views.yaml")
		require.NoError(t, os.WriteFile(contractPath, []byte(contractYAML), 0644))

		schemaJSON := `[
  {
    "name": "user_id",
    "type": "STRING",
    "mode": "REQUIRED",
    "description": "The unique identifier for the user"
  },
  {
    "name": "url",
    "type": "STRING",
    "mode": "REQUIRED",
    "description": "The URL of the page viewed"
  }
]`
		schemaPath := filepath.Join(schemasDir, "page_views.1.0.0.bigquery.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(schemaJSON), 0644))

		m, err := Load(manifestPath)
		require.NoError(t, err)
		require.NotNil(t, m)

		resolved, err := ResolveContracts(manifestPath, m)
		require.NoError(t, err)
		require.NotNil(t, resolved)
		require.Len(t, resolved, 1)

		contract, ok := resolved["urn:supercargo:hub:contract:page-views"]
		require.True(t, ok)
		assert.Equal(t, "urn:supercargo:hub:contract:page-views", contract.URN)
		assert.Equal(t, "1.0.0", contract.Version)
		assert.Equal(t, "page_views", contract.Name)
		assert.Equal(t, filepath.Clean(contractPath), contract.ContractPath)
		assert.Equal(t, filepath.Clean(schemaPath), contract.SchemaPath)
		assert.Equal(t, schemaJSON, contract.SchemaJSON)
		assert.Equal(t, "example-commit-sha-123456", contract.CommitSha)
		assert.Equal(t, "go://github.com/supercargo-dev/core/examples/data-producer", contract.DataAsset)
		assert.NotEmpty(t, contract.ContentHash)
	})

	t.Run("locates schema JSON in manifest root with kebab-case <contract-name>.<version>.bigquery.json", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:customer-telemetry
  version: v1.0.0
output_ports:
  - name: page_views
    contract:
      urn: urn:supercargo:hub:contract:page-views
      version: "1.0.0"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:page-views
  version: "1.0.0"
schema:
  - name: id
    type: DATA_TYPE_STRING
`
		contractPath := filepath.Join(dir, "page-views.contract.yaml")
		require.NoError(t, os.WriteFile(contractPath, []byte(contractYAML), 0644))

		schemaJSON := `[{"name":"id","type":"STRING","mode":"REQUIRED"}]`
		schemaPath := filepath.Join(dir, "page-views.1.0.0.bigquery.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(schemaJSON), 0644))

		m, err := Load(manifestPath)
		require.NoError(t, err)

		resolved, err := ResolveContracts(manifestPath, m)
		require.NoError(t, err)
		require.Len(t, resolved, 1)

		contract := resolved["urn:supercargo:hub:contract:page-views"]
		require.NotNil(t, contract)
		assert.Equal(t, filepath.Clean(contractPath), contract.ContractPath)
		assert.Equal(t, filepath.Clean(schemaPath), contract.SchemaPath)
		assert.Equal(t, schemaJSON, contract.SchemaJSON)
	})

	t.Run("dynamic fallback generation of BigQuery schema JSON when .bigquery.json is absent", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:order-service
  version: v2.0.0
output_ports:
  - name: orders
    contract:
      urn: urn:supercargo:hub:contract:orders
      version: "2.0.0"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:orders
  version: "2.0.0"
  data_asset: "go://github.com/supercargo-dev/core/orders"
schema:
  - name: order_id
    description: "Unique order identifier"
    type: "DATA_TYPE_STRING"
    mode: "FIELD_MODE_REQUIRED"
  - name: amount
    description: "Total order amount"
    type: "DATA_TYPE_FLOAT64"
    mode: "FIELD_MODE_NULLABLE"
  - name: items
    description: "Line items"
    type: "DATA_TYPE_STRUCT"
    mode: "FIELD_MODE_REPEATED"
    fields:
      - name: sku
        type: "DATA_TYPE_STRING"
        mode: "FIELD_MODE_REQUIRED"
`
		contractPath := filepath.Join(contractsDir, "orders.yaml")
		require.NoError(t, os.WriteFile(contractPath, []byte(contractYAML), 0644))

		m, err := Load(manifestPath)
		require.NoError(t, err)

		resolved, err := ResolveContracts(manifestPath, m)
		require.NoError(t, err)
		require.Len(t, resolved, 1)

		contract := resolved["urn:supercargo:hub:contract:orders"]
		require.NotNil(t, contract)
		assert.Equal(t, "", contract.SchemaPath, "SchemaPath should be empty when generated dynamically")
		assert.NotEmpty(t, contract.SchemaJSON)

		// Verify generated JSON can be unmarshaled into BQ schema structures
		type bqField struct {
			Name        string    `json:"name"`
			Type        string    `json:"type"`
			Mode        string    `json:"mode"`
			Description string    `json:"description,omitempty"`
			Fields      []bqField `json:"fields,omitempty"`
		}
		var fields []bqField
		err = json.Unmarshal([]byte(contract.SchemaJSON), &fields)
		require.NoError(t, err)
		require.Len(t, fields, 3)

		assert.Equal(t, "order_id", fields[0].Name)
		assert.Equal(t, "STRING", fields[0].Type)
		assert.Equal(t, "REQUIRED", fields[0].Mode)
		assert.Equal(t, "Unique order identifier", fields[0].Description)

		assert.Equal(t, "amount", fields[1].Name)
		assert.Equal(t, "FLOAT64", fields[1].Type)
		assert.Equal(t, "NULLABLE", fields[1].Mode)

		assert.Equal(t, "items", fields[2].Name)
		assert.Equal(t, "STRUCT", fields[2].Type)
		assert.Equal(t, "REPEATED", fields[2].Mode)
		require.Len(t, fields[2].Fields, 1)
		assert.Equal(t, "sku", fields[2].Fields[0].Name)
		assert.Equal(t, "STRING", fields[2].Fields[0].Type)
		assert.Equal(t, "REQUIRED", fields[2].Fields[0].Mode)
	})

	t.Run("explicit contract path in manifest is resolved", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		customDir := filepath.Join(dir, "custom", "contracts")
		require.NoError(t, os.MkdirAll(customDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:custom-prod
  version: v1.0.0
output_ports:
  - name: custom_port
    contract:
      urn: urn:supercargo:hub:contract:custom
      version: "1.0.0"
      path: "custom/contracts/my_custom_contract.yaml"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		contractYAML := `meta:
  urn: urn:supercargo:hub:contract:custom
  version: "1.0.0"
schema:
  - name: custom_field
    type: STRING
    mode: REQUIRED
`
		customContractPath := filepath.Join(customDir, "my_custom_contract.yaml")
		require.NoError(t, os.WriteFile(customContractPath, []byte(contractYAML), 0644))

		m, err := Load(manifestPath)
		require.NoError(t, err)

		resolved, err := ResolveContracts(manifestPath, m)
		require.NoError(t, err)
		require.Len(t, resolved, 1)

		contract := resolved["urn:supercargo:hub:contract:custom"]
		require.NotNil(t, contract)
		assert.Equal(t, filepath.Clean(customContractPath), contract.ContractPath)
	})

	t.Run("graceful handling when ports have no contracts or manifest is empty/nil", func(t *testing.T) {
		// Nil manifest
		res, err := ResolveContracts("some/path", nil)
		require.NoError(t, err)
		assert.Empty(t, res)

		// Manifest with no output ports
		manifestNoPorts := &hubv1.ProductManifest{}
		res, err = ResolveContracts("some/path", manifestNoPorts)
		require.NoError(t, err)
		assert.Empty(t, res)

		// Manifest with port having nil contract
		manifestNilContract := &hubv1.ProductManifest{
			OutputPorts: []*hubv1.OutputPort{
				{
					Name:     "raw_port",
					Contract: nil,
				},
			},
		}
		res, err = ResolveContracts("some/path", manifestNilContract)
		require.NoError(t, err)
		assert.Empty(t, res)
	})

	t.Run("error when contract file and schema are missing", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:missing-contract
  version: v1.0.0
output_ports:
  - name: missing_port
    contract:
      urn: urn:supercargo:hub:contract:missing
      version: "1.0.0"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		m, err := Load(manifestPath)
		require.NoError(t, err)

		_, err = ResolveContracts(manifestPath, m)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing")
	})

	t.Run("resolves contracts declared in input_ports for both brownfield pubsub and managed streams", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, "dataproduct.yaml")
		contractsDir := filepath.Join(dir, "contracts")
		require.NoError(t, os.MkdirAll(contractsDir, 0755))

		manifestYAML := `meta:
  urn: urn:supercargo:hub:product:rockify-analytics
  version: v1.0.0
  owner:
    team_name: rockify
input_ports:
  - name: subscriptions
    source: pubsub://projects/rockify-data/topics/rockify-subscription-events
    contract:
      urn: urn:supercargo:hub:contract:rockify-subscriptions
      version: "1.0.0"
  - name: telemetry
    source: pubsub://managed
    contract:
      urn: urn:supercargo:hub:contract:rockify-telemetry
      version: "1.0.0"
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYAML), 0644))

		subContractYAML := `meta:
  urn: urn:supercargo:hub:contract:rockify-subscriptions
  version: "1.0.0"
schema:
  - name: user_id
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REQUIRED
  - name: email
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REQUIRED
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "rockify-subscriptions.1.0.0.contract.yaml"), []byte(subContractYAML), 0644))

		telContractYAML := `meta:
  urn: urn:supercargo:hub:contract:rockify-telemetry
  version: "1.0.0"
schema:
  - name: event_id
    type: DATA_TYPE_STRING
    mode: FIELD_MODE_REQUIRED
  - name: duration_ms
    type: DATA_TYPE_INT64
    mode: FIELD_MODE_REQUIRED
`
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "rockify-telemetry.1.0.0.contract.yaml"), []byte(telContractYAML), 0644))

		m, err := Load(manifestPath)
		require.NoError(t, err)
		require.NotNil(t, m)
		require.Len(t, m.InputPorts, 2)
		assert.Equal(t, "pubsub://projects/rockify-data/topics/rockify-subscription-events", m.InputPorts[0].Source)
		assert.Equal(t, "pubsub://managed", m.InputPorts[1].Source)

		resolved, err := ResolveContracts(manifestPath, m)
		require.NoError(t, err)
		require.NotNil(t, resolved)
		require.Len(t, resolved, 2, "Both input port contracts should be resolved")

		subCont, ok := resolved["urn:supercargo:hub:contract:rockify-subscriptions"]
		require.True(t, ok)
		assert.Equal(t, "urn:supercargo:hub:contract:rockify-subscriptions", subCont.URN)
		assert.Equal(t, "1.0.0", subCont.Version)
		assert.NotEmpty(t, subCont.SchemaJSON)

		telCont, ok := resolved["urn:supercargo:hub:contract:rockify-telemetry"]
		require.True(t, ok)
		assert.Equal(t, "urn:supercargo:hub:contract:rockify-telemetry", telCont.URN)
		assert.Equal(t, "1.0.0", telCont.Version)
		assert.NotEmpty(t, telCont.SchemaJSON)
	})
}

func TestRockify_DataproductManifestAndContracts(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "..", "rockify", "dataproduct.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Skipf("rockify/dataproduct.yaml does not exist at %s (skipping external monorepo test in standalone provider CI)", manifestPath)
	}

	m, err := Load(manifestPath)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.Meta)
	assert.Equal(t, "urn:supercargo:hub:product:rockify-analytics:v1", m.Meta.Urn)
	require.Len(t, m.InputPorts, 4, "Must declare 4 input ports for the 4 Rockify microservices")

	resolved, err := ResolveContracts(manifestPath, m)
	require.NoError(t, err)
	require.Len(t, resolved, 4, "All 4 contracts must be resolved")

	expectedContracts := []string{
		"urn:supercargo:hub:contract:rockify-subscriptions",
		"urn:supercargo:hub:contract:rockify-playback",
		"urn:supercargo:hub:contract:rockify-telemetry",
		"urn:supercargo:hub:contract:rockify-recommendations",
	}

	for _, cURN := range expectedContracts {
		rc, ok := resolved[cURN]
		require.True(t, ok, "Contract %s must be present in resolved map", cURN)
		assert.NotEmpty(t, rc.SchemaJSON, "Schema JSON must not be empty for %s", cURN)
		assert.Equal(t, "1.0.0", rc.Version)
	}
}

func TestExtractViewQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "CREATE OR REPLACE VIEW standard",
			input: `CREATE OR REPLACE VIEW ` + "`acme.orders.orders_replica`" + ` AS
SELECT
  _sc_sequence,
  order_id,
  amount
FROM ` + "`acme.orders_internal.orders`",
			expected: `SELECT
  _sc_sequence,
  order_id,
  amount
FROM ` + "`acme.orders_internal.orders`",
		},
		{
			name: "CREATE VIEW with OPTIONS",
			input: `CREATE VIEW ` + "`acme.orders.orders_replica`" + ` OPTIONS(description="orders replica") AS
WITH ranked AS (
  SELECT * FROM ` + "`acme.orders_internal.orders`" + `
)
SELECT * FROM ranked;`,
			expected: `WITH ranked AS (
  SELECT * FROM ` + "`acme.orders_internal.orders`" + `
)
SELECT * FROM ranked`,
		},
		{
			name: "Already unwrapped query",
			input: `SELECT * FROM ` + "`acme.orders_internal.orders`",
			expected: `SELECT * FROM ` + "`acme.orders_internal.orders`",
		},
		{
			name: "Lowercase create or replace view",
			input: `create or replace view ` + "`my_project.my_ds.my_view`" + ` as
select id, name from ` + "`my_project.my_internal.my_table`",
			expected: `select id, name from ` + "`my_project.my_internal.my_table`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractViewQuery(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDiscoverLockDir(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "dataproduct.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte("meta:\n  urn: urn:supercargo:hub:product:orders\n"), 0644))

	t.Run("explicit missing directory returns error", func(t *testing.T) {
		missingPath := filepath.Join(dir, "missing_lock")
		_, found, err := DiscoverLockDir(manifestPath, "orders", missingPath)
		assert.False(t, found)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "explicitly configured lock_dir")
	})

	t.Run("explicit present directory returns path", func(t *testing.T) {
		presentPath := filepath.Join(dir, "custom_lock")
		require.NoError(t, os.MkdirAll(presentPath, 0755))
		discovered, found, err := DiscoverLockDir(manifestPath, "orders", presentPath)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, presentPath, discovered)
	})

	t.Run("omitted missing directory returns graceful false", func(t *testing.T) {
		emptyDir := t.TempDir()
		emptyManifest := filepath.Join(emptyDir, "dataproduct.yaml")
		require.NoError(t, os.WriteFile(emptyManifest, []byte("meta:\n  urn: urn:supercargo:hub:product:orders\n"), 0644))

		discovered, found, err := DiscoverLockDir(emptyManifest, "orders", "")
		require.NoError(t, err)
		assert.False(t, found)
		assert.Empty(t, discovered)
	})

	t.Run("omitted auto-discovery finds .supercargo/lock/<slug>", func(t *testing.T) {
		slugLockDir := filepath.Join(dir, ".supercargo", "lock", "orders")
		require.NoError(t, os.MkdirAll(slugLockDir, 0755))

		discovered, found, err := DiscoverLockDir(manifestPath, "orders", "")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, slugLockDir, discovered)
	})
}

func TestLoadCompiledOutputs(t *testing.T) {
	dir := t.TempDir()
	viewsDir := filepath.Join(dir, "views")
	contractsDir := filepath.Join(dir, "contracts")
	require.NoError(t, os.MkdirAll(viewsDir, 0755))
	require.NoError(t, os.MkdirAll(contractsDir, 0755))

	viewSQL := "CREATE OR REPLACE VIEW `acme.orders.orders_replica` AS\nSELECT order_id, amount FROM `acme.orders_internal.orders`;"
	require.NoError(t, os.WriteFile(filepath.Join(viewsDir, "orders_replica.sql"), []byte(viewSQL), 0644))

	odcsYAML := `id: urn:supercargo:hub:contract:orders-orders_replica:v1
info:
  title: orders_replica
  version: 1.0.0
`
	require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "orders_replica.odcs.yaml"), []byte(odcsYAML), 0644))

	outputs, err := LoadCompiledOutputs(dir, "orders")
	require.NoError(t, err)
	require.Len(t, outputs, 1)

	expectedURN := "urn:supercargo:hub:contract:orders-orders_replica:v1"
	out, ok := outputs[expectedURN]
	require.True(t, ok, "outputs must contain contract URN as key")
	assert.Equal(t, "orders_replica", out.Name)
	assert.Equal(t, expectedURN, out.URN)
	assert.Equal(t, "replica", out.Lens)
	assert.Equal(t, "view", out.Materialization)
	assert.Equal(t, "SELECT order_id, amount FROM `acme.orders_internal.orders`", out.ViewQuery)
	assert.Equal(t, viewSQL, out.ViewSQL)
}

func TestLoadCompiledInputs(t *testing.T) {
	dir := t.TempDir()
	ingressDir := filepath.Join(dir, "ingress")
	schemasDir := filepath.Join(dir, "schemas")
	require.NoError(t, os.MkdirAll(ingressDir, 0755))
	require.NoError(t, os.MkdirAll(schemasDir, 0755))

	ingressYAML := `meta:
  urn: urn:supercargo:hub:contract:orders-internal-stream:v1
  description: Internal ingress contract for orders.stream
`
	require.NoError(t, os.WriteFile(filepath.Join(ingressDir, "stream.contract.yaml"), []byte(ingressYAML), 0644))

	schemaJSON := `[
  {
    "name": "_sc_sequence",
    "type": "INT64",
    "mode": "REQUIRED"
  },
  {
    "name": "order_id",
    "type": "STRING",
    "mode": "REQUIRED"
  }
]`
	require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "orders_internal.orders.schema.json"), []byte(schemaJSON), 0644))

	inputs, err := LoadCompiledInputs(dir, "orders")
	require.NoError(t, err)
	require.Len(t, inputs, 1)

	expectedURN := "urn:supercargo:hub:contract:orders-internal-stream:v1"
	in, ok := inputs[expectedURN]
	require.True(t, ok, "inputs must contain contract URN as key")
	assert.Equal(t, "stream", in.Name)
	assert.Equal(t, expectedURN, in.URN)
	assert.Equal(t, "orders", in.RecordName)
	assert.Equal(t, "orders", in.TableName)
	assert.JSONEq(t, schemaJSON, in.SchemaJSON)
}

func TestLoadCompiledOutputs_AllLensesAndBridge(t *testing.T) {
	dir := t.TempDir()
	viewsDir := filepath.Join(dir, "views")
	contractsDir := filepath.Join(dir, "contracts")
	require.NoError(t, os.MkdirAll(viewsDir, 0755))
	require.NoError(t, os.MkdirAll(contractsDir, 0755))

	lenses := []struct {
		portName string
		expectedLens string
	}{
		{portName: "orders_event_log", expectedLens: "event_log"},
		{portName: "orders_replica", expectedLens: "replica"},
		{portName: "orders_scd2", expectedLens: "scd2"},
		{portName: "orders_cdc", expectedLens: "cdc"},
		{portName: "orders_reidentified", expectedLens: "reidentified"},
		{portName: "orders_v1", expectedLens: "bridge"},
	}

	for _, tc := range lenses {
		sql := fmt.Sprintf("CREATE OR REPLACE VIEW `acme.orders.%s` AS\nSELECT * FROM `acme.orders_internal.orders`;", tc.portName)
		require.NoError(t, os.WriteFile(filepath.Join(viewsDir, tc.portName+".sql"), []byte(sql), 0644))
	}

	outputs, err := LoadCompiledOutputs(dir, "orders")
	require.NoError(t, err)
	require.Len(t, outputs, 6)

	for _, tc := range lenses {
		expectedURN := fmt.Sprintf("urn:supercargo:hub:contract:orders-%s:v1", tc.portName)
		out, ok := outputs[expectedURN]
		require.True(t, ok, "missing output for %s", expectedURN)
		assert.Equal(t, tc.portName, out.Name)
		assert.Equal(t, tc.expectedLens, out.Lens)
		assert.Equal(t, "view", out.Materialization)
		assert.Equal(t, "SELECT * FROM `acme.orders_internal.orders`", out.ViewQuery)
	}
}

func TestLoadCompiledInputs_DataplexPolicyTags(t *testing.T) {
	dir := t.TempDir()
	schemasDir := filepath.Join(dir, "schemas")
	require.NoError(t, os.MkdirAll(schemasDir, 0755))

	schemaJSON := `[
  {
    "name": "email",
    "type": "STRING",
    "mode": "REQUIRED",
    "policyTags": {
      "names": [
        "projects/acme-prod/locations/europe-west1/taxonomies/tax_pii/policyTags/pt_confidential"
      ]
    }
  }
]`
	require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "orders_internal.customers.schema.json"), []byte(schemaJSON), 0644))

	inputs, err := LoadCompiledInputs(dir, "orders")
	require.NoError(t, err)
	require.Len(t, inputs, 1)

	expectedURN := "urn:supercargo:hub:contract:orders-internal-customers:v1"
	in, ok := inputs[expectedURN]
	require.True(t, ok)
	assert.Contains(t, in.SchemaJSON, "projects/acme-prod/locations/europe-west1/taxonomies/tax_pii/policyTags/pt_confidential", "Dataplex policy tags must be preserved in schema_json")
}

func TestLoadCompiledOutputs_DuplicateKeyError(t *testing.T) {
	dir := t.TempDir()
	viewsDir := filepath.Join(dir, "views")
	contractsDir := filepath.Join(dir, "contracts")
	require.NoError(t, os.MkdirAll(viewsDir, 0755))
	require.NoError(t, os.MkdirAll(contractsDir, 0755))

	// Two views mapping to the same contract URN
	require.NoError(t, os.WriteFile(filepath.Join(viewsDir, "orders_view_a.sql"), []byte("SELECT 1;"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(viewsDir, "orders_view_b.sql"), []byte("SELECT 2;"), 0644))

	sameURN := "urn:supercargo:hub:contract:orders-duplicate:v1"
	require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "orders_view_a.odcs.yaml"), []byte("id: "+sameURN+"\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "orders_view_b.odcs.yaml"), []byte("id: "+sameURN+"\n"), 0644))

	_, err := LoadCompiledOutputs(dir, "orders")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate collision-resistant collection key")
}

func TestDiscoverLockDir_MultiProductWorkspace(t *testing.T) {
	workspaceDir := t.TempDir()
	productDir := filepath.Join(workspaceDir, "products", "billing")
	require.NoError(t, os.MkdirAll(productDir, 0755))
	manifestPath := filepath.Join(productDir, "dataproduct.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte("meta:\n  urn: urn:supercargo:hub:product:billing\n"), 0644))

	slugLockDir := filepath.Join(workspaceDir, ".supercargo", "lock", "billing")
	require.NoError(t, os.MkdirAll(slugLockDir, 0755))

	discovered, found, err := DiscoverLockDir(manifestPath, "billing", "")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, slugLockDir, discovered)
}


