package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModules_DataProduct_UniversalInternalDatasetAndAuthorizedViews(t *testing.T) {
	dpDir := filepath.Join("..", "..", "modules", "data_product")

	parser := hclparse.NewParser()
	mainFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "main.tf"))
	require.False(t, diags.HasErrors(), "failed parsing main.tf: %v", diags)
	mainBody, ok := mainFile.Body.(*hclsyntax.Body)
	require.True(t, ok, "main.tf body must be hclsyntax.Body")

	dltFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "dlt_ports.tf"))
	require.False(t, diags.HasErrors(), "failed parsing dlt_ports.tf: %v", diags)
	dltBody, ok := dltFile.Body.(*hclsyntax.Body)
	require.True(t, ok, "dlt_ports.tf body must be hclsyntax.Body")

	varsFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "variables.tf"))
	require.False(t, diags.HasErrors(), "failed parsing variables.tf: %v", diags)
	varsBody, ok := varsFile.Body.(*hclsyntax.Body)
	require.True(t, ok, "variables.tf body must be hclsyntax.Body")

	outputsFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "outputs.tf"))
	require.False(t, diags.HasErrors(), "failed parsing outputs.tf: %v", diags)
	outputsBody, ok := outputsFile.Body.(*hclsyntax.Body)
	require.True(t, ok, "outputs.tf body must be hclsyntax.Body")

	t.Run("AC-7.1: Both internal and public datasets declared", func(t *testing.T) {
		var foundInternal, foundPublic bool
		for _, block := range mainBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_bigquery_dataset" {
				switch block.Labels[1] {
				case "internal":
					foundInternal = true
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["project"].Expr), "var.project_id")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr), "local.internal_dataset_id")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["location"].Expr), "var.region")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["delete_contents_on_destroy"].Expr), "!var.bigquery_deletion_protection")
				case "public":
					foundPublic = true
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["project"].Expr), "var.project_id")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr), "local.public_dataset_id")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["location"].Expr), "var.region")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["delete_contents_on_destroy"].Expr), "!var.bigquery_deletion_protection")
				}
			}
		}
		assert.True(t, foundInternal, "google_bigquery_dataset.internal must be declared")
		assert.True(t, foundPublic, "google_bigquery_dataset.public must be declared")
	})

	t.Run("AC-7.2: $O(1)$ Authorized Dataset Delegation", func(t *testing.T) {
		var foundDelegation bool
		for _, block := range mainBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_bigquery_dataset_access" && block.Labels[1] == "internal_views_delegation" {
				foundDelegation = true
				assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr), "google_bigquery_dataset.internal.dataset_id")
				require.Len(t, block.Body.Blocks, 1, "dataset_access must have 1 dataset block")
				dsOuter := block.Body.Blocks[0]
				assert.Equal(t, "dataset", dsOuter.Type)
				require.Len(t, dsOuter.Body.Blocks, 1, "dataset block must have nested dataset block")
				dsInner := dsOuter.Body.Blocks[0]
				assert.Contains(t, exprToString(mainFile.Bytes, dsInner.Body.Attributes["dataset_id"].Expr), "google_bigquery_dataset.public.dataset_id")
				assert.Contains(t, exprToString(mainFile.Bytes, dsOuter.Body.Attributes["target_types"].Expr), `"VIEWS"`)
			}
		}
		assert.True(t, foundDelegation, "google_bigquery_dataset_access.internal_views_delegation must be declared")
	})

	t.Run("AC-7.1 & AC-7.2: Boundary inputs landing tables and public authorized views", func(t *testing.T) {
		var foundInputs, foundOutputs bool
		for _, block := range mainBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_bigquery_table" {
				switch block.Labels[1] {
				case "inputs":
					foundInputs = true
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["for_each"].Expr), "local.internal_input_tables")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr), "google_bigquery_dataset.internal.dataset_id")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["schema"].Expr), "each.value.schema_json")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["deletion_protection"].Expr), "var.bigquery_deletion_protection")
				case "outputs":
					foundOutputs = true
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["for_each"].Expr), "local.public_output_views")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr), "google_bigquery_dataset.public.dataset_id")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["table_id"].Expr), "each.value.name")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["deletion_protection"].Expr), "var.bigquery_deletion_protection")
					assert.NotContains(t, block.Body.Attributes, "schema", "google_bigquery_table.outputs must NOT declare static schema")
					require.Len(t, block.Body.Blocks, 1, "outputs must have 1 view block")
					viewBlock := block.Body.Blocks[0]
					assert.Equal(t, "view", viewBlock.Type)
					assert.Contains(t, exprToString(mainFile.Bytes, viewBlock.Body.Attributes["query"].Expr), "each.value.view_query")
					assert.Contains(t, exprToString(mainFile.Bytes, viewBlock.Body.Attributes["use_legacy_sql"].Expr), "false")

					// Race-free depends_on ordering check
					require.Contains(t, block.Body.Attributes, "depends_on", "outputs must define depends_on")
					depStr := exprToString(mainFile.Bytes, block.Body.Attributes["depends_on"].Expr)
					assert.Contains(t, depStr, "google_bigquery_dataset_access.internal_views_delegation")
					assert.Contains(t, depStr, "google_bigquery_table.inputs")
				}
			}
		}
		assert.True(t, foundInputs, "google_bigquery_table.inputs must be declared")
		assert.True(t, foundOutputs, "google_bigquery_table.outputs must be declared")
	})

	t.Run("AC-7.3: Least-Privilege IAM separation & project leakage remediation", func(t *testing.T) {
		// Verify project-level dlt_runner_bq_editor is removed from dlt_ports.tf
		for _, block := range dltBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_project_iam_member" && block.Labels[1] == "dlt_runner_bq_editor" {
				t.Fatalf("dlt_ports.tf must NOT contain google_project_iam_member.dlt_runner_bq_editor (#1219)")
			}
		}

		var foundRunnersEditor, foundConsumersViewer bool
		for _, block := range mainBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_bigquery_dataset_iam_member" {
				switch block.Labels[1] {
				case "runners_data_editor":
					foundRunnersEditor = true
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr), "google_bigquery_dataset.internal.dataset_id")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["role"].Expr), `"roles/bigquery.dataEditor"`)
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["for_each"].Expr), "local.all_runner_members")
				case "consumers_data_viewer":
					foundConsumersViewer = true
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr), "google_bigquery_dataset.public.dataset_id")
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["role"].Expr), `"roles/bigquery.dataViewer"`)
					assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["for_each"].Expr), "local.all_consumer_members")
				}
			}
		}
		assert.True(t, foundRunnersEditor, "google_bigquery_dataset_iam_member.runners_data_editor must be declared")
		assert.True(t, foundConsumersViewer, "google_bigquery_dataset_iam_member.consumers_data_viewer must be declared")
	})

	t.Run("AC-7.4: Ingress targets and plan-time determinism", func(t *testing.T) {
		for _, block := range mainBody.Blocks {
			if block.Type == "module" && len(block.Labels) == 1 && block.Labels[0] == "gateway" {
				assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["bigquery_dataset_id"].Expr), "local.internal_dataset_id")
			}
		}

		// dlt_bigquery_dataset in dlt_ports.tf
		for _, block := range dltBody.Blocks {
			if block.Type == "locals" {
				if attr, ok := block.Body.Attributes["dlt_bigquery_dataset"]; ok {
					assert.Contains(t, exprToString(dltFile.Bytes, attr.Expr), "local.internal_dataset_id")
				}
			}
		}

		// Check lock_dir passed to data source
		for _, block := range mainBody.Blocks {
			if block.Type == "data" && len(block.Labels) == 2 && block.Labels[0] == "supercargo_data_product" && block.Labels[1] == "this" {
				require.Contains(t, block.Body.Attributes, "lock_dir", "data.supercargo_data_product.this must declare lock_dir = var.lock_dir")
				assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["lock_dir"].Expr), "var.lock_dir")
			}
		}
	})

	t.Run("AC-7.1 & AC-7.5: Variables and Outputs", func(t *testing.T) {
		expectedVars := []string{
			"internal_bigquery_dataset_id",
			"lock_dir",
			"authorized_consumers",
			"runner_service_accounts",
		}
		for _, ev := range expectedVars {
			var found bool
			for _, block := range varsBody.Blocks {
				if block.Type == "variable" && len(block.Labels) == 1 && block.Labels[0] == ev {
					found = true
					break
				}
			}
			assert.True(t, found, "variable %q must be declared in variables.tf", ev)
		}

		expectedOutputs := []string{
			"internal_dataset_id",
			"public_dataset_id",
			"authorized_view_table_ids",
			"landing_table_ids",
		}
		for _, eo := range expectedOutputs {
			var found bool
			for _, block := range outputsBody.Blocks {
				if block.Type == "output" && len(block.Labels) == 1 && block.Labels[0] == eo {
					found = true
					break
				}
			}
			assert.True(t, found, "output %q must be declared in outputs.tf", eo)
		}
	})
}

