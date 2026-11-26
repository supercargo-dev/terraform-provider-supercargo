package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supercargo-dev/core/pkg/validation"
)

func TestLoadCompiledPAMEntitlements_TransformBackedAndViews(t *testing.T) {
	t.Parallel()

	lockDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(lockDir, "contracts"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(lockDir, "views"), 0755))

	// ODPS lists both view-backed outputs AND a transform-backed output (no views/*.sql)
	odpsYAML := `apiVersion: v1.1.0
kind: DataProduct
id: urn:supercargo:hub:product:orders
info:
  title: Orders
  version: 1.0.0
  ownerTeam: orders-domain
outputPorts:
  - name: orders_reidentified
    contractUrn: urn:supercargo:hub:contract:orders-orders_reidentified:v1
    lens: reidentified
  - name: orders_mart
    contractUrn: urn:supercargo:hub:contract:orders-orders_mart:v1
  - name: orders_public_replica
    contractUrn: urn:supercargo:hub:contract:orders-orders_public_replica:v1
    lens: replica
`
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "dataproduct.odps.yaml"), []byte(odpsYAML), 0644))

	// Only view-backed ports have views/*.sql (orders_mart has NO view SQL)
	require.NoError(t, os.WriteFile(
		filepath.Join(lockDir, "views", "orders_reidentified.sql"),
		[]byte("CREATE OR REPLACE VIEW `p.orders.orders_reidentified` AS SELECT id, email FROM `p.orders_internal.raw_orders`;"),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(lockDir, "views", "orders_public_replica.sql"),
		[]byte("CREATE OR REPLACE VIEW `p.orders.orders_public_replica` AS SELECT id FROM `p.orders_internal.raw_orders`;"),
		0644,
	))

	reidentContract := `id: urn:supercargo:hub:contract:orders-orders_reidentified:v1
apiVersion: v3.2.0
kind: DataContract
name: orders_reidentified
version: 1.0.0
status: active
description:
  purpose: Reidentified order analytics
  usage: Approved fraud analysts only via PAM JIT
  limitations: No bulk CSV export
roles:
  - role: fraud_analyst
    access: read
    firstLevelApprovers: risk-team
  - role: reader
    access: read
    firstLevelApprovers: orders-domain
customProperties:
  - property: x-sc-lens
    value: reidentified
  - property: x-sc-sensitivity
    value: RESTRICTED
`
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "contracts", "orders_reidentified.odcs.yaml"), []byte(reidentContract), 0644))

	martContract := `id: urn:supercargo:hub:contract:orders-orders_mart:v1
apiVersion: v3.2.0
kind: DataContract
name: orders_mart
version: 1.0.0
status: active
description:
  purpose: Curated dbt order mart
  usage: Finance & risk reporting
  limitations: 1h max grant
roles:
  - role: finance_analyst
    access: read
    firstLevelApprovers: finance-governance
customProperties:
  - property: x-sc-sensitivity
    value: CONFIDENTIAL
`
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "contracts", "orders_mart.odcs.yaml"), []byte(martContract), 0644))

	publicContract := `id: urn:supercargo:hub:contract:orders-orders_public_replica:v1
apiVersion: v3.2.0
kind: DataContract
name: orders_public_replica
version: 1.0.0
status: active
description:
  purpose: Public order counts
customProperties:
  - property: x-sc-lens
    value: replica
  - property: x-sc-sensitivity
    value: PUBLIC
`
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "contracts", "orders_public_replica.odcs.yaml"), []byte(publicContract), 0644))

	// 1. LoadCompiledOutputs MUST NOT include transform-backed orders_mart (no empty view_query pollution)
	outputs, err := LoadCompiledOutputs(lockDir, "orders")
	require.NoError(t, err)
	require.Len(t, outputs, 2, "LoadCompiledOutputs must only return physical view-backed outputs")
	reidentOut, ok := outputs["urn:supercargo:hub:contract:orders-orders_reidentified:v1"]
	require.True(t, ok)
	assert.NotEmpty(t, reidentOut.ViewQuery)
	assert.Equal(t, "RESTRICTED", reidentOut.Sensitivity)
	assert.Equal(t, []string{"fraud_analyst", "reader"}, reidentOut.AuthorizedRoles)
	assert.Equal(t, []string{"orders-domain", "risk-team"}, reidentOut.FirstLevelApprovers)

	// 2. LoadCompiledPAMEntitlements MUST capture ALL 3 outputs (both view-backed AND transform-backed)
	pamMap, err := LoadCompiledPAMEntitlements(lockDir, "orders")
	require.NoError(t, err)
	require.Len(t, pamMap, 3)

	reidentPAM, ok := pamMap["orders_reidentified"]
	require.True(t, ok)
	assert.Equal(t, validation.SanitizePAMIdentifier("sc-orders-orders_reidentified"), reidentPAM.EntitlementID)
	assert.Equal(t, "orders_reidentified", reidentPAM.PortName)
	assert.Equal(t, "urn:supercargo:hub:contract:orders-orders_reidentified:v1", reidentPAM.PortURN)
	assert.Equal(t, "reidentified", reidentPAM.Lens)
	assert.Equal(t, "RESTRICTED", reidentPAM.Sensitivity)
	assert.Equal(t, []string{"fraud_analyst", "reader"}, reidentPAM.AuthorizedRoles)
	assert.Equal(t, []string{"orders-domain", "risk-team"}, reidentPAM.FirstLevelApprovers)
	assert.True(t, reidentPAM.RequiresPAM)
	assert.True(t, reidentPAM.RequiresKMSDecrypt)
	assert.Equal(t, "Reidentified order analytics", reidentPAM.TermsPurpose)
	assert.Equal(t, "Approved fraud analysts only via PAM JIT", reidentPAM.TermsUsage)
	assert.Equal(t, "No bulk CSV export", reidentPAM.TermsLimitations)

	martPAM, ok := pamMap["orders_mart"]
	require.True(t, ok, "transform-backed output without views/*.sql must still appear in pam_entitlements")
	assert.Equal(t, validation.SanitizePAMIdentifier("sc-orders-orders_mart"), martPAM.EntitlementID)
	assert.Equal(t, "orders_mart", martPAM.PortName)
	assert.Equal(t, "urn:supercargo:hub:contract:orders-orders_mart:v1", martPAM.PortURN)
	assert.Equal(t, "CONFIDENTIAL", martPAM.Sensitivity)
	assert.Equal(t, []string{"finance_analyst"}, martPAM.AuthorizedRoles)
	assert.Equal(t, []string{"finance-governance"}, martPAM.FirstLevelApprovers)
	assert.True(t, martPAM.RequiresPAM)
	assert.False(t, martPAM.RequiresKMSDecrypt)
	assert.Equal(t, "Curated dbt order mart", martPAM.TermsPurpose)

	pubPAM, ok := pamMap["orders_public_replica"]
	require.True(t, ok)
	assert.Equal(t, "PUBLIC", pubPAM.Sensitivity)
	assert.False(t, pubPAM.RequiresPAM)
	assert.False(t, pubPAM.RequiresKMSDecrypt)
}

