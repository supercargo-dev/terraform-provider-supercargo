package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supercargo-dev/core/pkg/validation"
)

func TestSupercargoDataProduct_PAMEntitlements(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ds := NewSupercargoDataProductDataSource()
	var schemaResp datasource.SchemaResponse
	ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	t.Run("plan-time deterministic pam_entitlements for view-backed and transform-backed outputs", func(t *testing.T) {
		t.Parallel()
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

		odpsYAML := `apiVersion: v1.1.0
kind: DataProduct
id: urn:supercargo:hub:product:orders
outputPorts:
  - name: orders_reidentified
    contractUrn: urn:supercargo:hub:contract:orders-orders_reidentified:v1
    lens: reidentified
  - name: orders_mart
    contractUrn: urn:supercargo:hub:contract:orders-orders_mart:v1
  - name: orders_public
    contractUrn: urn:supercargo:hub:contract:orders-orders_public:v1
    lens: replica
`
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, "dataproduct.odps.yaml"), []byte(odpsYAML), 0644))

		// View-backed outputs only (orders_mart is transform-backed with no views/*.sql)
		require.NoError(t, os.WriteFile(
			filepath.Join(viewsDir, "orders_reidentified.sql"),
			[]byte("CREATE OR REPLACE VIEW `p.orders.orders_reidentified` AS SELECT id FROM `p.orders_internal.raw`;"),
			0644,
		))
		require.NoError(t, os.WriteFile(
			filepath.Join(viewsDir, "orders_public.sql"),
			[]byte("CREATE OR REPLACE VIEW `p.orders.orders_public` AS SELECT id FROM `p.orders_internal.raw`;"),
			0644,
		))

		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "orders_reidentified.odcs.yaml"), []byte(`id: urn:supercargo:hub:contract:orders-orders_reidentified:v1
description:
  purpose: Reidentified fraud analytics
  usage: PAM JIT only
  limitations: No CSV export
roles:
  - role: fraud_analyst
    access: read
    firstLevelApprovers: risk-team
customProperties:
  - property: x-sc-lens
    value: reidentified
  - property: x-sc-sensitivity
    value: RESTRICTED
`), 0644))

		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "orders_mart.odcs.yaml"), []byte(`id: urn:supercargo:hub:contract:orders-orders_mart:v1
description:
  purpose: Transform-backed finance mart
roles:
  - role: finance_viewer
    access: read
    firstLevelApprovers: finance-ops
customProperties:
  - property: x-sc-sensitivity
    value: CONFIDENTIAL
`), 0644))

		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "orders_public.odcs.yaml"), []byte(`id: urn:supercargo:hub:contract:orders-orders_public:v1
customProperties:
  - property: x-sc-lens
    value: replica
  - property: x-sc-sensitivity
    value: PUBLIC
`), 0644))

		cfg, state := createTestConfigWithLockDir(ctx, t, schemaResp.Schema, manifestPath, "")
		req := datasource.ReadRequest{Config: cfg}
		resp := datasource.ReadResponse{State: state}
		ds.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %v", resp.Diagnostics)

		var resState supercargoDataProductDataSourceModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &resState)...)
		require.False(t, resp.Diagnostics.HasError())

		// outputs must ONLY include view-backed ports (no orders_mart empty view_query pollution)
		assert.False(t, resState.Outputs.IsNull())
		assert.False(t, resState.Outputs.IsUnknown())
		var outputs map[string]supercargoOutputPortModel
		resp.Diagnostics.Append(resState.Outputs.ElementsAs(ctx, &outputs, false)...)
		require.False(t, resp.Diagnostics.HasError())
		require.Len(t, outputs, 2)
		assert.NotContains(t, outputs, "urn:supercargo:hub:contract:orders-orders_mart:v1")

		reidentOut := outputs["urn:supercargo:hub:contract:orders-orders_reidentified:v1"]
		assert.False(t, reidentOut.Sensitivity.IsNull())
		assert.False(t, reidentOut.Sensitivity.IsUnknown())
		assert.Equal(t, "RESTRICTED", reidentOut.Sensitivity.ValueString())
		assert.False(t, reidentOut.AuthorizedRoles.IsNull())
		assert.False(t, reidentOut.AuthorizedRoles.IsUnknown())
		assert.False(t, reidentOut.FirstLevelApprovers.IsNull())
		assert.False(t, reidentOut.FirstLevelApprovers.IsUnknown())

		// pam_entitlements must be plan-time known map keyed by port slug containing ALL 3 ports
		assert.False(t, resState.PAMEntitlements.IsNull())
		assert.False(t, resState.PAMEntitlements.IsUnknown())

		var pamMap map[string]supercargoPAMEntitlementModel
		resp.Diagnostics.Append(resState.PAMEntitlements.ElementsAs(ctx, &pamMap, false)...)
		require.False(t, resp.Diagnostics.HasError())
		require.Len(t, pamMap, 3)

		for slug, ent := range pamMap {
			assert.NotEmpty(t, slug)
			assert.False(t, ent.EntitlementID.IsNull() || ent.EntitlementID.IsUnknown())
			assert.False(t, ent.PortName.IsNull() || ent.PortName.IsUnknown())
			assert.False(t, ent.PortURN.IsNull() || ent.PortURN.IsUnknown())
			assert.False(t, ent.Lens.IsNull() || ent.Lens.IsUnknown())
			assert.False(t, ent.Sensitivity.IsNull() || ent.Sensitivity.IsUnknown())
			assert.False(t, ent.AuthorizedRoles.IsNull() || ent.AuthorizedRoles.IsUnknown())
			assert.False(t, ent.FirstLevelApprovers.IsNull() || ent.FirstLevelApprovers.IsUnknown())
			assert.False(t, ent.RequiresPAM.IsNull() || ent.RequiresPAM.IsUnknown())
			assert.False(t, ent.RequiresKMSDecrypt.IsNull() || ent.RequiresKMSDecrypt.IsUnknown())
			assert.False(t, ent.TermsPurpose.IsNull() || ent.TermsPurpose.IsUnknown())
			assert.False(t, ent.TermsUsage.IsNull() || ent.TermsUsage.IsUnknown())
			assert.False(t, ent.TermsLimitations.IsNull() || ent.TermsLimitations.IsUnknown())
		}

		reident := pamMap["orders_reidentified"]
		assert.Equal(t, validation.SanitizePAMIdentifier("sc-orders-orders_reidentified"), reident.EntitlementID.ValueString())
		assert.Equal(t, "reidentified", reident.Lens.ValueString())
		assert.Equal(t, "RESTRICTED", reident.Sensitivity.ValueString())
		assert.True(t, reident.RequiresPAM.ValueBool())
		assert.True(t, reident.RequiresKMSDecrypt.ValueBool())
		assert.Equal(t, "Reidentified fraud analytics", reident.TermsPurpose.ValueString())
		assert.Equal(t, "PAM JIT only", reident.TermsUsage.ValueString())
		assert.Equal(t, "No CSV export", reident.TermsLimitations.ValueString())
		assert.Equal(t, []attr.Value{types.StringValue("fraud_analyst")}, reident.AuthorizedRoles.Elements())
		assert.Equal(t, []attr.Value{types.StringValue("risk-team")}, reident.FirstLevelApprovers.Elements())

		mart := pamMap["orders_mart"]
		assert.Equal(t, validation.SanitizePAMIdentifier("sc-orders-orders_mart"), mart.EntitlementID.ValueString())
		assert.Equal(t, "CONFIDENTIAL", mart.Sensitivity.ValueString())
		assert.True(t, mart.RequiresPAM.ValueBool())
		assert.False(t, mart.RequiresKMSDecrypt.ValueBool())
		assert.Equal(t, "Transform-backed finance mart", mart.TermsPurpose.ValueString())

		pub := pamMap["orders_public"]
		assert.Equal(t, "PUBLIC", pub.Sensitivity.ValueString())
		assert.False(t, pub.RequiresPAM.ValueBool())
		assert.False(t, pub.RequiresKMSDecrypt.ValueBool())
		assert.Empty(t, pub.AuthorizedRoles.Elements())
	})

	t.Run("no lockdir returns deterministic empty non-null pam_entitlements map", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, "dataproduct.yaml")
		require.NoError(t, os.WriteFile(manifestPath, []byte(`meta:
  urn: urn:supercargo:hub:product:orders
  version: v1.0.0
`), 0644))

		cfg, state := createTestConfigWithLockDir(ctx, t, schemaResp.Schema, manifestPath, "")
		req := datasource.ReadRequest{Config: cfg}
		resp := datasource.ReadResponse{State: state}
		ds.Read(ctx, req, &resp)
		require.False(t, resp.Diagnostics.HasError())

		var resState supercargoDataProductDataSourceModel
		resp.Diagnostics.Append(resp.State.Get(ctx, &resState)...)
		require.False(t, resp.Diagnostics.HasError())
		assert.False(t, resState.PAMEntitlements.IsNull())
		assert.False(t, resState.PAMEntitlements.IsUnknown())
		assert.Empty(t, resState.PAMEntitlements.Elements())
	})
}