func createTestConfigWithLockDir(ctx context.Context, t *testing.T, s dschema.Schema, manifestFile, lockDir string) (tfsdk.Config, tfsdk.State) {
	m := newTestConfigModel(manifestFile, "", "", "")
	if lockDir != "" {
		m.LockDir = types.StringValue(lockDir)
	} else {
		m.LockDir = types.StringNull()
	}
	cfg := newTestDataSourceConfig(ctx, t, s, m)
	state := newTestDataSourceState(ctx, t, s, m)
	return cfg, state
}

func TestSupercargoDataProductDataSource_LockDirHandling(t *testing.T) {
	ctx := context.Background()
	ds := NewSupercargoDataProductDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	// 1. Explicit missing lock_dir returns fatal Diagnostic Error with remediation hint
	t.Run("explicit missing lock_dir returns diagnostic error", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, "dataproduct.yaml")
		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:orders
  version: v1.0.0
`), 0644))

		cfg, state := createTestConfigWithLockDir(ctx, t, schemaResp.Schema, manifestPath, filepath.Join(tempDir, "missing_lock"))
		req := datasource.ReadRequest{Config: cfg}
		resp := datasource.ReadResponse{State: state}
		ds.Read(ctx, req, &resp)

		require.True(t, resp.Diagnostics.HasError(), "must return error for missing explicit lock_dir")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Explicitly Configured Lock Directory Missing")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "sc build --output-dir")
	})

	// 2. Omitted lock_dir and absent .supercargo/lock returns empty maps gracefully
	t.Run("omitted lock_dir without lock files returns empty maps gracefully", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, "dataproduct.yaml")
		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:unmigrated
  version: v1.0.0
`), 0644))

		cfg, state := createTestConfigWithLockDir(ctx, t, schemaResp.Schema, manifestPath, "")
		req := datasource.ReadRequest{Config: cfg}
		resp := datasource.ReadResponse{State: state}
		ds.Read(ctx, req, &resp)

		require.False(t, resp.Diagnostics.HasError(), "must succeed gracefully without lock files: %v", resp.Diagnostics)
		var resState supercargoDataProductDataSourceModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &resState)...)
		require.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, "unmigrated_internal", resState.InternalDatasetID.ValueString())
		assert.Equal(t, "unmigrated", resState.PublicDatasetID.ValueString())
		assert.Len(t, resState.Outputs.Elements(), 0, "outputs map must be empty {}")
		assert.Len(t, resState.Inputs.Elements(), 0, "inputs map must be empty {}")
	})

	// 3. Auto-discovered or provided lock_dir populates outputs and inputs
	t.Run("valid lock_dir populates outputs and inputs", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, "dataproduct.yaml")
		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:orders
  version: v1.0.0