func TestLoadCompiledPAMEntitlements_ParityAndEdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		productSlug     string
		odpsYAML        string
		contractName    string
		contractYAML    string
		wantSensitivity string
		wantLens        string
		wantRequiresPAM bool
		wantRequiresKMS bool
		wantPurpose     string
		wantUsage       string
		wantLimitations string
		wantRoles       []string
	}{
		{
			name:        "long slug truncation 1:1 parity with SanitizePAMIdentifier",
			productSlug: "enterprise_customer_billing_and_payments_platform",
			odpsYAML: `output_ports:
  - name: Customer.Orders__Reidentified_V1
    lens: reidentified
`,
			contractName: "Customer.Orders__Reidentified_V1",
			contractYAML: `id: urn:supercargo:hub:contract:long:v1
customProperties:
  - property: x-sc-lens
    value: reidentified
`,
			wantSensitivity: "",
			wantLens:        "reidentified",
			wantRequiresPAM: true,
			wantRequiresKMS: true,
		},
		{
			name:        "field classification and pii tag elevate sensitivity without x-sc-sensitivity",
			productSlug: "users",
			odpsYAML: `outputPorts:
  - name: user_profiles
    contractUrn: urn:supercargo:hub:contract:users-user_profiles:v1
`,
			contractName: "user_profiles",
			contractYAML: `id: urn:supercargo:hub:contract:users-user_profiles:v1
description: User profile dataset
schema:
  - name: user_profiles
    properties:
      - name: ssn
        classification: restricted
        tags:
          - pii:true
`,
			wantSensitivity: "RESTRICTED",
			wantRequiresPAM: true,
			wantRequiresKMS: false,
			wantPurpose:     "User profile dataset",
		},
		{
			name:        "public port with explicit authorized role triggers requires_pam",
			productSlug: "metrics",
			odpsYAML: `outputPorts:
  - name: revenue_metrics
`,
			contractName: "revenue_metrics",
			contractYAML: `id: urn:supercargo:hub:contract:metrics-revenue_metrics:v1
terms_of_use:
  purpose: Executive revenue metrics
  usage: Board reporting
  limitations: Internal only
roles:
  - role: finance_exec
    firstLevelApprovers: cfo-office
customProperties:
  - property: x-sc-sensitivity
    value: PUBLIC
`,
			wantSensitivity: "PUBLIC",
			wantRequiresPAM: true,
			wantRequiresKMS: false,
			wantPurpose:     "Executive revenue metrics",
			wantUsage:       "Board reporting",
			wantLimitations: "Internal only",
			wantRoles:       []string{"finance_exec"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lockDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(lockDir, "contracts"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(lockDir, "dataproduct.odps.yaml"), []byte(tc.odpsYAML), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(lockDir, "contracts", tc.contractName+".odcs.yaml"), []byte(tc.contractYAML), 0644))

			got, err := LoadCompiledPAMEntitlements(lockDir, tc.productSlug)
			require.NoError(t, err)
			ent, ok := got[tc.contractName]
			require.True(t, ok, "expected port %q in result", tc.contractName)

			expectedID := validation.SanitizePAMIdentifier("sc-" + tc.productSlug + "-" + tc.contractName)
			assert.Equal(t, expectedID, ent.EntitlementID)
			assert.LessOrEqual(t, len(ent.EntitlementID), 63)
			assert.Equal(t, tc.wantSensitivity, ent.Sensitivity)
			assert.Equal(t, tc.wantLens, ent.Lens)
			assert.Equal(t, tc.wantRequiresPAM, ent.RequiresPAM)
			assert.Equal(t, tc.wantRequiresKMS, ent.RequiresKMSDecrypt)
			assert.Equal(t, tc.wantPurpose, ent.TermsPurpose)
			assert.Equal(t, tc.wantUsage, ent.TermsUsage)
			assert.Equal(t, tc.wantLimitations, ent.TermsLimitations)
			if tc.wantRoles != nil {
				assert.Equal(t, tc.wantRoles, ent.AuthorizedRoles)
			}
		})
	}
}

