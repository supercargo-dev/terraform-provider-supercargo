package manifest

import (
	"encoding/json"
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
}