`), 0644))

		lockDir := filepath.Join(tempDir, ".supercargo", "lock", "orders")
		viewsDir := filepath.Join(lockDir, "views")
		schemasDir := filepath.Join(lockDir, "schemas")
		ingressDir := filepath.Join(lockDir, "ingress")
		require.NoError(t, os.MkdirAll(viewsDir, 0755))
		require.NoError(t, os.MkdirAll(schemasDir, 0755))
		require.NoError(t, os.MkdirAll(ingressDir, 0755))

		viewSQL := "CREATE OR REPLACE VIEW `acme.orders.orders_replica` AS\nSELECT order_id, amount FROM `acme.orders_internal.orders`;"
		require.NoError(t, os.WriteFile(filepath.Join(viewsDir, "orders_replica.sql"), []byte(viewSQL), 0644))

		ingressYAML := `meta:
  urn: urn:supercargo:hub:contract:orders-internal-stream:v1
`
		require.NoError(t, os.WriteFile(filepath.Join(ingressDir, "stream.contract.yaml"), []byte(ingressYAML), 0644))

		schemaJSON := `[{"name":"order_id","type":"STRING","mode":"REQUIRED"}]`
		require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "orders_internal.orders.schema.json"), []byte(schemaJSON), 0644))

		cfg, state := createTestConfigWithLockDir(ctx, t, schemaResp.Schema, manifestPath, "")
		req := datasource.ReadRequest{Config: cfg}
		resp := datasource.ReadResponse{State: state}
		ds.Read(ctx, req, &resp)

		require.False(t, resp.Diagnostics.HasError(), "must succeed: %v", resp.Diagnostics)
		var resState supercargoDataProductDataSourceModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &resState)...)
		require.False(t, resp.Diagnostics.HasError())
		assert.Equal(t, "orders_internal", resState.InternalDatasetID.ValueString())
		assert.Equal(t, "orders", resState.PublicDatasetID.ValueString())
		assert.Len(t, resState.Outputs.Elements(), 1)
		assert.Len(t, resState.Inputs.Elements(), 1)
	})
}

func exprToString(fileBytes []byte, expr hclsyntax.Expression) string {
	rng := expr.Range()
	return strings.TrimSpace(string(fileBytes[rng.Start.Byte:rng.End.Byte]))
}

func TestModules_DataProduct_PlanTimeDeterminismAndNegativeIAM(t *testing.T) {
	dpDir := filepath.Join("..", "..", "modules", "data_product")
	parser := hclparse.NewParser()

	tfFiles, err := filepath.Glob(filepath.Join(dpDir, "*.tf"))
	require.NoError(t, err)

	for _, fPath := range tfFiles {
		f, diags := parser.ParseHCLFile(fPath)
		require.False(t, diags.HasErrors(), "failed parsing %s: %v", fPath, diags)
		body, ok := f.Body.(*hclsyntax.Body)
		require.True(t, ok)

		for _, block := range body.Blocks {
			for _, attrName := range []string{"for_each", "count"} {
				if attr, ok := block.Body.Attributes[attrName]; ok {
					exprStr := exprToString(f.Bytes, attr.Expr)
					assert.NotContains(t, exprStr, "module.gateway.service_account_email", "Plan-time determinism violation in %s: %s contains module.gateway.service_account_email", fPath, attrName)
					assert.NotContains(t, exprStr, "module.gateway.service_account_id", "Plan-time determinism violation in %s: %s contains module.gateway.service_account_id", fPath, attrName)
				}
			}
		}
	}

	mainFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "main.tf"))
	require.False(t, diags.HasErrors())
	mainBody := mainFile.Body.(*hclsyntax.Body)

	for _, block := range mainBody.Blocks {
		if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_bigquery_dataset_iam_member" {
			switch block.Labels[1] {
			case "runners_data_editor":
				dsExpr := exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr)
				assert.Contains(t, dsExpr, "google_bigquery_dataset.internal.dataset_id")
				feExpr := exprToString(mainFile.Bytes, block.Body.Attributes["for_each"].Expr)
				assert.Contains(t, feExpr, "local.all_runner_members")
				assert.NotContains(t, feExpr, "consumer", "runners_data_editor must not iterate over consumers")
			case "consumers_data_viewer":
				dsExpr := exprToString(mainFile.Bytes, block.Body.Attributes["dataset_id"].Expr)
				assert.Contains(t, dsExpr, "google_bigquery_dataset.public.dataset_id")
				feExpr := exprToString(mainFile.Bytes, block.Body.Attributes["for_each"].Expr)
				assert.Contains(t, feExpr, "local.all_consumer_members")
				assert.NotContains(t, feExpr, "runner", "consumers_data_viewer must not iterate over runners")
			}
		}
	}
}

func TestSupercargoDataProductDataSource_MultiLensAndValidation(t *testing.T) {
	ctx := context.Background()
	ds := NewSupercargoDataProductDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	t.Run("5 lenses and bridge view extraction", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, "dataproduct.yaml")
		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:orders
  version: v1.0.0
`), 0644))

		lockDir := filepath.Join(tempDir, ".supercargo", "lock", "orders")
		viewsDir := filepath.Join(lockDir, "views")
		require.NoError(t, os.MkdirAll(viewsDir, 0755))

		lensViews := []string{
			"orders_event_log",
			"orders_replica",
			"orders_scd2",
			"orders_cdc",
			"orders_reidentified",
			"orders_v1",
		}
		for _, lv := range lensViews {
			sql := "CREATE OR REPLACE VIEW `acme.orders." + lv + "` AS SELECT 1 AS col;"
			require.NoError(t, os.WriteFile(filepath.Join(viewsDir, lv+".sql"), []byte(sql), 0644))
		}

		cfg, state := createTestConfigWithLockDir(ctx, t, schemaResp.Schema, manifestPath, "")
		req := datasource.ReadRequest{Config: cfg}
		resp := datasource.ReadResponse{State: state}
		ds.Read(ctx, req, &resp)

		require.False(t, resp.Diagnostics.HasError(), "must succeed: %v", resp.Diagnostics)
		var resState supercargoDataProductDataSourceModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &resState)...)
		require.False(t, resp.Diagnostics.HasError())
		assert.Len(t, resState.Outputs.Elements(), 6)
	})

	t.Run("unsupported materialization returns fatal diagnostic error", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, "dataproduct.yaml")
		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:orders
  version: v1.0.0