func TestLoadCompiledPAMEntitlements_ZeroBilateralIAMPrincipals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		role     string
		approver string
	}{
		{name: "user prefix in role", role: "user:alice@example.com", approver: "risk-team"},
		{name: "group prefix in role", role: "group:analysts@example.com", approver: "risk-team"},
		{name: "serviceAccount prefix in role", role: "serviceAccount:sa@proj.iam.gserviceaccount.com", approver: "risk-team"},
		{name: "domain prefix in role", role: "domain:example.com", approver: "risk-team"},
		{name: "uppercase USER prefix with bearer token", role: "USER:bob@example.com Bearer secret-jwt-xyz", approver: "risk-team"},
		{name: "user prefix in firstLevelApprovers", role: "reader", approver: "user:approver@example.com"},
		{name: "group prefix in firstLevelApprovers", role: "reader", approver: "group:sec@example.com"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lockDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(lockDir, "contracts"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(lockDir, "dataproduct.odps.yaml"), []byte("outputPorts:\n  - name: orders_reidentified\n"), 0644))
			contractYAML := "id: urn:supercargo:hub:contract:orders-orders_reidentified:v1\nroles:\n  - role: " + tc.role + "\n    firstLevelApprovers: " + tc.approver + "\n"
			require.NoError(t, os.WriteFile(filepath.Join(lockDir, "contracts", "orders_reidentified.odcs.yaml"), []byte(contractYAML), 0644))

			_, err := LoadCompiledPAMEntitlements(lockDir, "orders")
			require.Error(t, err, "bilateral IAM principal in ODCS must fail closed")
			assert.NotContains(t, err.Error(), "secret-jwt-xyz")
			if tc.name == "uppercase USER prefix with bearer token" {
				assert.Contains(t, err.Error(), "Bearer [REDACTED]")
			}
		})
	}
}