`), 0644))

		lockDir := filepath.Join(tempDir, ".supercargo", "lock", "orders")
		viewsDir := filepath.Join(lockDir, "views")
		require.NoError(t, os.MkdirAll(viewsDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(viewsDir, "orders_custom.sql"), []byte("SELECT 1;"), 0644))

		odpsYAML := `id: urn:supercargo:hub:product:orders:v1
output_ports:
  - name: orders_custom
    lens: replica
    materialization: materialized_table
`
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "dataproduct.odps.yaml"), []byte(odpsYAML), 0644))

		cfg, state := createTestConfigWithLockDir(ctx, t, schemaResp.Schema, manifestPath, "")
		req := datasource.ReadRequest{Config: cfg}
		resp := datasource.ReadResponse{State: state}
		ds.Read(ctx, req, &resp)

		require.True(t, resp.Diagnostics.HasError(), "unsupported materialization must fail")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Invalid Materialization")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "materialized_table")
	})

	t.Run("duplicate port key returns fatal diagnostic error", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, "dataproduct.yaml")
		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:orders
  version: v1.0.0
`), 0644))

		lockDir := filepath.Join(tempDir, ".supercargo", "lock", "orders")
		viewsDir := filepath.Join(lockDir, "views")
		contractsDir := filepath.Join(lockDir, "contracts")
		require.NoError(t, os.MkdirAll(viewsDir, 0755))
		require.NoError(t, os.MkdirAll(contractsDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(viewsDir, "port_a.sql"), []byte("SELECT 1;"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(viewsDir, "port_b.sql"), []byte("SELECT 2;"), 0644))

		sameURN := "urn:supercargo:hub:contract:orders-dup:v1"
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "port_a.odcs.yaml"), []byte("id: "+sameURN+"\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "port_b.odcs.yaml"), []byte("id: "+sameURN+"\n"), 0644))

		cfg, state := createTestConfigWithLockDir(ctx, t, schemaResp.Schema, manifestPath, "")
		req := datasource.ReadRequest{Config: cfg}
		resp := datasource.ReadResponse{State: state}
		ds.Read(ctx, req, &resp)

		require.True(t, resp.Diagnostics.HasError(), "duplicate key must fail")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Summary(), "Duplicate Collection Key")
		assert.Contains(t, resp.Diagnostics.Errors()[0].Detail(), sameURN)
	})
}

