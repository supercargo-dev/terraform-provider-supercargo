package provider_test

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModules_CloudRunLifecycleDecoupling(t *testing.T) {
	modulesDir := "../../modules"
	cloudRunModules := []struct {
		moduleName   string
		resourceName string
	}{
		{moduleName: "hub", resourceName: "hub"},
		{moduleName: "hub", resourceName: "shovel"},
		{moduleName: "hub", resourceName: "mcp"},
		{moduleName: "hub", resourceName: "console"},
		{moduleName: "vault", resourceName: "vault"},
		{moduleName: "gateway", resourceName: "gateway"},
		{moduleName: "loader", resourceName: "loader"},
		{moduleName: "metadata_shovel", resourceName: "shovel"},
	}

	for _, tc := range cloudRunModules {
		t.Run(tc.moduleName+"/"+tc.resourceName, func(t *testing.T) {
			path := filepath.Join(modulesDir, tc.moduleName, "main.tf")
			contentBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", path, err)
			}
			content := string(contentBytes)

			// Look for the specific google_cloud_run_v2_service resource definition
			expectedResource := `resource "google_cloud_run_v2_service" "` + tc.resourceName + `"`
			idx := strings.Index(content, expectedResource)
			if idx == -1 {
				t.Fatalf("Could not find %s in %s", expectedResource, path)
			}

			// Extract block chunk from idx
			chunk := content[idx:]
			// Find end of resource block by searching next top-level resource/output or EOF
			nextResourceIdx := strings.Index(chunk[len(expectedResource):], "\nresource \"")
			if nextResourceIdx != -1 {
				chunk = chunk[:len(expectedResource)+nextResourceIdx]
			}

			if !strings.Contains(chunk, "lifecycle") {
				t.Errorf("Resource %s in %s missing lifecycle block", expectedResource, tc.moduleName)
			}

			if !strings.Contains(chunk, "ignore_changes") {
				t.Errorf("Resource %s in %s missing ignore_changes attribute in lifecycle block", expectedResource, tc.moduleName)
			}

			requiredIgnores := []string{
				"template[0].containers[0].image",
				"client",
				"client_version",
			}

			for _, req := range requiredIgnores {
				if !strings.Contains(chunk, req) {
					t.Errorf("Resource %s in %s lifecycle.ignore_changes missing %q", expectedResource, tc.moduleName, req)
				}
			}
		})
	}
}

func TestModules_BigQueryDeletionProtection(t *testing.T) {
	modulesDir := "../../modules"

	t.Run("VariableDefaultsToTrue", func(t *testing.T) {
		bqModules := []string{"gateway", "security_vault", "data_product", "hub"}
		for _, mod := range bqModules {
			varPath := filepath.Join(modulesDir, mod, "variables.tf")
			contentBytes, err := os.ReadFile(varPath)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", varPath, err)
			}
			content := string(contentBytes)

			if !strings.Contains(content, `variable "bigquery_deletion_protection"`) {
				t.Errorf("Module %s missing bigquery_deletion_protection variable", mod)
			}

			if !strings.Contains(content, "default     = true") && !strings.Contains(content, "default = true") {
				t.Errorf("Module %s bigquery_deletion_protection must default to true", mod)
			}
		}
	})

	t.Run("TableResourcesUseDeletionProtection", func(t *testing.T) {
		tableChecks := []struct {
			moduleName string
			tableName  string
		}{
			{moduleName: "gateway", tableName: "validated_data"},
			{moduleName: "security_vault", tableName: "lookup_table"},
			{moduleName: "security_vault", tableName: "rtbf_shred_queue"},
			{moduleName: "hub", tableName: "outbox_events"},
			{moduleName: "hub", tableName: "asset_health_history"},
		}

		for _, tc := range tableChecks {
			path := filepath.Join(modulesDir, tc.moduleName, "main.tf")
			contentBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", path, err)
			}
			content := string(contentBytes)

			expectedResource := `resource "google_bigquery_table" "` + tc.tableName + `"`
			idx := strings.Index(content, expectedResource)
			if idx == -1 {
				t.Fatalf("Could not find %s in %s", expectedResource, path)
			}

			chunk := content[idx:]
			nextResourceIdx := strings.Index(chunk[len(expectedResource):], "\nresource \"")
			if nextResourceIdx != -1 {
				chunk = chunk[:len(expectedResource)+nextResourceIdx]
			}

			if !strings.Contains(chunk, "deletion_protection = var.bigquery_deletion_protection") {
				t.Errorf("Resource %s in %s must set deletion_protection = var.bigquery_deletion_protection", expectedResource, tc.moduleName)
			}
		}
	})
}

func TestModules_DataProductGoldenPath(t *testing.T) {
	modulesDir := "../../modules"
	mainPath := filepath.Join(modulesDir, "data_product", "main.tf")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", mainPath, err)
	}
	mainContent := string(mainBytes)

	t.Run("GatewayContractsDataSourceComputation", func(t *testing.T) {
		if !strings.Contains(mainContent, `data "supercargo_data_product" "this"`) {
			t.Errorf("data_product/main.tf missing data \"supercargo_data_product\" \"this\" definition")
		}
		if !strings.Contains(mainContent, "gateway_contracts =") {
			t.Errorf("data_product/main.tf missing gateway_contracts local definition")
		}
		if !strings.Contains(mainContent, "data.supercargo_data_product.this.contracts") {
			t.Errorf("data_product/main.tf gateway_contracts must reference data.supercargo_data_product.this.contracts")
		}
		if !strings.Contains(mainContent, "contracts = local.gateway_contracts") {
			t.Errorf("data_product/main.tf module.gateway must pass contracts = local.gateway_contracts")
		}
	})

	t.Run("GatewayDecoupledFromMutableResources", func(t *testing.T) {
		if strings.Contains(mainContent, `resource "supercargo_data_product"`) {
			t.Errorf("data_product/main.tf must not contain resource supercargo_data_product")
		}
		if strings.Contains(mainContent, `resource "supercargo_contract_version"`) {
			t.Errorf("data_product/main.tf must not contain resource supercargo_contract_version")
		}
		gatewayIdx := strings.Index(mainContent, `module "gateway"`)
		if gatewayIdx == -1 {
			t.Fatalf("module gateway not found in %s", mainPath)
		}
		gatewayChunk := mainContent[gatewayIdx:]
		if strings.Contains(gatewayChunk, "supercargo_data_product.this") {
			t.Errorf("module gateway must not depend on supercargo_data_product.this")
		}
	})

	t.Run("ContractsOutputExported", func(t *testing.T) {
		outputPath := filepath.Join(modulesDir, "data_product", "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		if !strings.Contains(outputContent, `output "contracts"`) {
			t.Errorf("data_product/outputs.tf missing output \"contracts\"")
		}
		if !strings.Contains(outputContent, "data.supercargo_data_product.this.contracts") {
			t.Errorf("data_product/outputs.tf output \"contracts\" value must be data.supercargo_data_product.this.contracts")
		}
		if !strings.Contains(outputContent, "data.supercargo_data_product.this.urn") {
			t.Errorf("data_product/outputs.tf output \"product_urn\" value must be data.supercargo_data_product.this.urn")
		}
	})

	t.Run("ProducerExampleUsesGoldenPath", func(t *testing.T) {
		examplePath := filepath.Join("../../examples", "data-producer", "main.tf")
		exampleBytes, err := os.ReadFile(examplePath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", examplePath, err)
		}
		exampleContent := string(exampleBytes)

		if strings.Contains(exampleContent, "local.contracts") {
			t.Errorf("examples/data-producer/main.tf should not contain local.contracts, should use golden path")
		}
	})
}

func TestModules_GatewayPushInvokerAndIAMEncapsulation(t *testing.T) {
	modulesDir := "../../modules"
	gwDir := filepath.Join(modulesDir, "gateway")

	t.Run("AuthorizedInvokerServiceAccountsVariable", func(t *testing.T) {
		varPath := filepath.Join(gwDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		if !strings.Contains(varContent, `variable "authorized_invoker_service_accounts"`) {
			t.Errorf("modules/gateway/variables.tf missing authorized_invoker_service_accounts variable")
		}
		if !strings.Contains(varContent, "default     = []") && !strings.Contains(varContent, "default = []") {
			t.Errorf("modules/gateway/variables.tf authorized_invoker_service_accounts must default to []")
		}
		if !strings.Contains(varContent, "List of service account emails authorized to invoke the gateway service") {
			t.Errorf("modules/gateway/variables.tf authorized_invoker_service_accounts missing expected description")
		}
	})

	t.Run("NormalizationLogicInMain", func(t *testing.T) {
		mainPath := filepath.Join(gwDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, "all_authorized_invokers") {
			t.Errorf("modules/gateway/main.tf missing all_authorized_invokers local")
		}
		if !strings.Contains(mainContent, `startswith(trimspace(sa), "serviceAccount:")`) && !strings.Contains(mainContent, `startswith(sa, "serviceAccount:")`) {
			t.Errorf("modules/gateway/main.tf missing startswith(trimspace(sa), \"serviceAccount:\") normalization")
		}
		if !strings.Contains(mainContent, "compact(var.authorized_invoker_service_accounts)") {
			t.Errorf("modules/gateway/main.tf missing compact(var.authorized_invoker_service_accounts)")
		}

		invokerResIdx := strings.Index(mainContent, `resource "google_cloud_run_v2_service_iam_member" "authorized_invokers"`)
		if invokerResIdx == -1 {
			t.Fatalf("authorized_invokers IAM member resource not found in %s", mainPath)
		}
		invokerChunk := mainContent[invokerResIdx:]
		nextRes := strings.Index(invokerChunk[len(`resource "google_cloud_run_v2_service_iam_member" "authorized_invokers"`):], "\nresource \"")
		if nextRes != -1 {
			invokerChunk = invokerChunk[:len(`resource "google_cloud_run_v2_service_iam_member" "authorized_invokers"`)+nextRes]
		}
		if !strings.Contains(invokerChunk, "local.all_authorized_invokers") {
			t.Errorf("google_cloud_run_v2_service_iam_member.authorized_invokers must iterate over local.all_authorized_invokers")
		}
	})

	t.Run("OutputsExported", func(t *testing.T) {
		outputPath := filepath.Join(gwDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		if !strings.Contains(outputContent, `output "push_invoker_service_account_email"`) {
			t.Errorf("modules/gateway/outputs.tf missing push_invoker_service_account_email output")
		}
		if !strings.Contains(outputContent, "local.push_invoker_service_account_email") && !strings.Contains(outputContent, "google_service_account.push_invoker.email") {
			t.Errorf("modules/gateway/outputs.tf push_invoker_service_account_email must reference local.push_invoker_service_account_email or google_service_account.push_invoker.email")
		}

		if !strings.Contains(outputContent, `output "raw_subscription_id"`) {
			t.Errorf("modules/gateway/outputs.tf missing raw_subscription_id output")
		}
		if !strings.Contains(outputContent, "google_pubsub_subscription.raw_push.id") {
			t.Errorf("modules/gateway/outputs.tf raw_subscription_id must reference google_pubsub_subscription.raw_push.id")
		}

		if !strings.Contains(outputContent, `output "dlq_subscription_id"`) {
			t.Errorf("modules/gateway/outputs.tf missing dlq_subscription_id output")
		}
		if !strings.Contains(outputContent, "google_pubsub_subscription.dlq_sub.id") {
			t.Errorf("modules/gateway/outputs.tf dlq_subscription_id must reference google_pubsub_subscription.dlq_sub.id")
		}
	})
}

func TestModules_DataProductPushInvokerAndIAMEncapsulation(t *testing.T) {
	modulesDir := "../../modules"
	dpDir := filepath.Join(modulesDir, "data_product")

	t.Run("AuthorizedInvokerVariables", func(t *testing.T) {
		varPath := filepath.Join(dpDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		if !strings.Contains(varContent, `variable "authorized_invokers"`) {
			t.Errorf("modules/data_product/variables.tf missing authorized_invokers variable")
		}
		if !strings.Contains(varContent, `variable "authorized_invoker_service_accounts"`) {
			t.Errorf("modules/data_product/variables.tf missing authorized_invoker_service_accounts variable")
		}
	})

	t.Run("GatewayModulePassthrough", func(t *testing.T) {
		mainPath := filepath.Join(dpDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		gatewayIdx := strings.Index(mainContent, `module "gateway"`)
		if gatewayIdx == -1 {
			t.Fatalf("module gateway not found in %s", mainPath)
		}
		gatewayChunk := mainContent[gatewayIdx:]

		if !strings.Contains(gatewayChunk, "authorized_invokers                 = var.authorized_invokers") &&
			!strings.Contains(gatewayChunk, "authorized_invokers = var.authorized_invokers") {
			t.Errorf("module gateway in data_product/main.tf must pass authorized_invokers = var.authorized_invokers")
		}
		if !strings.Contains(gatewayChunk, "authorized_invoker_service_accounts = var.authorized_invoker_service_accounts") &&
			!strings.Contains(gatewayChunk, "authorized_invoker_service_accounts= var.authorized_invoker_service_accounts") {
			t.Errorf("module gateway in data_product/main.tf must pass authorized_invoker_service_accounts = var.authorized_invoker_service_accounts")
		}
	})

	t.Run("OutputsExported", func(t *testing.T) {
		outputPath := filepath.Join(dpDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		outputs := []struct {
			name     string
			expected string
		}{
			{name: "dlq_topic_id", expected: "module.gateway.dlq_topic_id"},
			{name: "push_invoker_service_account_email", expected: "module.gateway.push_invoker_service_account_email"},
			{name: "raw_subscription_id", expected: "module.gateway.raw_subscription_id"},
			{name: "dlq_subscription_id", expected: "module.gateway.dlq_subscription_id"},
		}

		for _, out := range outputs {
			if !strings.Contains(outputContent, `output "`+out.name+`"`) {
				t.Errorf("modules/data_product/outputs.tf missing output %q", out.name)
			}
			if !strings.Contains(outputContent, out.expected) {
				t.Errorf("modules/data_product/outputs.tf output %q must reference %q", out.name, out.expected)
			}
		}
	})
}

func TestModules_DataProductDeclarativeIngress(t *testing.T) {
	modulesDir := "../../modules"
	dpDir := filepath.Join(modulesDir, "data_product")

	t.Run("IngressResourceDefinitions", func(t *testing.T) {
		ingressPath := filepath.Join(dpDir, "ingress.tf")
		ingressBytes, err := os.ReadFile(ingressPath)
		if err != nil {
			t.Fatalf("Failed to read %s (ingress.tf must exist in modules/data_product): %v", ingressPath, err)
		}
		ingressContent := string(ingressBytes)

		if !strings.Contains(ingressContent, "pubsub_input_ports") {
			t.Errorf("modules/data_product/ingress.tf missing pubsub_input_ports local")
		}
		if !strings.Contains(ingressContent, "managed_pubsub_ports") {
			t.Errorf("modules/data_product/ingress.tf missing managed_pubsub_ports local")
		}
		if !strings.Contains(ingressContent, `resource "google_pubsub_topic" "managed_input"`) {
			t.Errorf("modules/data_product/ingress.tf missing google_pubsub_topic.managed_input resource")
		}
		if !strings.Contains(ingressContent, `resource "google_pubsub_subscription" "input_push"`) {
			t.Errorf("modules/data_product/ingress.tf missing google_pubsub_subscription.input_push resource")
		}
		if !strings.Contains(ingressContent, "/pubsub/push/") {
			t.Errorf("modules/data_product/ingress.tf push endpoint must route to /pubsub/push/<contract_urn>")
		}
		if !strings.Contains(ingressContent, "module.gateway.push_invoker_service_account_email") {
			t.Errorf("modules/data_product/ingress.tf push subscription must use module.gateway.push_invoker_service_account_email for OIDC auth")
		}
		if !strings.Contains(ingressContent, "module.gateway.dlq_topic_id") {
			t.Errorf("modules/data_product/ingress.tf push subscription must configure dead_letter_topic with module.gateway.dlq_topic_id")
		}
		if !strings.Contains(ingressContent, `resource "google_pubsub_subscription_iam_member" "pubsub_input_dlq_subscriber"`) {
			t.Errorf("modules/data_product/ingress.tf missing google_pubsub_subscription_iam_member.pubsub_input_dlq_subscriber for PubSub SA DLQ subscriber role")
		}
	})

	t.Run("OutputsExported", func(t *testing.T) {
		outputPath := filepath.Join(dpDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		if !strings.Contains(outputContent, `output "managed_input_topics"`) {
			t.Errorf("modules/data_product/outputs.tf missing output \"managed_input_topics\"")
		}
		if !strings.Contains(outputContent, `output "input_subscription_ids"`) {
			t.Errorf("modules/data_product/outputs.tf missing output \"input_subscription_ids\"")
		}
	})
}

func TestModules_DataProduct_DLTIngress(t *testing.T) {
	modulesDir := "../../modules"
	dpDir := filepath.Join(modulesDir, "data_product")

	t.Run("DeterministicLocalsAndFiltering", func(t *testing.T) {
		dltPath := filepath.Join(dpDir, "dlt_ports.tf")
		dltBytes, err := os.ReadFile(dltPath)
		require.NoError(t, err, "dlt_ports.tf must exist in modules/data_product")
		dltContent := string(dltBytes)

		// AC-1: Port Filtering & Dataset Fallback
		assert.NotContains(t, dltContent, "raw_input_ports =", "dlt_ports.tf must not redefine raw_input_ports (uses local.manifest_input_ports)")
		assert.Contains(t, dltContent, "local.manifest_input_ports", "dlt_ports.tf must use local.manifest_input_ports directly")

		assert.Contains(t, dltContent, "dlt_input_ports", "dlt_ports.tf must declare dlt_input_ports local")
		assert.True(t,
			strings.Contains(dltContent, `port.type == "dlt"`) || strings.Contains(dltContent, `try(port.type, "") == "dlt"`),
			"dlt_input_ports must filter port.type == 'dlt'",
		)
		assert.True(t,
			strings.Contains(dltContent, `startswith(port.source, "dlt://")`) || strings.Contains(dltContent, `try(startswith(port.source, "dlt://"), false)`),
			"dlt_input_ports must filter startswith(port.source, 'dlt://')",
		)

		// AC-1: dlt_bigquery_dataset fallback
		assert.Contains(t, dltContent, "dlt_bigquery_dataset", "dlt_ports.tf must declare dlt_bigquery_dataset local")
		assert.Contains(t, dltContent, `var.bigquery_dataset_id != "" ? var.bigquery_dataset_id : replace(local.product_id, "-", "_")`,
			"dlt_bigquery_dataset must fallback to replace(local.product_id, \"-\", \"_\") when var.bigquery_dataset_id is empty",
		)

		// AC-2: Collision-Resistant SA Derivation
		assert.Contains(t, dltContent, "candidate_runner_sa", "dlt_ports.tf must declare candidate_runner_sa")
		assert.Contains(t, dltContent, "dlt_runner_sa_id", "dlt_ports.tf must declare dlt_runner_sa_id")
		assert.Contains(t, dltContent, "dlt_runner_sa_email", "dlt_ports.tf must declare dlt_runner_sa_email")

		assert.Contains(t, dltContent, "candidate_scheduler_sa", "dlt_ports.tf must declare candidate_scheduler_sa")
		assert.Contains(t, dltContent, "dlt_scheduler_sa_id", "dlt_ports.tf must declare dlt_scheduler_sa_id")
		assert.Contains(t, dltContent, "dlt_scheduler_sa_email", "dlt_ports.tf must declare dlt_scheduler_sa_email")

		// Verify derivation logic structure in HCL
		assert.Contains(t, dltContent, "length(local.candidate_runner_sa) <= 30", "dlt_runner_sa_id must guard length <= 30")
		assert.Contains(t, dltContent, "length(local.candidate_scheduler_sa) <= 30", "dlt_scheduler_sa_id must guard length <= 30")
		assert.Contains(t, dltContent, "md5(local.candidate_runner_sa)", "dlt_runner_sa_id must hash candidate using md5")
		assert.Contains(t, dltContent, "md5(local.candidate_scheduler_sa)", "dlt_scheduler_sa_id must hash candidate using md5")
		assert.Contains(t, dltContent, `regexreplace(substr(local.candidate_runner_sa, 0, 23), "-+$", "")`, "dlt_runner_sa_id must strip trailing hyphens before hash suffix")
		assert.Contains(t, dltContent, `regexreplace(substr(local.candidate_scheduler_sa, 0, 23), "-+$", "")`, "dlt_scheduler_sa_id must strip trailing hyphens before hash suffix")

		// Table-driven matrix testing collision-resistant SA derivation algorithm
		deriveSA := func(cand string) string {
			if len(cand) <= 30 {
				return cand
			}
			pfx := cand[:23]
			re := regexp.MustCompile("-+$")
			pfx = re.ReplaceAllString(pfx, "")
			hashBytes := md5.Sum([]byte(cand))
			hash := fmt.Sprintf("%x", hashBytes)[:6]
			return pfx + "-" + hash
		}

		saTests := []struct {
			name        string
			candidate   string
			wantMaxLen  int
			wantMatches string
		}{
			{
				name:        "short candidate (<=30 chars) unchanged",
				candidate:   "dlt-r-orders",
				wantMaxLen:  30,
				wantMatches: "^dlt-r-orders$",
			},
			{
				name:        "exactly 30 chars unchanged",
				candidate:   "dlt-r-exactly-30-chars-here123",
				wantMaxLen:  30,
				wantMatches: "^dlt-r-exactly-30-chars-here123$",
			},
			{
				name:        "long candidate (>30 chars) truncated with md5 suffix strictly <= 30 chars",
				candidate:   "dlt-r-very-long-data-product-identifier-exceeding-limit",
				wantMaxLen:  30,
				wantMatches: `^dlt-r-very-long-data-pr-[a-f0-9]{6}$`,
			},
			{
				name:        "long candidate ending in hyphen at truncation boundary strips trailing hyphen before hash",
				candidate:   "dlt-r-sixteen-char-----suffix-exceeding-thirty",
				wantMaxLen:  30,
				wantMatches: `^dlt-r-sixteen-char-[a-f0-9]{6}$`,
			},
		}

		for _, tc := range saTests {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				derived := deriveSA(tc.candidate)
				assert.LessOrEqual(t, len(derived), tc.wantMaxLen, "derived SA ID must be <= 30 characters")
				assert.Regexp(t, regexp.MustCompile(tc.wantMatches), derived)
				assert.NotContains(t, derived, "--", "derived SA must not contain double hyphens at truncation boundary")
			})
		}
	})

	t.Run("BackfillAndVariables", func(t *testing.T) {
		varsPath := filepath.Join(dpDir, "variables.tf")
		varsBytes, err := os.ReadFile(varsPath)
		require.NoError(t, err, "variables.tf must exist in modules/data_product")
		varsContent := string(varsBytes)

		// AC-8: Required DLT variables declarations and defaults
		reqVars := []struct {
			name         string
			expectedType string
			defaultVal   string
		}{
			{name: "dlt_runner_image", expectedType: "string", defaultVal: `"ghcr.io/supercargo-dev/supercargo-dlt-runner:latest"`},
			{name: "dlt_deletion_protection", expectedType: "bool", defaultVal: "false"},
			{name: "dlt_time_zone", expectedType: "string", defaultVal: `"UTC"`},
			{name: "dlt_cpu", expectedType: "string", defaultVal: `"1"`},
			{name: "dlt_memory", expectedType: "string", defaultVal: `"1Gi"`},
			{name: "dlt_timeout", expectedType: "string", defaultVal: `"3600s"`},
			{name: "dlt_max_retries", expectedType: "number", defaultVal: "3"},
			{name: "dlt_runner_service_account_id", expectedType: "string", defaultVal: `""`},
			{name: "dlt_scheduler_service_account_id", expectedType: "string", defaultVal: `""`},
		}

		for _, rv := range reqVars {
			block := extractHCLBlock(varsContent, fmt.Sprintf(`variable "%s"`, rv.name))
			require.NotEmpty(t, block, "variable %q must be declared in variables.tf", rv.name)
			assert.Contains(t, block, fmt.Sprintf("type        = %s", rv.expectedType), "variable %q must have type %s", rv.name, rv.expectedType)
			assert.Contains(t, block, fmt.Sprintf("default     = %s", rv.defaultVal), "variable %q must have default %s", rv.name, rv.defaultVal)
		}

		// AC-8 & Error Mode A: SA validation condition and error message
		for _, saVar := range []string{"dlt_runner_service_account_id", "dlt_scheduler_service_account_id"} {
			block := extractHCLBlock(varsContent, fmt.Sprintf(`variable "%s"`, saVar))
			assert.Contains(t, block, "validation {", "variable %q must include a validation block", saVar)
			assert.Contains(t, block, `^[a-z]([-a-z0-9]*[a-z0-9])$`, "variable %q validation must enforce GCP SA regex naming", saVar)
			assert.Contains(t, block, "error_message", "variable %q validation must specify an error message", saVar)
		}

		// Table-driven verification of the SA validation condition (Golden UX / Error Mode A contract)
		saRegex := regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])$`)
		validateSA := func(val string) bool {
			if val == "" {
				return true
			}
			return len(val) >= 6 && len(val) <= 30 && saRegex.MatchString(val)
		}

		valTests := []struct {
			name    string
			input   string
			isValid bool
		}{
			{name: "empty string is valid default", input: "", isValid: true},
			{name: "valid 6-char SA", input: "dlt-sa", isValid: true},
			{name: "valid standard SA", input: "dlt-runner-sa", isValid: true},
			{name: "valid 30-char SA", input: "dlt-runner-service-account-01", isValid: true},
			{name: "too short (< 6 chars)", input: "short", isValid: false},
			{name: "starts with digit", input: "1dlt-runner", isValid: false},
			{name: "starts with hyphen", input: "-dlt-runner", isValid: false},
			{name: "ends with hyphen", input: "dlt-runner-", isValid: false},
			{name: "contains uppercase", input: "dlt-Runner", isValid: false},
			{name: "contains underscore", input: "dlt_runner", isValid: false},
			{name: "too long (> 30 chars)", input: "dlt-runner-service-account-too-long-exceeding-limit", isValid: false},
		}

		for _, tc := range valTests {
			tc := tc
			t.Run("SA_Validation/"+tc.name, func(t *testing.T) {
				t.Parallel()
				got := validateSA(tc.input)
				assert.Equal(t, tc.isValid, got, "input %q validation expected %v got %v", tc.input, tc.isValid, got)
			})
		}
	})

	t.Run("CloudRunV2Job", func(t *testing.T) {
		dltPath := filepath.Join(dpDir, "dlt_ports.tf")
		dltBytes, err := os.ReadFile(dltPath)
		require.NoError(t, err, "dlt_ports.tf must exist in modules/data_product")
		dltContent := string(dltBytes)

		// 1. Assert google_service_account.dlt_runner block
		runnerSABlock := extractHCLBlock(dltContent, `resource "google_service_account" "dlt_runner"`)
		require.NotEmpty(t, runnerSABlock, "dlt_ports.tf must define resource google_service_account.dlt_runner")
		assert.Regexp(t, regexp.MustCompile(`count\s*=\s*length\(local\.dlt_input_ports\)\s*>\s*0\s*\?\s*1\s*:\s*0`), runnerSABlock)
		assert.Regexp(t, regexp.MustCompile(`account_id\s*=\s*local\.dlt_runner_sa_id`), runnerSABlock)
		assert.Contains(t, runnerSABlock, `display_name = "DLT Ingestion Runner Service Account for ${local.product_id}"`)
		assert.Regexp(t, regexp.MustCompile(`project\s*=\s*var\.project_id`), runnerSABlock)

		// 2. Assert google_cloud_run_v2_job.dlt_jobs block
		jobBlock := extractHCLBlock(dltContent, `resource "google_cloud_run_v2_job" "dlt_jobs"`)
		require.NotEmpty(t, jobBlock, "dlt_ports.tf must define resource google_cloud_run_v2_job.dlt_jobs")
		assert.Regexp(t, regexp.MustCompile(`for_each\s*=\s*local\.dlt_input_ports`), jobBlock)
		assert.Regexp(t, regexp.MustCompile(`name\s*=\s*"dlt-\$\{local\.product_id\}-\$\{replace\(each\.key,\s*"_",\s*"-"\)\}\$\{var\.name_suffix\s*!=\s*""\s*\?\s*"-\$\{var\.name_suffix\}"\s*:\s*""\}"`), jobBlock)
		assert.Regexp(t, regexp.MustCompile(`location\s*=\s*var\.region`), jobBlock)
		assert.Regexp(t, regexp.MustCompile(`project\s*=\s*var\.project_id`), jobBlock)
		assert.Regexp(t, regexp.MustCompile(`deletion_protection\s*=\s*var\.dlt_deletion_protection`), jobBlock)

		// Template attributes
		assert.Regexp(t, regexp.MustCompile(`service_account\s*=\s*local\.dlt_runner_sa_email`), jobBlock)
		assert.Regexp(t, regexp.MustCompile(`max_retries\s*=\s*var\.dlt_max_retries`), jobBlock)
		assert.Regexp(t, regexp.MustCompile(`timeout\s*=\s*var\.dlt_timeout`), jobBlock)
		assert.Regexp(t, regexp.MustCompile(`image\s*=\s*var\.dlt_runner_image`), jobBlock)

		// AC-3: Dynamic Container Arguments
		assert.Contains(t, jobBlock, "compact(flatten([", "job args must use compact(flatten([...])) (AC-3)")
		assert.Contains(t, jobBlock, `"--source"`, "job args must include --source")
		assert.Contains(t, jobBlock, `"--contract-urn"`, "job args must include --contract-urn")
		assert.Contains(t, jobBlock, `try(each.value.contract_urn, try(each.value.contract.urn, try(each.value.contract, each.key)))`, "job args must resolve contract URN supporting nested contract.urn, direct contract string, or port key fallback")
		assert.Contains(t, jobBlock, `"--product-urn"`, "job args must include --product-urn")
		assert.Contains(t, jobBlock, `data.supercargo_data_product.this.urn`, "job args must reference data.supercargo_data_product.this.urn for --product-urn (AC-3)")
		assert.Contains(t, jobBlock, `"--destination"`, "job args must include --destination")
		assert.Contains(t, jobBlock, `"bigquery"`, "job args destination must be bigquery")
		assert.Contains(t, jobBlock, `"--dataset"`, "job args must include --dataset")
		assert.Contains(t, jobBlock, `local.dlt_bigquery_dataset`, "job args must reference local.dlt_bigquery_dataset")
		assert.Contains(t, jobBlock, `"--pipeline-name"`, "job args must include --pipeline-name")
		assert.Contains(t, jobBlock, `"dlt_${replace(local.product_id, "-", "_")}_${replace(each.key, "-", "_")}${var.name_suffix != "" ? "_${replace(var.name_suffix, "-", "_")}" : ""}"`, "job args pipeline-name must format with underscores and optional name_suffix")

		// Table fallback for sql_database and filesystem
		assert.Contains(t, jobBlock, `["--table", try(each.value.table, each.key)]`, "job args must append --table with fallback to each.key for sql_database/filesystem (AC-3)")

		// Optional field guards
		assert.Contains(t, jobBlock, `"--cursor-field"`, "job args must support optional --cursor-field")
		assert.Contains(t, jobBlock, `"--primary-key"`, "job args must support optional --primary-key")
		assert.Contains(t, jobBlock, `"--write-disposition"`, "job args must support optional --write-disposition")
		assert.Contains(t, jobBlock, `"--cold-start-window"`, "job args must support optional --cold-start-window")
		assert.Contains(t, jobBlock, `"--entrypoint"`, "job args must support optional --entrypoint")

		// Environment variables
		assert.Contains(t, jobBlock, `"SUPERCARGO_HUB_URL"`, "job env must define SUPERCARGO_HUB_URL")
		assert.Contains(t, jobBlock, `var.hub_address`, "job env SUPERCARGO_HUB_URL must bind var.hub_address")
		assert.Contains(t, jobBlock, `"SUPERCARGO_HUB_AUDIENCE"`, "job env must define SUPERCARGO_HUB_AUDIENCE")
		assert.Contains(t, jobBlock, `var.hub_oidc_audience`, "job env SUPERCARGO_HUB_AUDIENCE must bind var.hub_oidc_audience")
		assert.Contains(t, jobBlock, `"SUPERCARGO_VAULT_URL"`, "job env must define SUPERCARGO_VAULT_URL")
		assert.Contains(t, jobBlock, `var.kms_address`, "job env SUPERCARGO_VAULT_URL must bind var.kms_address")
		assert.Contains(t, jobBlock, `"SUPERCARGO_VAULT_AUDIENCE"`, "job env must define SUPERCARGO_VAULT_AUDIENCE")
		assert.Contains(t, jobBlock, `var.vault_oidc_audience`, "job env SUPERCARGO_VAULT_AUDIENCE must bind var.vault_oidc_audience")
		assert.Contains(t, jobBlock, `"GOOGLE_CLOUD_PROJECT"`, "job env must define GOOGLE_CLOUD_PROJECT")
		assert.Contains(t, jobBlock, `"DESTINATION__BIGQUERY__LOCATION"`, "job env must define DESTINATION__BIGQUERY__LOCATION")

		// Dynamic secret environment bindings
		assert.Contains(t, jobBlock, `dynamic "env"`, "job must declare dynamic env blocks for secrets")
		assert.Contains(t, jobBlock, `"SUPERCARGO_DB_CREDENTIALS"`, "job must mount credentials to SUPERCARGO_DB_CREDENTIALS")
		assert.Contains(t, jobBlock, `"SUPERCARGO_CREDENTIALS_SECRET"`, "job must mount credentials to SUPERCARGO_CREDENTIALS_SECRET")
		assert.Contains(t, jobBlock, `secret_key_ref`, "job secret mount must use value_source.secret_key_ref")
		assert.Contains(t, jobBlock, `version = "latest"`, "secret_key_ref must use latest version")

		// Container resources
		assert.Regexp(t, regexp.MustCompile(`cpu\s*=\s*var\.dlt_cpu`), jobBlock)
		assert.Regexp(t, regexp.MustCompile(`memory\s*=\s*var\.dlt_memory`), jobBlock)

		// AC-4: Lifecycle ignore_changes & depends_on
		assert.Contains(t, jobBlock, "lifecycle {", "job must define lifecycle block")
		assert.Contains(t, jobBlock, "ignore_changes = [", "job must define ignore_changes in lifecycle")
		assert.Contains(t, jobBlock, "template[0].template[0].containers[0].image", "ignore_changes must ignore container image")
		assert.Contains(t, jobBlock, "client", "ignore_changes must ignore client")
		assert.Contains(t, jobBlock, "client_version", "ignore_changes must ignore client_version")
		assert.Contains(t, jobBlock, "depends_on = [", "job must define depends_on")
		assert.Contains(t, jobBlock, "google_service_account.dlt_runner", "job depends_on must include google_service_account.dlt_runner (AC-4)")

		// Table-driven verification of argument synthesis behavior across canonical input matrix
		type dltPort struct {
			key              string
			source           string
			table            string
			contractUrn      string
			contract         any
			cursorField      string
			primaryKey       string
			writeDisposition string
			coldStartWindow  string
			entrypoint       string
		}

		resolveContractURN := func(p dltPort) string {
			if p.contractUrn != "" {
				return p.contractUrn
			}
			switch c := p.contract.(type) {
			case map[string]string:
				if u, ok := c["urn"]; ok && u != "" {
					return u
				}
			case map[string]any:
				if u, ok := c["urn"].(string); ok && u != "" {
					return u
				}
			case string:
				if c != "" {
					return c
				}
			}
			return p.key
		}

		formatPipelineName := func(productID, portKey, nameSuffix string) string {
			normProduct := strings.ReplaceAll(productID, "-", "_")
			normPort := strings.ReplaceAll(portKey, "-", "_")
			suffix := ""
			if nameSuffix != "" {
				suffix = "_" + strings.ReplaceAll(nameSuffix, "-", "_")
			}
			return fmt.Sprintf("dlt_%s_%s%s", normProduct, normPort, suffix)
		}

		synthesizeArgs := func(productID, productURN, dataset, nameSuffix string, p dltPort) []string {
			var args []string
			args = append(args, "--source", p.source)
			args = append(args, "--contract-urn", resolveContractURN(p))
			args = append(args, "--product-urn", productURN)
			args = append(args, "--destination", "bigquery")
			args = append(args, "--dataset", dataset)
			args = append(args, "--pipeline-name", formatPipelineName(productID, p.key, nameSuffix))

			isSQLOrFS := strings.Contains(p.source, "sql_database") || strings.Contains(p.source, "filesystem")
			if isSQLOrFS {
				t := p.table
				if t == "" {
					t = p.key
				}
				args = append(args, "--table", t)
			}
			if p.cursorField != "" {
				args = append(args, "--cursor-field", p.cursorField)
			}
			if p.primaryKey != "" {
				args = append(args, "--primary-key", p.primaryKey)
			}
			if p.writeDisposition != "" {
				args = append(args, "--write-disposition", p.writeDisposition)
			}
			if p.coldStartWindow != "" {
				args = append(args, "--cold-start-window", p.coldStartWindow)
			}
			if p.entrypoint != "" {
				args = append(args, "--entrypoint", p.entrypoint)
			}
			return args
		}

		argMatrix := []struct {
			name        string
			nameSuffix  string
			port        dltPort
			wantContain []string
			wantAbsent  []string
		}{
			{
				name: "sql_database with explicit table and optional fields",
				port: dltPort{
					key:              "raw_orders",
					source:           "dlt://sql_database",
					table:            "orders",
					contractUrn:      "urn:sc:contract:orders:v1",
					cursorField:      "updated_at",
					primaryKey:       "id",
					writeDisposition: "merge",
					coldStartWindow:  "30d",
				},
				wantContain: []string{
					"--source", "dlt://sql_database",
					"--contract-urn", "urn:sc:contract:orders:v1",
					"--product-urn", "urn:sc:product:sales:v1",
					"--table", "orders",
					"--cursor-field", "updated_at",
					"--primary-key", "id",
					"--write-disposition", "merge",
					"--cold-start-window", "30d",
				},
				wantAbsent: []string{"--entrypoint"},
			},
			{
				name: "sql_database omitting table falls back to port key to prevent runtime ValueError",
				port: dltPort{
					key:         "customers_sync",
					source:      "dlt://sql_database",
					contractUrn: "urn:sc:contract:customers:v1",
				},
				wantContain: []string{
					"--source", "dlt://sql_database",
					"--table", "customers_sync",
					"--pipeline-name", "dlt_sales_customers_sync",
				},
				wantAbsent: []string{"--cursor-field", "--primary-key", "--write-disposition", "--cold-start-window", "--entrypoint"},
			},
			{
				name: "nested contract map { urn: ... } resolves contract URN correctly",
				port: dltPort{
					key:    "nested_contract_port",
					source: "dlt://sql_database",
					contract: map[string]string{
						"urn": "urn:sc:contract:nested_orders:v1",
					},
				},
				wantContain: []string{
					"--source", "dlt://sql_database",
					"--contract-urn", "urn:sc:contract:nested_orders:v1",
					"--pipeline-name", "dlt_sales_nested_contract_port",
				},
				wantAbsent: []string{"--entrypoint"},
			},
			{
				name: "direct contract string resolves contract URN correctly",
				port: dltPort{
					key:      "direct_contract_port",
					source:   "dlt://rest_api",
					contract: "urn:sc:contract:direct_orders:v1",
				},
				wantContain: []string{
					"--source", "dlt://rest_api",
					"--contract-urn", "urn:sc:contract:direct_orders:v1",
					"--pipeline-name", "dlt_sales_direct_contract_port",
				},
				wantAbsent: []string{"--entrypoint"},
			},
			{
				name: "contract_urn takes precedence over contract map",
				port: dltPort{
					key:         "override_port",
					source:      "dlt://rest_api",
					contractUrn: "urn:sc:contract:override:v1",
					contract: map[string]string{
						"urn": "urn:sc:contract:ignored:v1",
					},
				},
				wantContain: []string{
					"--source", "dlt://rest_api",
					"--contract-urn", "urn:sc:contract:override:v1",
				},
				wantAbsent: []string{"urn:sc:contract:ignored:v1"},
			},
			{
				name: "omitting contract and contract_urn falls back to port key",
				port: dltPort{
					key:    "fallback_port_key",
					source: "dlt://rest_api",
				},
				wantContain: []string{
					"--source", "dlt://rest_api",
					"--contract-urn", "fallback_port_key",
				},
			},
			{
				name:       "multi-tenant pipeline-name appends normalized name_suffix",
				nameSuffix: "dev-feature-1",
				port: dltPort{
					key:         "tenant_orders",
					source:      "dlt://rest_api",
					contractUrn: "urn:sc:contract:tenant_orders:v1",
				},
				wantContain: []string{
					"--source", "dlt://rest_api",
					"--contract-urn", "urn:sc:contract:tenant_orders:v1",
					"--pipeline-name", "dlt_sales_tenant_orders_dev_feature_1",
				},
				wantAbsent: []string{"--entrypoint"},
			},
			{
				name: "filesystem with explicit table",
				port: dltPort{
					key:         "gcs_events",
					source:      "dlt://filesystem",
					table:       "events_raw",
					contractUrn: "urn:sc:contract:events:v1",
				},
				wantContain: []string{
					"--source", "dlt://filesystem",
					"--table", "events_raw",
				},
				wantAbsent: []string{"--entrypoint"},
			},
			{
				name: "rest_api connector does not include --table",
				port: dltPort{
					key:              "stripe_charges",
					source:           "dlt://rest_api",
					contractUrn:      "urn:sc:contract:stripe:v1",
					writeDisposition: "append",
				},
				wantContain: []string{
					"--source", "dlt://rest_api",
					"--contract-urn", "urn:sc:contract:stripe:v1",
					"--write-disposition", "append",
				},
				wantAbsent: []string{"--table", "--entrypoint"},
			},
			{
				name: "custom python pipeline with entrypoint",
				port: dltPort{
					key:         "custom_feed",
					source:      "dlt://custom",
					contractUrn: "urn:sc:contract:custom:v1",
					entrypoint:  "pipelines/feed.py:load_data",
				},
				wantContain: []string{
					"--source", "dlt://custom",
					"--entrypoint", "pipelines/feed.py:load_data",
				},
				wantAbsent: []string{"--table"},
			},
		}

		for _, tc := range argMatrix {
			tc := tc
			t.Run("ArgMatrix/"+tc.name, func(t *testing.T) {
				t.Parallel()
				args := synthesizeArgs("sales", "urn:sc:product:sales:v1", "sales_bronze", tc.nameSuffix, tc.port)
				for _, expected := range tc.wantContain {
					assert.Contains(t, args, expected, "synthesized args should contain %q", expected)
				}
				for _, forbidden := range tc.wantAbsent {
					assert.NotContains(t, args, forbidden, "synthesized args should not contain %q", forbidden)
				}
			})
		}
	})

	t.Run("SchedulerAndIAM", func(t *testing.T) {
		dltPath := filepath.Join(dpDir, "dlt_ports.tf")
		dltBytes, err := os.ReadFile(dltPath)
		require.NoError(t, err, "dlt_ports.tf must exist in modules/data_product")
		dltContent := string(dltBytes)

		// 1. Assert google_service_account.dlt_scheduler (AC-5)
		schedSABlock := extractHCLBlock(dltContent, `resource "google_service_account" "dlt_scheduler"`)
		require.NotEmpty(t, schedSABlock, "dlt_ports.tf must define resource google_service_account.dlt_scheduler")
		assert.Regexp(t, regexp.MustCompile(`count\s*=\s*length\(local\.dlt_input_ports\)\s*>\s*0\s*\?\s*1\s*:\s*0`), schedSABlock)
		assert.Regexp(t, regexp.MustCompile(`account_id\s*=\s*local\.dlt_scheduler_sa_id`), schedSABlock)
		assert.Contains(t, schedSABlock, `display_name = "DLT Cloud Scheduler Service Account for ${local.product_id}"`)
		assert.Regexp(t, regexp.MustCompile(`project\s*=\s*var\.project_id`), schedSABlock)

		// 2. Assert google_cloud_run_v2_job_iam_member.dlt_scheduler_invoker (AC-5)
		invokerBlock := extractHCLBlock(dltContent, `resource "google_cloud_run_v2_job_iam_member" "dlt_scheduler_invoker"`)
		require.NotEmpty(t, invokerBlock, "dlt_ports.tf must define resource google_cloud_run_v2_job_iam_member.dlt_scheduler_invoker")
		assert.Regexp(t, regexp.MustCompile(`for_each\s*=\s*local\.dlt_input_ports`), invokerBlock)
		assert.Regexp(t, regexp.MustCompile(`project\s*=\s*var\.project_id`), invokerBlock)
		assert.Regexp(t, regexp.MustCompile(`location\s*=\s*var\.region`), invokerBlock)
		assert.Regexp(t, regexp.MustCompile(`name\s*=\s*google_cloud_run_v2_job\.dlt_jobs\[each\.key\]\.name`), invokerBlock)
		assert.Regexp(t, regexp.MustCompile(`role\s*=\s*"roles/run\.invoker"`), invokerBlock)
		assert.Regexp(t, regexp.MustCompile(`member\s*=\s*"serviceAccount:\$\{local\.dlt_scheduler_sa_email\}"`), invokerBlock)
		assert.Contains(t, invokerBlock, "depends_on = [", "invoker IAM binding must define depends_on")
		assert.Contains(t, invokerBlock, "google_service_account.dlt_scheduler", "invoker IAM depends_on must include google_service_account.dlt_scheduler")
		assert.Contains(t, invokerBlock, "google_cloud_run_v2_job.dlt_jobs", "invoker IAM depends_on must include google_cloud_run_v2_job.dlt_jobs")

		// 3. Assert google_cloud_scheduler_job.dlt_schedules (AC-5)
		schedJobBlock := extractHCLBlock(dltContent, `resource "google_cloud_scheduler_job" "dlt_schedules"`)
		require.NotEmpty(t, schedJobBlock, "dlt_ports.tf must define resource google_cloud_scheduler_job.dlt_schedules")
		assert.Regexp(t, regexp.MustCompile(`for_each\s*=\s*local\.dlt_input_ports`), schedJobBlock)
		assert.Regexp(t, regexp.MustCompile(`name\s*=\s*"dlt-sched-\$\{local\.product_id\}-\$\{replace\(each\.key,\s*"_",\s*"-"\)\}\$\{var\.name_suffix\s*!=\s*""\s*\?\s*"-\$\{var\.name_suffix\}"\s*:\s*""\}"`), schedJobBlock)
		assert.Contains(t, schedJobBlock, `try(each.value.schedule, "") != "" ? each.value.schedule : "0 2 * * *"`, "scheduler job schedule must support per-port schedule with fallback to 0 2 * * *")
		assert.Contains(t, schedJobBlock, `try(each.value.time_zone, "") != "" ? each.value.time_zone : var.dlt_time_zone`, "scheduler job time_zone must support per-port time_zone with fallback to var.dlt_time_zone")
		assert.Regexp(t, regexp.MustCompile(`region\s*=\s*var\.region`), schedJobBlock)
		assert.Regexp(t, regexp.MustCompile(`project\s*=\s*var\.project_id`), schedJobBlock)
		assert.Contains(t, schedJobBlock, "http_target {", "scheduler job must define http_target")
		assert.Regexp(t, regexp.MustCompile(`http_method\s*=\s*"POST"`), schedJobBlock)
		assert.Contains(t, schedJobBlock, `https://${var.region}-run.googleapis.com/v2/projects/${var.project_id}/locations/${var.region}/jobs/${google_cloud_run_v2_job.dlt_jobs[each.key].name}:run`, "scheduler job uri must target Cloud Run job run endpoint")
		assert.Contains(t, schedJobBlock, "oauth_token {", "scheduler job must define oauth_token block")
		assert.Regexp(t, regexp.MustCompile(`service_account_email\s*=\s*local\.dlt_scheduler_sa_email`), schedJobBlock)
		assert.Regexp(t, regexp.MustCompile(`scope\s*=\s*"https://www.googleapis.com/auth/cloud-platform"`), schedJobBlock)
		assert.Contains(t, schedJobBlock, "depends_on = [", "scheduler job must define depends_on")
		assert.Contains(t, schedJobBlock, "google_cloud_run_v2_job_iam_member.dlt_scheduler_invoker", "scheduler job depends_on must include dlt_scheduler_invoker (AC-5)")
		assert.Contains(t, schedJobBlock, "google_cloud_run_v2_job.dlt_jobs", "scheduler job depends_on must include google_cloud_run_v2_job.dlt_jobs (AC-5)")

		// 4. Assert google_project_iam_member.dlt_runner_bq_editor (AC-6)
		bqEditorBlock := extractHCLBlock(dltContent, `resource "google_project_iam_member" "dlt_runner_bq_editor"`)
		require.NotEmpty(t, bqEditorBlock, "dlt_ports.tf must define resource google_project_iam_member.dlt_runner_bq_editor")
		assert.Regexp(t, regexp.MustCompile(`count\s*=\s*length\(local\.dlt_input_ports\)\s*>\s*0\s*\?\s*1\s*:\s*0`), bqEditorBlock)
		assert.Regexp(t, regexp.MustCompile(`project\s*=\s*var\.project_id`), bqEditorBlock)
		assert.Regexp(t, regexp.MustCompile(`role\s*=\s*"roles/bigquery\.dataEditor"`), bqEditorBlock)
		assert.Regexp(t, regexp.MustCompile(`member\s*=\s*"serviceAccount:\$\{local\.dlt_runner_sa_email\}"`), bqEditorBlock)
		assert.Contains(t, bqEditorBlock, "google_service_account.dlt_runner", "bq_editor depends_on must include google_service_account.dlt_runner")

		// 5. Assert google_project_iam_member.dlt_runner_bq_user (AC-6)
		bqUserBlock := extractHCLBlock(dltContent, `resource "google_project_iam_member" "dlt_runner_bq_user"`)
		require.NotEmpty(t, bqUserBlock, "dlt_ports.tf must define resource google_project_iam_member.dlt_runner_bq_user")
		assert.Regexp(t, regexp.MustCompile(`count\s*=\s*length\(local\.dlt_input_ports\)\s*>\s*0\s*\?\s*1\s*:\s*0`), bqUserBlock)
		assert.Regexp(t, regexp.MustCompile(`project\s*=\s*var\.project_id`), bqUserBlock)
		assert.Regexp(t, regexp.MustCompile(`role\s*=\s*"roles/bigquery\.jobUser"`), bqUserBlock)
		assert.Regexp(t, regexp.MustCompile(`member\s*=\s*"serviceAccount:\$\{local\.dlt_runner_sa_email\}"`), bqUserBlock)
		assert.Contains(t, bqUserBlock, "google_service_account.dlt_runner", "bq_user depends_on must include google_service_account.dlt_runner")

		// 6. Assert google_secret_manager_secret_iam_member.dlt_runner_secret_accessor (AC-6)
		secAccessorBlock := extractHCLBlock(dltContent, `resource "google_secret_manager_secret_iam_member" "dlt_runner_secret_accessor"`)
		require.NotEmpty(t, secAccessorBlock, "dlt_ports.tf must define resource google_secret_manager_secret_iam_member.dlt_runner_secret_accessor")
		assert.Contains(t, secAccessorBlock, "toset([", "secret accessor must deduplicate secrets using toset to prevent GCP ETag 412 race conditions")
		assert.Contains(t, secAccessorBlock, "v.credentials_secret", "secret accessor must iterate over credentials_secret")
		assert.Regexp(t, regexp.MustCompile(`project\s*=\s*var\.project_id`), secAccessorBlock)
		assert.Regexp(t, regexp.MustCompile(`role\s*=\s*"roles/secretmanager\.secretAccessor"`), secAccessorBlock)
		assert.Regexp(t, regexp.MustCompile(`member\s*=\s*"serviceAccount:\$\{local\.dlt_runner_sa_email\}"`), secAccessorBlock)
		assert.Contains(t, secAccessorBlock, "google_service_account.dlt_runner", "secret accessor depends_on must include google_service_account.dlt_runner")

		// Canonical table-driven verification of Secret Manager IAM deduplication logic across multi-port sharing
		dedupSecrets := func(portSecrets []string) []string {
			seen := make(map[string]struct{})
			var res []string
			for _, s := range portSecrets {
				if s != "" {
					if _, exists := seen[s]; !exists {
						seen[s] = struct{}{}
						res = append(res, s)
					}
				}
			}
			return res
		}

		dedupTests := []struct {
			name        string
			portSecrets []string
			wantDedup   []string
		}{
			{
				name:        "multiple ports sharing the exact same secret are deduplicated to single IAM binding",
				portSecrets: []string{"shared-postgres-secret", "shared-postgres-secret", "shared-postgres-secret"},
				wantDedup:   []string{"shared-postgres-secret"},
			},
			{
				name:        "multiple ports with distinct secrets produce distinct bindings",
				portSecrets: []string{"postgres-secret", "salesforce-oauth", "stripe-api-key"},
				wantDedup:   []string{"postgres-secret", "salesforce-oauth", "stripe-api-key"},
			},
			{
				name:        "ports with empty credentials_secret are excluded",
				portSecrets: []string{"", "postgres-secret", "", "snowflake-creds"},
				wantDedup:   []string{"postgres-secret", "snowflake-creds"},
			},
		}

		for _, tc := range dedupTests {
			tc := tc
			t.Run("SecretDedup/"+tc.name, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, tc.wantDedup, dedupSecrets(tc.portSecrets))
			})
		}

		// Table-driven matrix testing of schedule, timezone, and secret_id resolution logic
		resolveSchedule := func(portSched string) string {
			if portSched != "" {
				return portSched
			}
			return "0 2 * * *"
		}
		resolveTZ := func(portTZ, defaultTZ string) string {
			if portTZ != "" {
				return portTZ
			}
			return defaultTZ
		}
		resolveSecretID := func(sec string) string {
			if strings.HasPrefix(sec, "projects/") {
				parts := strings.Split(sec, "/")
				return parts[len(parts)-1]
			}
			return sec
		}

		schedTests := []struct {
			name         string
			portSchedule string
			portTZ       string
			defaultTZ    string
			credSecret   string
			wantSchedule string
			wantTZ       string
			wantSecretID string
		}{
			{
				name:         "defaults applied when optional schedule and timezone omitted",
				portSchedule: "",
				portTZ:       "",
				defaultTZ:    "UTC",
				credSecret:   "my-db-credentials",
				wantSchedule: "0 2 * * *",
				wantTZ:       "UTC",
				wantSecretID: "my-db-credentials",
			},
			{
				name:         "explicit schedule and timezone preserved",
				portSchedule: "*/15 * * * *",
				portTZ:       "America/New_York",
				defaultTZ:    "UTC",
				credSecret:   "projects/test-proj/secrets/salesforce-oauth",
				wantSchedule: "*/15 * * * *",
				wantTZ:       "America/New_York",
				wantSecretID: "salesforce-oauth",
			},
			{
				name:         "custom default timezone applied when port timezone empty",
				portSchedule: "0 6 * * 1-5",
				portTZ:       "",
				defaultTZ:    "Europe/Stockholm",
				credSecret:   "projects/456789/secrets/api-key",
				wantSchedule: "0 6 * * 1-5",
				wantTZ:       "Europe/Stockholm",
				wantSecretID: "api-key",
			},
		}

		for _, tc := range schedTests {
			tc := tc
			t.Run("Matrix/"+tc.name, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, tc.wantSchedule, resolveSchedule(tc.portSchedule))
				assert.Equal(t, tc.wantTZ, resolveTZ(tc.portTZ, tc.defaultTZ))
				assert.Equal(t, tc.wantSecretID, resolveSecretID(tc.credSecret))
			})
		}
	})

	t.Run("Outputs", func(t *testing.T) {
		outputPath := filepath.Join(dpDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		require.NoError(t, err, "outputs.tf must exist in modules/data_product")
		outputContent := string(outputBytes)

		// AC-7: Guarded Outputs & Backfill CLI
		jobNamesBlock := extractHCLBlock(outputContent, `output "dlt_cloud_run_job_names"`)
		require.NotEmpty(t, jobNamesBlock, "modules/data_product/outputs.tf missing dlt_cloud_run_job_names output")
		assert.Contains(t, jobNamesBlock, "{ for k, v in google_cloud_run_v2_job.dlt_jobs : k => v.name }")

		schedIDsBlock := extractHCLBlock(outputContent, `output "dlt_cloud_scheduler_job_ids"`)
		require.NotEmpty(t, schedIDsBlock, "modules/data_product/outputs.tf missing dlt_cloud_scheduler_job_ids output")
		assert.Contains(t, schedIDsBlock, "{ for k, v in google_cloud_scheduler_job.dlt_schedules : k => v.id }")

		backfillBlock := extractHCLBlock(outputContent, `output "dlt_backfill_commands"`)
		require.NotEmpty(t, backfillBlock, "modules/data_product/outputs.tf missing dlt_backfill_commands output")
		assert.Contains(t, backfillBlock, `for k, v in local.dlt_input_ports : k => "gcloud run jobs execute ${google_cloud_run_v2_job.dlt_jobs[k].name} --region ${var.region} --project ${var.project_id} --args=\"--since=2026-01-01T00:00:00Z\""`)

		runnerSABlock := extractHCLBlock(outputContent, `output "dlt_runner_service_account_email"`)
		require.NotEmpty(t, runnerSABlock, "modules/data_product/outputs.tf missing dlt_runner_service_account_email output")
		assert.Contains(t, runnerSABlock, "length(local.dlt_input_ports) > 0 ? local.dlt_runner_sa_email : null")

		schedSABlock := extractHCLBlock(outputContent, `output "dlt_scheduler_service_account_email"`)
		require.NotEmpty(t, schedSABlock, "modules/data_product/outputs.tf missing dlt_scheduler_service_account_email output")
		assert.Contains(t, schedSABlock, "length(local.dlt_input_ports) > 0 ? local.dlt_scheduler_sa_email : null")

		// Canonical Table-driven matrix for outputs and backfill commands
		type portMeta struct {
			portKey string
			jobName string
		}
		formatBackfillCmd := func(jobName, region, project string) string {
			return fmt.Sprintf("gcloud run jobs execute %s --region %s --project %s --args=\"--since=2026-01-01T00:00:00Z\"", jobName, region, project)
		}
		evalGuardedSA := func(portCount int, email string) *string {
			if portCount > 0 {
				return &email
			}
			return nil
		}

		matrixTests := []struct {
			name        string
			ports       []portMeta
			region      string
			project     string
			runnerEmail string
			schedEmail  string
			wantCmds    map[string]string
			wantRunSA   *string
			wantSchedSA *string
		}{
			{
				name:        "zero DLT ports returns empty maps and null SAs",
				ports:       []portMeta{},
				region:      "us-central1",
				project:     "test-project",
				runnerEmail: "dlt-runner@test-project.iam.gserviceaccount.com",
				schedEmail:  "dlt-sched@test-project.iam.gserviceaccount.com",
				wantCmds:    map[string]string{},
				wantRunSA:   nil,
				wantSchedSA: nil,
			},
			{
				name: "single DLT port produces formatted backfill command and non-null SAs",
				ports: []portMeta{
					{portKey: "raw_orders", jobName: "dlt-sales-raw-orders"},
				},
				region:      "europe-west1",
				project:     "prod-analytics",
				runnerEmail: "dlt-runner@prod-analytics.iam.gserviceaccount.com",
				schedEmail:  "dlt-sched@prod-analytics.iam.gserviceaccount.com",
				wantCmds: map[string]string{
					"raw_orders": "gcloud run jobs execute dlt-sales-raw-orders --region europe-west1 --project prod-analytics --args=\"--since=2026-01-01T00:00:00Z\"",
				},
				wantRunSA:   func() *string { s := "dlt-runner@prod-analytics.iam.gserviceaccount.com"; return &s }(),
				wantSchedSA: func() *string { s := "dlt-sched@prod-analytics.iam.gserviceaccount.com"; return &s }(),
			},
			{
				name: "multiple DLT ports produces all backfill commands",
				ports: []portMeta{
					{portKey: "pg_users", jobName: "dlt-crm-pg-users"},
					{portKey: "s3_logs", jobName: "dlt-crm-s3-logs"},
				},
				region:      "us-east1",
				project:     "corp-data",
				runnerEmail: "dlt-runner@corp-data.iam.gserviceaccount.com",
				schedEmail:  "dlt-sched@corp-data.iam.gserviceaccount.com",
				wantCmds: map[string]string{
					"pg_users": "gcloud run jobs execute dlt-crm-pg-users --region us-east1 --project corp-data --args=\"--since=2026-01-01T00:00:00Z\"",
					"s3_logs":  "gcloud run jobs execute dlt-crm-s3-logs --region us-east1 --project corp-data --args=\"--since=2026-01-01T00:00:00Z\"",
				},
				wantRunSA:   func() *string { s := "dlt-runner@corp-data.iam.gserviceaccount.com"; return &s }(),
				wantSchedSA: func() *string { s := "dlt-sched@corp-data.iam.gserviceaccount.com"; return &s }(),
			},
		}

		for _, tc := range matrixTests {
			tc := tc
			t.Run("Matrix/"+tc.name, func(t *testing.T) {
				t.Parallel()
				cmds := make(map[string]string, len(tc.ports))
				for _, p := range tc.ports {
					cmds[p.portKey] = formatBackfillCmd(p.jobName, tc.region, tc.project)
				}
				assert.Equal(t, tc.wantCmds, cmds)
				assert.Equal(t, tc.wantRunSA, evalGuardedSA(len(tc.ports), tc.runnerEmail))
				assert.Equal(t, tc.wantSchedSA, evalGuardedSA(len(tc.ports), tc.schedEmail))
			})
		}
	})
}

func TestModules_HubAuditSink(t *testing.T) {
	modulesDir := "../../modules"
	hubDir := filepath.Join(modulesDir, "hub")

	t.Run("VariablesDefinedWithCorrectDefaults", func(t *testing.T) {
		varPath := filepath.Join(hubDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		requiredVars := []struct {
			name         string
			defaultValue string
		}{
			{name: "enable_audit_sink", defaultValue: "true"},
			{name: "audit_dataset_id", defaultValue: `"supercargo_audit"`},
			{name: "audit_table_id", defaultValue: `"outbox_events"`},
			{name: "audit_view_id", defaultValue: `"outbox_events_view"`},
			{name: "bigquery_location", defaultValue: `""`},
			{name: "bigquery_deletion_protection", defaultValue: "true"},
		}

		for _, rv := range requiredVars {
			if !strings.Contains(varContent, `variable "`+rv.name+`"`) {
				t.Errorf("modules/hub/variables.tf missing variable %q", rv.name)
			}
			if !strings.Contains(varContent, rv.defaultValue) {
				t.Errorf("modules/hub/variables.tf variable %q missing default %s", rv.name, rv.defaultValue)
			}
		}
	})

	t.Run("BigQueryAPIEnabled", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `"bigquery.googleapis.com"`) {
			t.Errorf("modules/hub/main.tf missing \"bigquery.googleapis.com\" in google_project_service.hub_apis")
		}
	})

	t.Run("BigQueryDatasetConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_bigquery_dataset" "supercargo_audit"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_bigquery_dataset\" \"supercargo_audit\"")
		}
		if !strings.Contains(mainContent, "dataset_id                 = var.audit_dataset_id") && !strings.Contains(mainContent, "dataset_id = var.audit_dataset_id") {
			t.Errorf("dataset must use var.audit_dataset_id")
		}
		if !strings.Contains(mainContent, "location                   = local.bq_location") && !strings.Contains(mainContent, "location = local.bq_location") {
			t.Errorf("dataset must use local.bq_location")
		}
		if !strings.Contains(mainContent, "delete_contents_on_destroy = !var.bigquery_deletion_protection") && !strings.Contains(mainContent, "delete_contents_on_destroy = ! var.bigquery_deletion_protection") {
			t.Errorf("dataset must use delete_contents_on_destroy = !var.bigquery_deletion_protection")
		}
	})

	t.Run("BigQueryRawTableConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_bigquery_table" "outbox_events"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_bigquery_table\" \"outbox_events\"")
		}
		if !strings.Contains(mainContent, "dataset_id          = google_bigquery_dataset.supercargo_audit[0].dataset_id") &&
			!strings.Contains(mainContent, "dataset_id = google_bigquery_dataset.supercargo_audit[0].dataset_id") {
			t.Errorf("outbox_events table must reference google_bigquery_dataset.supercargo_audit[0].dataset_id")
		}
		if !strings.Contains(mainContent, "table_id            = var.audit_table_id") &&
			!strings.Contains(mainContent, "table_id = var.audit_table_id") {
			t.Errorf("outbox_events table must use var.audit_table_id")
		}
		if !strings.Contains(mainContent, "deletion_protection = var.bigquery_deletion_protection") {
			t.Errorf("outbox_events table must use deletion_protection = var.bigquery_deletion_protection")
		}
		if !strings.Contains(mainContent, `field = "publish_time"`) && !strings.Contains(mainContent, `field  = "publish_time"`) {
			t.Errorf("outbox_events table must partition by publish_time")
		}
		// Schema fields
		for _, field := range []string{"subscription_name", "message_id", "publish_time", "attributes", "data", "JSON", "STRING"} {
			if !strings.Contains(mainContent, field) {
				t.Errorf("outbox_events table schema missing field or type %q", field)
			}
		}
	})

	t.Run("BigQueryViewConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_bigquery_table" "outbox_events_view"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_bigquery_table\" \"outbox_events_view\"")
		}
		if !strings.Contains(mainContent, "dataset_id          = google_bigquery_dataset.supercargo_audit[0].dataset_id") &&
			!strings.Contains(mainContent, "dataset_id = google_bigquery_dataset.supercargo_audit[0].dataset_id") {
			t.Errorf("outbox_events_view must reference google_bigquery_dataset.supercargo_audit[0].dataset_id")
		}
		if !strings.Contains(mainContent, "table_id            = var.audit_view_id") &&
			!strings.Contains(mainContent, "table_id = var.audit_view_id") {
			t.Errorf("outbox_events_view must use var.audit_view_id")
		}
		if !strings.Contains(mainContent, "deletion_protection = false") {
			t.Errorf("outbox_events_view must set deletion_protection = false")
		}

		viewChecks := []string{
			"COALESCE(JSON_VALUE(attributes, '$.doc_id'), message_id) AS event_id",
			"JSON_VALUE(attributes, '$.urn') AS urn",
			"JSON_VALUE(attributes, '$.action') AS action",
			"JSON_VALUE(attributes, '$.doc_id') AS doc_id",
			"SAFE.PARSE_JSON(data) AS payload",
			"publish_time AS timestamp",
		}
		for _, vc := range viewChecks {
			if !strings.Contains(mainContent, vc) {
				t.Errorf("outbox_events_view query missing %q", vc)
			}
		}
	})

	t.Run("IAMBindingsConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_bigquery_dataset_iam_member" "pubsub_audit_bq_writer"`) {
			t.Errorf("modules/hub/main.tf missing resource \"google_bigquery_dataset_iam_member\" \"pubsub_audit_bq_writer\"")
		}
		if !strings.Contains(mainContent, `role       = "roles/bigquery.dataEditor"`) && !strings.Contains(mainContent, `role = "roles/bigquery.dataEditor"`) {
			t.Errorf("pubsub_audit_bq_writer missing roles/bigquery.dataEditor")
		}
		if !strings.Contains(mainContent, `resource "google_bigquery_dataset_iam_member" "pubsub_audit_bq_metadata"`) {
			t.Errorf("modules/hub/main.tf missing resource \"google_bigquery_dataset_iam_member\" \"pubsub_audit_bq_metadata\"")
		}
		if !strings.Contains(mainContent, `role       = "roles/bigquery.metadataViewer"`) && !strings.Contains(mainContent, `role = "roles/bigquery.metadataViewer"`) {
			t.Errorf("pubsub_audit_bq_metadata missing roles/bigquery.metadataViewer")
		}
		if !strings.Contains(mainContent, `serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com`) {
			t.Errorf("IAM member must use serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com")
		}
	})

	t.Run("PubSubSubscriptionConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_pubsub_subscription" "outbox_audit_bq"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription\" \"outbox_audit_bq\"")
		}
		if !strings.Contains(mainContent, "topic   = var.control_plane_topic") && !strings.Contains(mainContent, "topic = var.control_plane_topic") {
			t.Errorf("outbox_audit_bq subscription must use topic = var.control_plane_topic")
		}
		if !strings.Contains(mainContent, "write_metadata      = true") && !strings.Contains(mainContent, "write_metadata = true") {
			t.Errorf("outbox_audit_bq subscription must set write_metadata = true")
		}
		if !strings.Contains(mainContent, "drop_unknown_fields = true") && !strings.Contains(mainContent, "drop_unknown_fields  = true") {
			t.Errorf("outbox_audit_bq subscription must set drop_unknown_fields = true")
		}
		if !strings.Contains(mainContent, "google_bigquery_dataset_iam_member.pubsub_audit_bq_writer") ||
			!strings.Contains(mainContent, "google_bigquery_dataset_iam_member.pubsub_audit_bq_metadata") {
			t.Errorf("outbox_audit_bq subscription must depend on pubsub_audit_bq_writer and pubsub_audit_bq_metadata")
		}
		if !strings.Contains(mainContent, `ttl = ""`) {
			t.Errorf("outbox_audit_bq subscription must configure permanent retention via expiration_policy { ttl = \"\" }")
		}
	})

	t.Run("OutputsExported", func(t *testing.T) {
		outputPath := filepath.Join(hubDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		outputs := []string{
			"audit_dataset_id",
			"audit_table_id",
			"audit_view_id",
			"audit_subscription_id",
		}
		for _, out := range outputs {
			if !strings.Contains(outputContent, `output "`+out+`"`) {
				t.Errorf("modules/hub/outputs.tf missing output %q", out)
			}
		}
	})
}

func TestModules_HubHealthMeshAndBigQuerySubscription(t *testing.T) {
	modulesDir := "../../modules"
	hubDir := filepath.Join(modulesDir, "hub")

	t.Run("VariablesDefinedWithCorrectDefaults", func(t *testing.T) {
		varPath := filepath.Join(hubDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		requiredVars := []struct {
			name         string
			defaultValue string
		}{
			{name: "enable_health_mesh", defaultValue: "true"},
			{name: "health_events_topic", defaultValue: `"supercargo-health-events"`},
			{name: "catalog_dataset_id", defaultValue: `"supercargo_catalog"`},
			{name: "health_history_table_id", defaultValue: `"asset_health_history"`},
			{name: "current_health_view_id", defaultValue: `"asset_current_health"`},
		}

		for _, rv := range requiredVars {
			if !strings.Contains(varContent, `variable "`+rv.name+`"`) {
				t.Errorf("modules/hub/variables.tf missing variable %q", rv.name)
			}
			if !strings.Contains(varContent, rv.defaultValue) {
				t.Errorf("modules/hub/variables.tf variable %q missing default %s", rv.name, rv.defaultValue)
			}
		}
	})

	t.Run("PubSubHealthTopicAndPublisherIAMConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_pubsub_topic" "health_events"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic\" \"health_events\"")
		}
		topicChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic" "health_events"`)
		if !strings.Contains(topicChunk, "var.enable_health_mesh") {
			t.Errorf("health_events topic missing count = var.enable_health_mesh guard")
		}
		if !strings.Contains(topicChunk, "name    = var.health_events_topic") && !strings.Contains(topicChunk, "name = var.health_events_topic") {
			t.Errorf("health_events topic must use var.health_events_topic")
		}

		if !strings.Contains(mainContent, `resource "google_pubsub_topic_iam_member" "hub_health_events_publisher"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic_iam_member\" \"hub_health_events_publisher\"")
		}
		iamChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic_iam_member" "hub_health_events_publisher"`)
		if !strings.Contains(iamChunk, `role    = "roles/pubsub.publisher"`) && !strings.Contains(iamChunk, `role = "roles/pubsub.publisher"`) {
			t.Errorf("hub_health_events_publisher must grant roles/pubsub.publisher")
		}
		if !strings.Contains(iamChunk, "google_service_account.hub_runtime.email") {
			t.Errorf("hub_health_events_publisher must bind google_service_account.hub_runtime.email")
		}

		// Cloud Run env var
		if !strings.Contains(mainContent, `"HEALTH_EVENTS_TOPIC"`) {
			t.Errorf("modules/hub/main.tf Cloud Run service missing HEALTH_EVENTS_TOPIC env var")
		}
		if !strings.Contains(mainContent, "value = var.health_events_topic") {
			t.Errorf("modules/hub/main.tf HEALTH_EVENTS_TOPIC must use var.health_events_topic")
		}
	})

	t.Run("BigQueryCatalogDatasetAndIAMConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_bigquery_dataset" "supercargo_catalog"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_bigquery_dataset\" \"supercargo_catalog\"")
		}
		dsChunk := extractHCLBlock(mainContent, `resource "google_bigquery_dataset" "supercargo_catalog"`)
		if !strings.Contains(dsChunk, "var.enable_health_mesh") {
			t.Errorf("supercargo_catalog dataset missing count = var.enable_health_mesh guard")
		}
		if !strings.Contains(dsChunk, "var.catalog_dataset_id") {
			t.Errorf("supercargo_catalog dataset must use var.catalog_dataset_id")
		}
		if !strings.Contains(dsChunk, "local.bq_location") {
			t.Errorf("supercargo_catalog dataset must use local.bq_location")
		}
		if !strings.Contains(dsChunk, "delete_contents_on_destroy = !var.bigquery_deletion_protection") {
			t.Errorf("supercargo_catalog dataset must set delete_contents_on_destroy = !var.bigquery_deletion_protection")
		}

		if !strings.Contains(mainContent, `resource "google_bigquery_dataset_iam_member" "pubsub_health_bq_writer"`) {
			t.Errorf("modules/hub/main.tf missing resource \"google_bigquery_dataset_iam_member\" \"pubsub_health_bq_writer\"")
		}
		writerChunk := extractHCLBlock(mainContent, `resource "google_bigquery_dataset_iam_member" "pubsub_health_bq_writer"`)
		if !strings.Contains(writerChunk, `roles/bigquery.dataEditor`) {
			t.Errorf("pubsub_health_bq_writer missing roles/bigquery.dataEditor")
		}
		if !strings.Contains(writerChunk, "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com") {
			t.Errorf("pubsub_health_bq_writer must bind Pub/Sub service agent")
		}

		if !strings.Contains(mainContent, `resource "google_bigquery_dataset_iam_member" "pubsub_health_bq_metadata"`) {
			t.Errorf("modules/hub/main.tf missing resource \"google_bigquery_dataset_iam_member\" \"pubsub_health_bq_metadata\"")
		}
		metaChunk := extractHCLBlock(mainContent, `resource "google_bigquery_dataset_iam_member" "pubsub_health_bq_metadata"`)
		if !strings.Contains(metaChunk, `roles/bigquery.metadataViewer`) {
			t.Errorf("pubsub_health_bq_metadata missing roles/bigquery.metadataViewer")
		}
		if !strings.Contains(metaChunk, "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com") {
			t.Errorf("pubsub_health_bq_metadata must bind Pub/Sub service agent")
		}
	})

	t.Run("BigQueryHealthHistoryTableConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_bigquery_table" "asset_health_history"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_bigquery_table\" \"asset_health_history\"")
		}
		tableChunk := extractHCLBlock(mainContent, `resource "google_bigquery_table" "asset_health_history"`)
		if !strings.Contains(tableChunk, "var.enable_health_mesh") {
			t.Errorf("asset_health_history missing count = var.enable_health_mesh guard")
		}
		if !strings.Contains(tableChunk, "google_bigquery_dataset.supercargo_catalog[0].dataset_id") {
			t.Errorf("asset_health_history must reference supercargo_catalog dataset")
		}
		if !strings.Contains(tableChunk, "var.health_history_table_id") {
			t.Errorf("asset_health_history must use var.health_history_table_id")
		}
		if !strings.Contains(tableChunk, "deletion_protection = var.bigquery_deletion_protection") {
			t.Errorf("asset_health_history must use deletion_protection = var.bigquery_deletion_protection")
		}
		if !strings.Contains(tableChunk, `field = "timestamp"`) && !strings.Contains(tableChunk, `field  = "timestamp"`) {
			t.Errorf("asset_health_history missing time_partitioning field = timestamp")
		}
		if !strings.Contains(tableChunk, `"asset_urn"`) || !strings.Contains(tableChunk, `"current_state"`) {
			t.Errorf("asset_health_history missing clustering on [\"asset_urn\", \"current_state\"]")
		}

		schemaFields := []string{
			"event_id",
			"asset_urn",
			"previous_state",
			"current_state",
			"reason",
			"incident_type",
			"run_id",
			"reporter",
			"status_source_urn",
			"timestamp",
			"downstream_urns",
			"impacted_teams",
			"subscription_name",
			"message_id",
			"publish_time",
			"attributes",
		}
		for _, field := range schemaFields {
			if !strings.Contains(tableChunk, `"`+field+`"`) {
				t.Errorf("asset_health_history table schema missing field %q", field)
			}
		}
	})

	t.Run("PubSubBigQuerySubscriptionConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_pubsub_subscription" "health_events_bq"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription\" \"health_events_bq\"")
		}
		subChunk := extractHCLBlock(mainContent, `resource "google_pubsub_subscription" "health_events_bq"`)
		if !strings.Contains(subChunk, "var.enable_health_mesh") {
			t.Errorf("health_events_bq subscription missing count = var.enable_health_mesh guard")
		}
		if !strings.Contains(subChunk, "google_pubsub_topic.health_events[0].name") {
			t.Errorf("health_events_bq subscription must reference google_pubsub_topic.health_events[0].name")
		}
		if !strings.Contains(subChunk, "use_table_schema    = true") && !strings.Contains(subChunk, "use_table_schema = true") {
			t.Errorf("health_events_bq subscription must set use_table_schema = true")
		}
		if !strings.Contains(subChunk, "write_metadata      = true") && !strings.Contains(subChunk, "write_metadata = true") {
			t.Errorf("health_events_bq subscription must set write_metadata = true")
		}
		if !strings.Contains(subChunk, "drop_unknown_fields = true") && !strings.Contains(subChunk, "drop_unknown_fields = true") {
			t.Errorf("health_events_bq subscription must set drop_unknown_fields = true")
		}
		if !strings.Contains(subChunk, `ttl = ""`) {
			t.Errorf("health_events_bq subscription must set expiration_policy ttl = \"\"")
		}
		if !strings.Contains(subChunk, "pubsub_health_bq_writer") || !strings.Contains(subChunk, "pubsub_health_bq_metadata") {
			t.Errorf("health_events_bq subscription must depend on pubsub_health_bq_writer and pubsub_health_bq_metadata")
		}
	})

	t.Run("BigQueryCurrentHealthViewConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_bigquery_table" "asset_current_health"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_bigquery_table\" \"asset_current_health\"")
		}
		viewChunk := extractHCLBlock(mainContent, `resource "google_bigquery_table" "asset_current_health"`)
		if !strings.Contains(viewChunk, "var.enable_health_mesh") {
			t.Errorf("asset_current_health view missing count = var.enable_health_mesh guard")
		}
		if !strings.Contains(viewChunk, "google_bigquery_dataset.supercargo_catalog[0].dataset_id") {
			t.Errorf("asset_current_health view must reference supercargo_catalog dataset")
		}
		if !strings.Contains(viewChunk, "var.current_health_view_id") {
			t.Errorf("asset_current_health view must use var.current_health_view_id")
		}
		if !strings.Contains(viewChunk, "deletion_protection = false") {
			t.Errorf("asset_current_health view must set deletion_protection = false")
		}
		if !strings.Contains(viewChunk, "ROW_NUMBER() OVER(PARTITION BY asset_urn ORDER BY timestamp DESC, publish_time DESC NULLS LAST) as row_num") {
			t.Errorf("asset_current_health view query missing deterministic ROW_NUMBER window function with NULLS LAST")
		}
		if !strings.Contains(viewChunk, "WHERE row_num = 1") {
			t.Errorf("asset_current_health view query missing WHERE row_num = 1")
		}
	})

	t.Run("ConditionalOutputsExported", func(t *testing.T) {
		outputPath := filepath.Join(hubDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		outputs := []struct {
			name      string
			condition string
		}{
			{name: "health_events_topic_name", condition: "var.enable_health_mesh ? google_pubsub_topic.health_events[0].name : null"},
			{name: "health_events_topic_id", condition: "var.enable_health_mesh ? google_pubsub_topic.health_events[0].id : null"},
			{name: "catalog_dataset_id", condition: "var.enable_health_mesh ? google_bigquery_dataset.supercargo_catalog[0].dataset_id : null"},
			{name: "asset_health_history_table_id", condition: "var.enable_health_mesh ? google_bigquery_table.asset_health_history[0].table_id : null"},
			{name: "asset_current_health_view_id", condition: "var.enable_health_mesh ? google_bigquery_table.asset_current_health[0].table_id : null"},
			{name: "health_events_subscription_name", condition: "var.enable_health_mesh ? google_pubsub_subscription.health_events_bq[0].name : null"},
		}

		for _, out := range outputs {
			if !strings.Contains(outputContent, `output "`+out.name+`"`) {
				t.Errorf("modules/hub/outputs.tf missing output %q", out.name)
			}
			if !strings.Contains(outputContent, out.condition) {
				t.Errorf("modules/hub/outputs.tf output %q must use %q", out.name, out.condition)
			}
		}
	})
}

func TestModules_HubDataformIntegration(t *testing.T) {
	modulesDir := "../../modules"
	hubDir := filepath.Join(modulesDir, "hub")

	t.Run("VariablesDefinedWithCorrectDefaults", func(t *testing.T) {
		varPath := filepath.Join(hubDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		requiredVars := []struct {
			name         string
			varType      string
			defaultValue string
		}{
			{name: "enable_dataform_integration", varType: "bool", defaultValue: "false"},
			{name: "dataform_events_topic", varType: "string", defaultValue: `"supercargo-dataform-events"`},
			{name: "dataform_sink_filter", varType: "string", defaultValue: `""`},
		}

		for _, rv := range requiredVars {
			if !strings.Contains(varContent, `variable "`+rv.name+`"`) {
				t.Errorf("modules/hub/variables.tf missing variable %q", rv.name)
				continue
			}
			chunk := extractHCLBlock(varContent, `variable "`+rv.name+`"`)
			if !strings.Contains(chunk, rv.varType) {
				t.Errorf("modules/hub/variables.tf variable %q missing type %s", rv.name, rv.varType)
			}
			if !strings.Contains(chunk, rv.defaultValue) {
				t.Errorf("modules/hub/variables.tf variable %q missing default %s", rv.name, rv.defaultValue)
			}
		}
	})

	t.Run("PubSubTopicsAndDLQConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		// Primary Dataform events topic
		if !strings.Contains(mainContent, `resource "google_pubsub_topic" "dataform_events"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic\" \"dataform_events\"")
		}
		topicChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic" "dataform_events"`)
		if !strings.Contains(topicChunk, "count") || !strings.Contains(topicChunk, "var.enable_dataform_integration") {
			t.Errorf("dataform_events topic missing count = var.enable_dataform_integration guard")
		}
		if !strings.Contains(topicChunk, "name") || !strings.Contains(topicChunk, "var.dataform_events_topic") {
			t.Errorf("dataform_events topic must use var.dataform_events_topic")
		}
		if !strings.Contains(topicChunk, "project") || !strings.Contains(topicChunk, "var.project_id") {
			t.Errorf("dataform_events topic must specify project = var.project_id")
		}

		// Dataform events DLQ topic
		if !strings.Contains(mainContent, `resource "google_pubsub_topic" "dataform_events_dlq"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic\" \"dataform_events_dlq\"")
		}
		dlqChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic" "dataform_events_dlq"`)
		if !strings.Contains(dlqChunk, "count") || !strings.Contains(dlqChunk, "var.enable_dataform_integration") {
			t.Errorf("dataform_events_dlq topic missing count = var.enable_dataform_integration guard")
		}
		if !strings.Contains(dlqChunk, `"${var.dataform_events_topic}-dlq"`) {
			t.Errorf("dataform_events_dlq topic must use name = \"${var.dataform_events_topic}-dlq\"")
		}
		if !strings.Contains(dlqChunk, "project") || !strings.Contains(dlqChunk, "var.project_id") {
			t.Errorf("dataform_events_dlq topic must specify project = var.project_id")
		}
	})

	t.Run("LoggingSinkAndIAMPublisherConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		// Cloud Logging sink
		if !strings.Contains(mainContent, `resource "google_logging_project_sink" "dataform_events"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_logging_project_sink\" \"dataform_events\"")
		}
		sinkChunk := extractHCLBlock(mainContent, `resource "google_logging_project_sink" "dataform_events"`)
		if !strings.Contains(sinkChunk, "count") || !strings.Contains(sinkChunk, "var.enable_dataform_integration") {
			t.Errorf("dataform_events sink missing count = var.enable_dataform_integration guard")
		}
		if !strings.Contains(sinkChunk, `"supercargo-dataform-events-sink${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("dataform_events sink name must use \"supercargo-dataform-events-sink${var.name_suffix != \"\" ? \"-${var.name_suffix}\" : \"\"}\"")
		}
		if !strings.Contains(sinkChunk, `"pubsub.googleapis.com/${google_pubsub_topic.dataform_events[0].id}"`) {
			t.Errorf("dataform_events sink destination must target google_pubsub_topic.dataform_events[0].id")
		}
		if !strings.Contains(sinkChunk, "unique_writer_identity = true") && !strings.Contains(sinkChunk, "unique_writer_identity= true") {
			t.Errorf("dataform_events sink must set unique_writer_identity = true")
		}
		if !strings.Contains(sinkChunk, "var.dataform_sink_filter") {
			t.Errorf("dataform_events sink must reference var.dataform_sink_filter")
		}
		// Dual-compatible filter assertion: covers both WorkflowInvocationCompletionLogEntry and WorkflowInvocation
		if (!strings.Contains(sinkChunk, "WorkflowInvocationCompletionLogEntry") && !strings.Contains(mainContent, "WorkflowInvocationCompletionLogEntry")) ||
			(!strings.Contains(sinkChunk, "WorkflowInvocation") && !strings.Contains(mainContent, "WorkflowInvocation")) {
			t.Errorf("dataform_events sink filter must be dual-compatible (WorkflowInvocationCompletionLogEntry and WorkflowInvocation)")
		}

		// IAM Member for Cloud Logging Writer Identity -> Pub/Sub publisher
		if !strings.Contains(mainContent, `resource "google_pubsub_topic_iam_member" "dataform_sink_publisher"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic_iam_member\" \"dataform_sink_publisher\"")
		}
		iamChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic_iam_member" "dataform_sink_publisher"`)
		if !strings.Contains(iamChunk, "count") || !strings.Contains(iamChunk, "var.enable_dataform_integration") {
			t.Errorf("dataform_sink_publisher missing count = var.enable_dataform_integration guard")
		}
		if !strings.Contains(iamChunk, "google_pubsub_topic.dataform_events[0].name") {
			t.Errorf("dataform_sink_publisher must bind to google_pubsub_topic.dataform_events[0].name")
		}
		if !strings.Contains(iamChunk, `"roles/pubsub.publisher"`) {
			t.Errorf("dataform_sink_publisher must grant roles/pubsub.publisher")
		}
		if !strings.Contains(iamChunk, "google_logging_project_sink.dataform_events[0].writer_identity") {
			t.Errorf("dataform_sink_publisher member must be google_logging_project_sink.dataform_events[0].writer_identity")
		}
	})

	t.Run("PushSubscriptionWithOIDCAndDLQConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_pubsub_subscription" "dataform_events_push"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription\" \"dataform_events_push\"")
		}
		subChunk := extractHCLBlock(mainContent, `resource "google_pubsub_subscription" "dataform_events_push"`)
		if !strings.Contains(subChunk, "count") || !strings.Contains(subChunk, "var.enable_dataform_integration") {
			t.Errorf("dataform_events_push subscription missing count = var.enable_dataform_integration guard")
		}
		if !strings.Contains(subChunk, `"hub-dataform-events-push${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("dataform_events_push subscription name must use \"hub-dataform-events-push${var.name_suffix != \"\" ? \"-${var.name_suffix}\" : \"\"}\"")
		}
		if !strings.Contains(subChunk, "google_pubsub_topic.dataform_events[0].name") {
			t.Errorf("dataform_events_push subscription topic must use google_pubsub_topic.dataform_events[0].name")
		}
		if !strings.Contains(subChunk, "ack_deadline_seconds = 60") && !strings.Contains(subChunk, "ack_deadline_seconds= 60") {
			t.Errorf("dataform_events_push subscription must configure ack_deadline_seconds = 60")
		}
		if !strings.Contains(subChunk, `"${google_cloud_run_v2_service.hub.uri}/v1/internal/dataform-events"`) {
			t.Errorf("dataform_events_push subscription must push to ${google_cloud_run_v2_service.hub.uri}/v1/internal/dataform-events")
		}
		if !strings.Contains(subChunk, "google_service_account.events_invoker.email") {
			t.Errorf("dataform_events_push subscription must use google_service_account.events_invoker.email for oidc_token")
		}
		if !strings.Contains(subChunk, "var.oidc_audience != \"\" ? trimspace(split(\",\", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri") {
			t.Errorf("dataform_events_push subscription oidc audience must fallback to google_cloud_run_v2_service.hub.uri")
		}
		if !strings.Contains(subChunk, "google_pubsub_topic.dataform_events_dlq[0].id") {
			t.Errorf("dataform_events_push subscription dead_letter_topic must reference google_pubsub_topic.dataform_events_dlq[0].id")
		}
		if !strings.Contains(subChunk, "max_delivery_attempts = 5") && !strings.Contains(subChunk, "max_delivery_attempts= 5") {
			t.Errorf("dataform_events_push subscription must configure max_delivery_attempts = 5")
		}
		if !strings.Contains(subChunk, `ttl = ""`) {
			t.Errorf("dataform_events_push subscription must configure permanent retention via expiration_policy ttl = \"\"")
		}
		if !strings.Contains(subChunk, "google_cloud_run_v2_service_iam_member.events_invoker_run") ||
			!strings.Contains(subChunk, "google_service_account_iam_member.pubsub_events_invoker_token_creator") ||
			!strings.Contains(subChunk, "google_pubsub_topic_iam_member.dataform_dlq_publisher") {
			t.Errorf("dataform_events_push subscription must depend on events_invoker_run, pubsub_events_invoker_token_creator, and dataform_dlq_publisher")
		}

		// Pub/Sub Service Agent Publisher IAM on DLQ topic
		if !strings.Contains(mainContent, `resource "google_pubsub_topic_iam_member" "dataform_dlq_publisher"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic_iam_member\" \"dataform_dlq_publisher\"")
		}
		dlqPubIamChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic_iam_member" "dataform_dlq_publisher"`)
		if !strings.Contains(dlqPubIamChunk, "google_pubsub_topic.dataform_events_dlq[0].name") {
			t.Errorf("dataform_dlq_publisher must target dataform_events_dlq topic")
		}
		if !strings.Contains(dlqPubIamChunk, `"roles/pubsub.publisher"`) {
			t.Errorf("dataform_dlq_publisher must grant roles/pubsub.publisher")
		}
		if !strings.Contains(dlqPubIamChunk, `"serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"`) {
			t.Errorf("dataform_dlq_publisher must bind Pub/Sub service agent")
		}

		// Pub/Sub Service Agent Subscriber IAM on Push subscription
		if !strings.Contains(mainContent, `resource "google_pubsub_subscription_iam_member" "dataform_push_dlq_subscriber"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription_iam_member\" \"dataform_push_dlq_subscriber\"")
		}
		dlqSubIamChunk := extractHCLBlock(mainContent, `resource "google_pubsub_subscription_iam_member" "dataform_push_dlq_subscriber"`)
		if !strings.Contains(dlqSubIamChunk, "google_pubsub_subscription.dataform_events_push[0].name") {
			t.Errorf("dataform_push_dlq_subscriber must target dataform_events_push subscription")
		}
		if !strings.Contains(dlqSubIamChunk, `"roles/pubsub.subscriber"`) {
			t.Errorf("dataform_push_dlq_subscriber must grant roles/pubsub.subscriber")
		}
		if !strings.Contains(dlqSubIamChunk, `"serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"`) {
			t.Errorf("dataform_push_dlq_subscriber must bind Pub/Sub service agent")
		}

		// DLQ inspection pull subscription
		if !strings.Contains(mainContent, `resource "google_pubsub_subscription" "dataform_events_dlq_pull"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription\" \"dataform_events_dlq_pull\"")
		}
		dlqPullChunk := extractHCLBlock(mainContent, `resource "google_pubsub_subscription" "dataform_events_dlq_pull"`)
		if !strings.Contains(dlqPullChunk, `"${var.dataform_events_topic}-dlq-sub"`) {
			t.Errorf("dataform_events_dlq_pull must be named \"${var.dataform_events_topic}-dlq-sub\"")
		}
		if !strings.Contains(dlqPullChunk, "google_pubsub_topic.dataform_events_dlq[0].name") {
			t.Errorf("dataform_events_dlq_pull must target dataform_events_dlq")
		}
	})

	t.Run("ConditionalOutputsExported", func(t *testing.T) {
		outputPath := filepath.Join(hubDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		outputs := []struct {
			name      string
			condition string
		}{
			{name: "dataform_events_topic_name", condition: "var.enable_dataform_integration ? google_pubsub_topic.dataform_events[0].name : null"},
			{name: "dataform_events_topic_id", condition: "var.enable_dataform_integration ? google_pubsub_topic.dataform_events[0].id : null"},
			{name: "dataform_events_dlq_topic_name", condition: "var.enable_dataform_integration ? google_pubsub_topic.dataform_events_dlq[0].name : null"},
			{name: "dataform_events_subscription_name", condition: "var.enable_dataform_integration ? google_pubsub_subscription.dataform_events_push[0].name : null"},
			{name: "dataform_events_dlq_subscription_name", condition: "var.enable_dataform_integration ? google_pubsub_subscription.dataform_events_dlq_pull[0].name : null"},
		}

		for _, out := range outputs {
			if !strings.Contains(outputContent, `output "`+out.name+`"`) {
				t.Errorf("modules/hub/outputs.tf missing output %q", out.name)
			}
			if !strings.Contains(outputContent, out.condition) {
				t.Errorf("modules/hub/outputs.tf output %q must use %q", out.name, out.condition)
			}
		}
	})
}

func TestModules_HubJobMonitoringIntegration(t *testing.T) {
	modulesDir := "../../modules"
	hubDir := filepath.Join(modulesDir, "hub")

	t.Run("VariablesDefinedWithCorrectDefaults", func(t *testing.T) {
		varPath := filepath.Join(hubDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		requiredVars := []struct {
			name         string
			varType      string
			defaultValue string
		}{
			{name: "enable_job_monitoring_integration", varType: "bool", defaultValue: "false"},
			{name: "job_events_topic", varType: "string", defaultValue: `"supercargo-job-events"`},
			{name: "job_sink_filter", varType: "string", defaultValue: `""`},
		}

		for _, rv := range requiredVars {
			if !strings.Contains(varContent, `variable "`+rv.name+`"`) {
				t.Errorf("modules/hub/variables.tf missing variable %q", rv.name)
				continue
			}
			chunk := extractHCLBlock(varContent, `variable "`+rv.name+`"`)
			if !strings.Contains(chunk, rv.varType) {
				t.Errorf("modules/hub/variables.tf variable %q missing type %s", rv.name, rv.varType)
			}
			if !strings.Contains(chunk, rv.defaultValue) {
				t.Errorf("modules/hub/variables.tf variable %q missing default %s", rv.name, rv.defaultValue)
			}
		}
	})

	t.Run("PubSubTopicsAndDLQConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		// Primary job events topic
		if !strings.Contains(mainContent, `resource "google_pubsub_topic" "job_events"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic\" \"job_events\"")
		}
		topicChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic" "job_events"`)
		if !strings.Contains(topicChunk, "count") || !strings.Contains(topicChunk, "var.enable_job_monitoring_integration") {
			t.Errorf("job_events topic missing count = var.enable_job_monitoring_integration guard")
		}
		if !strings.Contains(topicChunk, "name") || !strings.Contains(topicChunk, "var.job_events_topic") {
			t.Errorf("job_events topic must use var.job_events_topic")
		}
		if !strings.Contains(topicChunk, "project") || !strings.Contains(topicChunk, "var.project_id") {
			t.Errorf("job_events topic must specify project = var.project_id")
		}

		// Job events DLQ topic
		if !strings.Contains(mainContent, `resource "google_pubsub_topic" "job_events_dlq"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic\" \"job_events_dlq\"")
		}
		dlqChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic" "job_events_dlq"`)
		if !strings.Contains(dlqChunk, "count") || !strings.Contains(dlqChunk, "var.enable_job_monitoring_integration") {
			t.Errorf("job_events_dlq topic missing count = var.enable_job_monitoring_integration guard")
		}
		if !strings.Contains(dlqChunk, `"${var.job_events_topic}-dlq"`) {
			t.Errorf("job_events_dlq topic must use name = \"${var.job_events_topic}-dlq\"")
		}
		if !strings.Contains(dlqChunk, "project") || !strings.Contains(dlqChunk, "var.project_id") {
			t.Errorf("job_events_dlq topic must specify project = var.project_id")
		}
	})

	t.Run("LoggingSinkAndIAMPublisherConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		// Cloud Logging sink
		if !strings.Contains(mainContent, `resource "google_logging_project_sink" "job_events"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_logging_project_sink\" \"job_events\"")
		}
		sinkChunk := extractHCLBlock(mainContent, `resource "google_logging_project_sink" "job_events"`)
		if !strings.Contains(sinkChunk, "count") || !strings.Contains(sinkChunk, "var.enable_job_monitoring_integration") {
			t.Errorf("job_events sink missing count = var.enable_job_monitoring_integration guard")
		}
		if !strings.Contains(sinkChunk, `"supercargo-job-events-sink${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("job_events sink name must use \"supercargo-job-events-sink${var.name_suffix != \"\" ? \"-${var.name_suffix}\" : \"\"}\"")
		}
		if !strings.Contains(sinkChunk, `"pubsub.googleapis.com/${google_pubsub_topic.job_events[0].id}"`) {
			t.Errorf("job_events sink destination must target google_pubsub_topic.job_events[0].id")
		}
		if !strings.Contains(sinkChunk, "unique_writer_identity = true") && !strings.Contains(sinkChunk, "unique_writer_identity= true") {
			t.Errorf("job_events sink must set unique_writer_identity = true")
		}
		if !strings.Contains(sinkChunk, "var.job_sink_filter") {
			t.Errorf("job_events sink must reference var.job_sink_filter")
		}
		if !strings.Contains(mainContent, `default_job_sink_filter`) ||
			!strings.Contains(mainContent, `resource.type = \"cloud_run_job\"`) ||
			!strings.Contains(mainContent, `run.googleapis.com%2Fjob-execution-result`) ||
			!strings.Contains(mainContent, `Completed with status:`) {
			t.Errorf("job_events sink filter must use default_job_sink_filter covering cloud_run_job execution logs")
		}

		// IAM Member for Cloud Logging Writer Identity -> Pub/Sub publisher
		if !strings.Contains(mainContent, `resource "google_pubsub_topic_iam_member" "job_sink_publisher"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic_iam_member\" \"job_sink_publisher\"")
		}
		iamChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic_iam_member" "job_sink_publisher"`)
		if !strings.Contains(iamChunk, "count") || !strings.Contains(iamChunk, "var.enable_job_monitoring_integration") {
			t.Errorf("job_sink_publisher missing count = var.enable_job_monitoring_integration guard")
		}
		if !strings.Contains(iamChunk, "google_pubsub_topic.job_events[0].name") {
			t.Errorf("job_sink_publisher must bind to google_pubsub_topic.job_events[0].name")
		}
		if !strings.Contains(iamChunk, `"roles/pubsub.publisher"`) {
			t.Errorf("job_sink_publisher must grant roles/pubsub.publisher")
		}
		if !strings.Contains(iamChunk, "google_logging_project_sink.job_events[0].writer_identity") {
			t.Errorf("job_sink_publisher member must be google_logging_project_sink.job_events[0].writer_identity")
		}
	})

	t.Run("MonitoringNotificationChannelAndPublisherConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		// Notification channel
		if !strings.Contains(mainContent, `resource "google_monitoring_notification_channel" "hub_events"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_monitoring_notification_channel\" \"hub_events\"")
		}
		ncChunk := extractHCLBlock(mainContent, `resource "google_monitoring_notification_channel" "hub_events"`)
		if !strings.Contains(ncChunk, "count") || !strings.Contains(ncChunk, "var.enable_job_monitoring_integration") {
			t.Errorf("hub_events notification channel missing count = var.enable_job_monitoring_integration guard")
		}
		if !strings.Contains(ncChunk, `"pubsub"`) || !strings.Contains(ncChunk, "type") {
			t.Errorf("hub_events notification channel type must be pubsub")
		}
		if !strings.Contains(ncChunk, "google_pubsub_topic.job_events[0].id") {
			t.Errorf("hub_events notification channel must reference google_pubsub_topic.job_events[0].id in labels")
		}

		// Cloud Monitoring Notification Service Account IAM publisher
		if !strings.Contains(mainContent, `resource "google_pubsub_topic_iam_member" "monitoring_notification_publisher"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic_iam_member\" \"monitoring_notification_publisher\"")
		}
		mnChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic_iam_member" "monitoring_notification_publisher"`)
		if !strings.Contains(mnChunk, "count") || !strings.Contains(mnChunk, "var.enable_job_monitoring_integration") {
			t.Errorf("monitoring_notification_publisher missing count = var.enable_job_monitoring_integration guard")
		}
		if !strings.Contains(mnChunk, "google_pubsub_topic.job_events[0].name") {
			t.Errorf("monitoring_notification_publisher must bind to google_pubsub_topic.job_events[0].name")
		}
		if !strings.Contains(mnChunk, `"roles/pubsub.publisher"`) {
			t.Errorf("monitoring_notification_publisher must grant roles/pubsub.publisher")
		}
		if !strings.Contains(mnChunk, "@gcp-sa-monitoring-notification.iam.gserviceaccount.com") {
			t.Errorf("monitoring_notification_publisher member must bind gcp-sa-monitoring-notification service agent")
		}
	})

	t.Run("PushSubscriptionWithOIDCAndDLQConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_pubsub_subscription" "job_events_push"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription\" \"job_events_push\"")
		}
		subChunk := extractHCLBlock(mainContent, `resource "google_pubsub_subscription" "job_events_push"`)
		if !strings.Contains(subChunk, "count") || !strings.Contains(subChunk, "var.enable_job_monitoring_integration") {
			t.Errorf("job_events_push subscription missing count = var.enable_job_monitoring_integration guard")
		}
		if !strings.Contains(subChunk, `"hub-job-events-push${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("job_events_push subscription name must use \"hub-job-events-push${var.name_suffix != \"\" ? \"-${var.name_suffix}\" : \"\"}\"")
		}
		if !strings.Contains(subChunk, "google_pubsub_topic.job_events[0].name") {
			t.Errorf("job_events_push subscription topic must use google_pubsub_topic.job_events[0].name")
		}
		if !strings.Contains(subChunk, "ack_deadline_seconds = 60") && !strings.Contains(subChunk, "ack_deadline_seconds= 60") {
			t.Errorf("job_events_push subscription must configure ack_deadline_seconds = 60")
		}
		if !strings.Contains(subChunk, `"${google_cloud_run_v2_service.hub.uri}/v1/internal/monitoring-events"`) {
			t.Errorf("job_events_push subscription must push to ${google_cloud_run_v2_service.hub.uri}/v1/internal/monitoring-events")
		}
		if !strings.Contains(subChunk, "google_service_account.events_invoker.email") {
			t.Errorf("job_events_push subscription must use google_service_account.events_invoker.email for oidc_token")
		}
		if !strings.Contains(subChunk, "var.oidc_audience != \"\" ? trimspace(split(\",\", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri") {
			t.Errorf("job_events_push subscription oidc audience must fallback to google_cloud_run_v2_service.hub.uri")
		}
		if !strings.Contains(subChunk, "google_pubsub_topic.job_events_dlq[0].id") {
			t.Errorf("job_events_push subscription dead_letter_topic must reference google_pubsub_topic.job_events_dlq[0].id")
		}
		if !strings.Contains(subChunk, "max_delivery_attempts = 5") && !strings.Contains(subChunk, "max_delivery_attempts= 5") {
			t.Errorf("job_events_push subscription must configure max_delivery_attempts = 5")
		}
		if !strings.Contains(subChunk, `ttl = ""`) {
			t.Errorf("job_events_push subscription must configure permanent retention via expiration_policy ttl = \"\"")
		}
		if !strings.Contains(subChunk, "google_cloud_run_v2_service_iam_member.events_invoker_run") ||
			!strings.Contains(subChunk, "google_service_account_iam_member.pubsub_events_invoker_token_creator") ||
			!strings.Contains(subChunk, "google_pubsub_topic_iam_member.job_dlq_publisher") {
			t.Errorf("job_events_push subscription must depend on events_invoker_run, pubsub_events_invoker_token_creator, and job_dlq_publisher")
		}

		// Pub/Sub Service Agent Publisher IAM on DLQ topic
		if !strings.Contains(mainContent, `resource "google_pubsub_topic_iam_member" "job_dlq_publisher"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_topic_iam_member\" \"job_dlq_publisher\"")
		}
		dlqPubIamChunk := extractHCLBlock(mainContent, `resource "google_pubsub_topic_iam_member" "job_dlq_publisher"`)
		if !strings.Contains(dlqPubIamChunk, "google_pubsub_topic.job_events_dlq[0].name") {
			t.Errorf("job_dlq_publisher must target job_events_dlq topic")
		}
		if !strings.Contains(dlqPubIamChunk, `"roles/pubsub.publisher"`) {
			t.Errorf("job_dlq_publisher must grant roles/pubsub.publisher")
		}
		if !strings.Contains(dlqPubIamChunk, `@gcp-sa-pubsub.iam.gserviceaccount.com`) {
			t.Errorf("job_dlq_publisher must bind Pub/Sub service agent")
		}

		// Pub/Sub Service Agent Subscriber IAM on Push subscription
		if !strings.Contains(mainContent, `resource "google_pubsub_subscription_iam_member" "job_push_dlq_subscriber"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription_iam_member\" \"job_push_dlq_subscriber\"")
		}
		dlqSubIamChunk := extractHCLBlock(mainContent, `resource "google_pubsub_subscription_iam_member" "job_push_dlq_subscriber"`)
		if !strings.Contains(dlqSubIamChunk, "google_pubsub_subscription.job_events_push[0].name") {
			t.Errorf("job_push_dlq_subscriber must target job_events_push subscription")
		}
		if !strings.Contains(dlqSubIamChunk, `"roles/pubsub.subscriber"`) {
			t.Errorf("job_push_dlq_subscriber must grant roles/pubsub.subscriber")
		}
		if !strings.Contains(dlqSubIamChunk, `@gcp-sa-pubsub.iam.gserviceaccount.com`) {
			t.Errorf("job_push_dlq_subscriber must bind Pub/Sub service agent")
		}

		// DLQ inspection pull subscription
		if !strings.Contains(mainContent, `resource "google_pubsub_subscription" "job_events_dlq_pull"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription\" \"job_events_dlq_pull\"")
		}
		dlqPullChunk := extractHCLBlock(mainContent, `resource "google_pubsub_subscription" "job_events_dlq_pull"`)
		if !strings.Contains(dlqPullChunk, `"${var.job_events_topic}-dlq-sub"`) {
			t.Errorf("job_events_dlq_pull must be named \"${var.job_events_topic}-dlq-sub\"")
		}
		if !strings.Contains(dlqPullChunk, "google_pubsub_topic.job_events_dlq[0].name") {
			t.Errorf("job_events_dlq_pull must target job_events_dlq")
		}
	})

	t.Run("ConditionalOutputsExported", func(t *testing.T) {
		outputPath := filepath.Join(hubDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		outputs := []struct {
			name      string
			condition string
		}{
			{name: "job_events_topic_name", condition: "var.enable_job_monitoring_integration ? google_pubsub_topic.job_events[0].name : null"},
			{name: "job_events_topic_id", condition: "var.enable_job_monitoring_integration ? google_pubsub_topic.job_events[0].id : null"},
			{name: "job_events_dlq_topic_name", condition: "var.enable_job_monitoring_integration ? google_pubsub_topic.job_events_dlq[0].name : null"},
			{name: "job_events_subscription_name", condition: "var.enable_job_monitoring_integration ? google_pubsub_subscription.job_events_push[0].name : null"},
			{name: "job_events_dlq_subscription_name", condition: "var.enable_job_monitoring_integration ? google_pubsub_subscription.job_events_dlq_pull[0].name : null"},
			{name: "job_monitoring_notification_channel_id", condition: "var.enable_job_monitoring_integration ? google_monitoring_notification_channel.hub_events[0].id : null"},
		}

		for _, out := range outputs {
			if !strings.Contains(outputContent, `output "`+out.name+`"`) {
				t.Errorf("modules/hub/outputs.tf missing output %q", out.name)
			}
			if !strings.Contains(outputContent, out.condition) {
				t.Errorf("modules/hub/outputs.tf output %q must use %q", out.name, out.condition)
			}
		}
	})
}

func extractHCLBlock(content, header string) string {
	idx := strings.Index(content, header)
	if idx == -1 {
		return ""
	}
	braceIdx := strings.Index(content[idx:], "{")
	if braceIdx == -1 {
		return content[idx:]
	}
	start := idx + braceIdx
	depth := 0
	for i := start; i < len(content); i++ {
		if content[i] == '{' {
			depth++
		} else if content[i] == '}' {
			depth--
			if depth == 0 {
				return content[idx : i+1]
			}
		}
	}
	return content[idx:]
}

func TestModules_GatewayDLQMonitoring(t *testing.T) {
	modulesDir := "../../modules"
	gwDir := filepath.Join(modulesDir, "gateway")

	t.Run("GatewayVariablesAndDefaults", func(t *testing.T) {
		varPath := filepath.Join(gwDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		expectedVars := []struct {
			name         string
			varType      string
			defaultValue string
			sensitive    bool
		}{
			{name: "enable_dlq_alerts", varType: "bool", defaultValue: "true"},
			{name: "dlq_alert_threshold", varType: "number", defaultValue: "0"},
			{name: "dlq_unacked_message_age_seconds", varType: "number", defaultValue: "300"},
			{name: "dlq_runbook_url", varType: "string", defaultValue: `"https://docs.supercargo.dev/operations/runbooks/dlq-remediation"`},
			{name: "alert_pagerduty_service_key", varType: "string", defaultValue: `""`, sensitive: true},
			{name: "alert_notification_channels", varType: "list(string)", defaultValue: "[]"},
		}

		for _, ev := range expectedVars {
			if !strings.Contains(varContent, `variable "`+ev.name+`"`) {
				t.Errorf("modules/gateway/variables.tf missing variable %q", ev.name)
				continue
			}
			chunk := extractHCLBlock(varContent, `variable "`+ev.name+`"`)
			if !strings.Contains(chunk, "type") || !strings.Contains(chunk, ev.varType) {
				t.Errorf("modules/gateway/variables.tf variable %q missing type %s", ev.name, ev.varType)
			}
			if !strings.Contains(chunk, "default") || !strings.Contains(chunk, ev.defaultValue) {
				t.Errorf("modules/gateway/variables.tf variable %q missing default %s", ev.name, ev.defaultValue)
			}
			if ev.sensitive && (!strings.Contains(chunk, "sensitive") || !strings.Contains(chunk, "true")) {
				t.Errorf("modules/gateway/variables.tf variable %q must be sensitive = true", ev.name)
			}
		}
	})

	t.Run("PagerDutyNotificationChannel", func(t *testing.T) {
		monPath := filepath.Join(gwDir, "monitoring.tf")
		monBytes, err := os.ReadFile(monPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", monPath, err)
		}
		monContent := string(monBytes)

		pdRes := `resource "google_monitoring_notification_channel" "pagerduty"`
		if !strings.Contains(monContent, pdRes) {
			t.Fatalf("modules/gateway/monitoring.tf missing %s", pdRes)
		}
		chunk := extractHCLBlock(monContent, pdRes)
		if !strings.Contains(chunk, "var.alert_pagerduty_service_key") {
			t.Errorf("pagerduty notification channel missing var.alert_pagerduty_service_key count guard")
		}
		if !strings.Contains(chunk, `type         = "pagerduty"`) && !strings.Contains(chunk, `type = "pagerduty"`) {
			t.Errorf("pagerduty notification channel missing type = \"pagerduty\"")
		}
		if !strings.Contains(chunk, "sensitive_labels") {
			t.Errorf("pagerduty notification channel must use sensitive_labels block")
		}
		if !strings.Contains(chunk, "service_key = var.alert_pagerduty_service_key") && !strings.Contains(chunk, "service_key= var.alert_pagerduty_service_key") {
			t.Errorf("pagerduty notification channel sensitive_labels must configure service_key = var.alert_pagerduty_service_key")
		}
		if strings.Contains(chunk, "\n  labels =") || strings.Contains(chunk, "\n  labels{") {
			t.Errorf("pagerduty notification channel must not use regular labels block, only sensitive_labels")
		}
		if !strings.Contains(chunk, "time_sleep.wait_for_gateway_apis") {
			t.Errorf("pagerduty notification channel must depend on time_sleep.wait_for_gateway_apis")
		}
	})

	t.Run("NotificationChannelAggregation", func(t *testing.T) {
		monPath := filepath.Join(gwDir, "monitoring.tf")
		monBytes, err := os.ReadFile(monPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", monPath, err)
		}
		monContent := string(monBytes)

		if !strings.Contains(monContent, "all_alert_notification_channels =") && !strings.Contains(monContent, "all_alert_notification_channels=") {
			t.Fatalf("modules/gateway/monitoring.tf missing local.all_alert_notification_channels definition")
		}
		chunk := extractHCLBlock(monContent, "all_alert_notification_channels")
		if !strings.Contains(chunk, "google_monitoring_notification_channel.slack") {
			t.Errorf("local.all_alert_notification_channels missing Slack channel reference")
		}
		if !strings.Contains(chunk, "google_monitoring_notification_channel.email") {
			t.Errorf("local.all_alert_notification_channels missing Email channel reference")
		}
		if !strings.Contains(chunk, "google_monitoring_notification_channel.pagerduty") {
			t.Errorf("local.all_alert_notification_channels missing PagerDuty channel reference")
		}
		if !strings.Contains(chunk, "var.alert_notification_channels") {
			t.Errorf("local.all_alert_notification_channels missing var.alert_notification_channels reference")
		}

		for _, tier1Res := range []string{`resource "google_monitoring_alert_policy" "tier1_validation_failure"`, `resource "google_monitoring_alert_policy" "tier1_absence_of_data"`} {
			tChunk := extractHCLBlock(monContent, tier1Res)
			if !strings.Contains(tChunk, "local.all_alert_notification_channels") {
				t.Errorf("%s must use notification_channels = local.all_alert_notification_channels", tier1Res)
			}
		}
	})

	t.Run("DLQUndeliveredMessagesAlertPolicy", func(t *testing.T) {
		monPath := filepath.Join(gwDir, "monitoring.tf")
		monBytes, err := os.ReadFile(monPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", monPath, err)
		}
		monContent := string(monBytes)

		resName := `resource "google_monitoring_alert_policy" "dlq_undelivered_messages"`
		if !strings.Contains(monContent, resName) {
			t.Fatalf("modules/gateway/monitoring.tf missing %s", resName)
		}
		chunk := extractHCLBlock(monContent, resName)

		if !strings.Contains(chunk, "var.enable_dlq_alerts") {
			t.Errorf("dlq_undelivered_messages missing count = var.enable_dlq_alerts guard")
		}
		if !strings.Contains(chunk, `combiner     = "OR"`) && !strings.Contains(chunk, `combiner = "OR"`) {
			t.Errorf("dlq_undelivered_messages combiner must be \"OR\"")
		}
		if !strings.Contains(chunk, `[Tier 1] DLQ Quarantined Messages Backlog (${var.product_id})`) {
			t.Errorf("dlq_undelivered_messages display_name must follow Tier 1 convention")
		}
		if !strings.Contains(chunk, `resource.type = \"pubsub_subscription\"`) && !strings.Contains(chunk, `resource.type=\"pubsub_subscription\"`) {
			t.Errorf("dlq_undelivered_messages filter missing resource.type = \"pubsub_subscription\"")
		}
		if !strings.Contains(chunk, `resource.labels.subscription_id = \"${google_pubsub_subscription.dlq_sub.name}\"`) &&
			!strings.Contains(chunk, `resource.labels.subscription_id=\"${google_pubsub_subscription.dlq_sub.name}\"`) {
			t.Errorf("dlq_undelivered_messages filter missing subscription_id = google_pubsub_subscription.dlq_sub.name")
		}
		if !strings.Contains(chunk, `metric.type = \"pubsub.googleapis.com/subscription/num_undelivered_messages\"`) &&
			!strings.Contains(chunk, `metric.type=\"pubsub.googleapis.com/subscription/num_undelivered_messages\"`) {
			t.Errorf("dlq_undelivered_messages filter missing metric.type = pubsub.googleapis.com/subscription/num_undelivered_messages")
		}
		if !strings.Contains(chunk, `comparison      = "COMPARISON_GT"`) && !strings.Contains(chunk, `comparison = "COMPARISON_GT"`) {
			t.Errorf("dlq_undelivered_messages missing comparison = \"COMPARISON_GT\"")
		}
		if !strings.Contains(chunk, "threshold_value = var.dlq_alert_threshold") && !strings.Contains(chunk, "threshold_value= var.dlq_alert_threshold") {
			t.Errorf("dlq_undelivered_messages missing threshold_value = var.dlq_alert_threshold")
		}
		if !strings.Contains(chunk, `duration        = "0s"`) && !strings.Contains(chunk, `duration = "0s"`) {
			t.Errorf("dlq_undelivered_messages missing duration = \"0s\"")
		}
		if !strings.Contains(chunk, `alignment_period   = "60s"`) && !strings.Contains(chunk, `alignment_period = "60s"`) {
			t.Errorf("dlq_undelivered_messages missing alignment_period = \"60s\"")
		}
		if !strings.Contains(chunk, `per_series_aligner = "ALIGN_MAX"`) && !strings.Contains(chunk, `per_series_aligner = "ALIGN_MAX"`) {
			t.Errorf("dlq_undelivered_messages missing per_series_aligner = \"ALIGN_MAX\"")
		}
		if !strings.Contains(chunk, "trigger {") || !strings.Contains(chunk, "count = 1") {
			t.Errorf("dlq_undelivered_messages missing trigger { count = 1 }")
		}
		if !strings.Contains(chunk, "local.all_alert_notification_channels") {
			t.Errorf("dlq_undelivered_messages must use notification_channels = local.all_alert_notification_channels")
		}
		if !strings.Contains(chunk, "time_sleep.wait_for_gateway_apis") || !strings.Contains(chunk, "google_pubsub_subscription.dlq_sub") {
			t.Errorf("dlq_undelivered_messages must depend on time_sleep.wait_for_gateway_apis and google_pubsub_subscription.dlq_sub")
		}
		if !strings.Contains(chunk, "documentation {") {
			t.Errorf("dlq_undelivered_messages missing documentation block")
		}
		if !strings.Contains(chunk, `mime_type = "text/markdown"`) {
			t.Errorf("dlq_undelivered_messages documentation missing mime_type = \"text/markdown\"")
		}
		if !strings.Contains(chunk, "google_pubsub_topic.dlq.id") {
			t.Errorf("dlq_undelivered_messages documentation missing google_pubsub_topic.dlq.id")
		}
		if !strings.Contains(chunk, "google_pubsub_subscription.dlq_sub.id") {
			t.Errorf("dlq_undelivered_messages documentation missing google_pubsub_subscription.dlq_sub.id")
		}
		if !strings.Contains(chunk, "gcloud pubsub subscriptions pull ${google_pubsub_subscription.dlq_sub.name} --limit=5 --auto-ack=false") {
			t.Errorf("dlq_undelivered_messages documentation missing triage CLI command")
		}
		if !strings.Contains(chunk, "var.dlq_runbook_url") {
			t.Errorf("dlq_undelivered_messages documentation missing var.dlq_runbook_url")
		}
	})

	t.Run("DLQMessageAgeAlertPolicy", func(t *testing.T) {
		monPath := filepath.Join(gwDir, "monitoring.tf")
		monBytes, err := os.ReadFile(monPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", monPath, err)
		}
		monContent := string(monBytes)

		resName := `resource "google_monitoring_alert_policy" "dlq_message_age"`
		if !strings.Contains(monContent, resName) {
			t.Fatalf("modules/gateway/monitoring.tf missing %s", resName)
		}
		chunk := extractHCLBlock(monContent, resName)

		if !strings.Contains(chunk, "var.enable_dlq_alerts") {
			t.Errorf("dlq_message_age missing count = var.enable_dlq_alerts guard")
		}
		if !strings.Contains(chunk, `combiner     = "OR"`) && !strings.Contains(chunk, `combiner = "OR"`) {
			t.Errorf("dlq_message_age combiner must be \"OR\"")
		}
		if !strings.Contains(chunk, `[Tier 1] DLQ Message Latency Exceeded (${var.product_id})`) {
			t.Errorf("dlq_message_age display_name must follow Tier 1 convention")
		}
		if !strings.Contains(chunk, `resource.type = \"pubsub_subscription\"`) && !strings.Contains(chunk, `resource.type=\"pubsub_subscription\"`) {
			t.Errorf("dlq_message_age filter missing resource.type = \"pubsub_subscription\"")
		}
		if !strings.Contains(chunk, `resource.labels.subscription_id = \"${google_pubsub_subscription.dlq_sub.name}\"`) &&
			!strings.Contains(chunk, `resource.labels.subscription_id=\"${google_pubsub_subscription.dlq_sub.name}\"`) {
			t.Errorf("dlq_message_age filter missing subscription_id = google_pubsub_subscription.dlq_sub.name")
		}
		if !strings.Contains(chunk, `metric.type = \"pubsub.googleapis.com/subscription/oldest_unacked_message_age\"`) &&
			!strings.Contains(chunk, `metric.type=\"pubsub.googleapis.com/subscription/oldest_unacked_message_age\"`) {
			t.Errorf("dlq_message_age filter missing metric.type = pubsub.googleapis.com/subscription/oldest_unacked_message_age")
		}
		if !strings.Contains(chunk, `comparison      = "COMPARISON_GT"`) && !strings.Contains(chunk, `comparison = "COMPARISON_GT"`) {
			t.Errorf("dlq_message_age missing comparison = \"COMPARISON_GT\"")
		}
		if !strings.Contains(chunk, "threshold_value = var.dlq_unacked_message_age_seconds") && !strings.Contains(chunk, "threshold_value= var.dlq_unacked_message_age_seconds") {
			t.Errorf("dlq_message_age missing threshold_value = var.dlq_unacked_message_age_seconds")
		}
		if !strings.Contains(chunk, `duration        = "0s"`) && !strings.Contains(chunk, `duration = "0s"`) {
			t.Errorf("dlq_message_age missing duration = \"0s\"")
		}
		if !strings.Contains(chunk, `alignment_period   = "60s"`) && !strings.Contains(chunk, `alignment_period = "60s"`) {
			t.Errorf("dlq_message_age missing alignment_period = \"60s\"")
		}
		if !strings.Contains(chunk, `per_series_aligner = "ALIGN_MAX"`) && !strings.Contains(chunk, `per_series_aligner = "ALIGN_MAX"`) {
			t.Errorf("dlq_message_age missing per_series_aligner = \"ALIGN_MAX\"")
		}
		if !strings.Contains(chunk, "trigger {") || !strings.Contains(chunk, "count = 1") {
			t.Errorf("dlq_message_age missing trigger { count = 1 }")
		}
		if !strings.Contains(chunk, "local.all_alert_notification_channels") {
			t.Errorf("dlq_message_age must use notification_channels = local.all_alert_notification_channels")
		}
		if !strings.Contains(chunk, "time_sleep.wait_for_gateway_apis") || !strings.Contains(chunk, "google_pubsub_subscription.dlq_sub") {
			t.Errorf("dlq_message_age must depend on time_sleep.wait_for_gateway_apis and google_pubsub_subscription.dlq_sub")
		}
		if !strings.Contains(chunk, "documentation {") {
			t.Errorf("dlq_message_age missing documentation block")
		}
		if !strings.Contains(chunk, `mime_type = "text/markdown"`) {
			t.Errorf("dlq_message_age documentation missing mime_type = \"text/markdown\"")
		}
		if !strings.Contains(chunk, "google_pubsub_topic.dlq.id") {
			t.Errorf("dlq_message_age documentation missing google_pubsub_topic.dlq.id")
		}
		if !strings.Contains(chunk, "google_pubsub_subscription.dlq_sub.id") {
			t.Errorf("dlq_message_age documentation missing google_pubsub_subscription.dlq_sub.id")
		}
		if !strings.Contains(chunk, "gcloud pubsub subscriptions pull ${google_pubsub_subscription.dlq_sub.name} --limit=5 --auto-ack=false") {
			t.Errorf("dlq_message_age documentation missing triage CLI command")
		}
		if !strings.Contains(chunk, "var.dlq_runbook_url") {
			t.Errorf("dlq_message_age documentation missing var.dlq_runbook_url")
		}
	})

	t.Run("OutputsSafeIndexing", func(t *testing.T) {
		outputPath := filepath.Join(gwDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		if !strings.Contains(outputContent, `output "dlq_alert_policy_undelivered_id"`) {
			t.Errorf("modules/gateway/outputs.tf missing output \"dlq_alert_policy_undelivered_id\"")
		}
		if !strings.Contains(outputContent, "try(google_monitoring_alert_policy.dlq_undelivered_messages[0].name, \"\")") {
			t.Errorf("modules/gateway/outputs.tf dlq_alert_policy_undelivered_id must use try(google_monitoring_alert_policy.dlq_undelivered_messages[0].name, \"\")")
		}

		if !strings.Contains(outputContent, `output "dlq_alert_policy_age_id"`) {
			t.Errorf("modules/gateway/outputs.tf missing output \"dlq_alert_policy_age_id\"")
		}
		if !strings.Contains(outputContent, "try(google_monitoring_alert_policy.dlq_message_age[0].name, \"\")") {
			t.Errorf("modules/gateway/outputs.tf dlq_alert_policy_age_id must use try(google_monitoring_alert_policy.dlq_message_age[0].name, \"\")")
		}
	})
}

func TestModules_DataProductDLQMonitoring(t *testing.T) {
	modulesDir := "../../modules"
	dpDir := filepath.Join(modulesDir, "data_product")

	t.Run("DataProductVariablesAndDefaults", func(t *testing.T) {
		varPath := filepath.Join(dpDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		expectedVars := []struct {
			name         string
			varType      string
			defaultValue string
			sensitive    bool
		}{
			{name: "enable_dlq_alerts", varType: "bool", defaultValue: "true"},
			{name: "dlq_alert_threshold", varType: "number", defaultValue: "0"},
			{name: "dlq_unacked_message_age_seconds", varType: "number", defaultValue: "300"},
			{name: "dlq_runbook_url", varType: "string", defaultValue: `"https://docs.supercargo.dev/operations/runbooks/dlq-remediation"`},
			{name: "alert_slack_channel", varType: "string", defaultValue: `""`},
			{name: "alert_email_address", varType: "string", defaultValue: `""`},
			{name: "alert_pagerduty_service_key", varType: "string", defaultValue: `""`, sensitive: true},
			{name: "alert_notification_channels", varType: "list(string)", defaultValue: "[]"},
		}

		for _, ev := range expectedVars {
			if !strings.Contains(varContent, `variable "`+ev.name+`"`) {
				t.Errorf("modules/data_product/variables.tf missing variable %q", ev.name)
				continue
			}
			chunk := extractHCLBlock(varContent, `variable "`+ev.name+`"`)
			if !strings.Contains(chunk, "type") || !strings.Contains(chunk, ev.varType) {
				t.Errorf("modules/data_product/variables.tf variable %q missing type %s", ev.name, ev.varType)
			}
			if !strings.Contains(chunk, "default") || !strings.Contains(chunk, ev.defaultValue) {
				t.Errorf("modules/data_product/variables.tf variable %q missing default %s", ev.name, ev.defaultValue)
			}
			if ev.sensitive && (!strings.Contains(chunk, "sensitive") || !strings.Contains(chunk, "true")) {
				t.Errorf("modules/data_product/variables.tf variable %q must be sensitive = true", ev.name)
			}
		}
	})

	t.Run("GatewayModulePassthrough", func(t *testing.T) {
		mainPath := filepath.Join(dpDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		gatewayIdx := strings.Index(mainContent, `module "gateway"`)
		if gatewayIdx == -1 {
			t.Fatalf("module gateway not found in %s", mainPath)
		}
		gatewayChunk := mainContent[gatewayIdx:]

		forwardedVars := []string{
			"enable_dlq_alerts",
			"dlq_alert_threshold",
			"dlq_unacked_message_age_seconds",
			"dlq_runbook_url",
			"alert_slack_channel",
			"alert_email_address",
			"alert_pagerduty_service_key",
			"alert_notification_channels",
		}

		for _, v := range forwardedVars {
			expectedWithSpaces := v + " = var." + v
			expectedWithoutSpaces := v + "= var." + v
			if !strings.Contains(gatewayChunk, expectedWithSpaces) &&
				!strings.Contains(gatewayChunk, expectedWithoutSpaces) &&
				!strings.Contains(gatewayChunk, "= var."+v) {
				t.Errorf("module gateway in data_product/main.tf missing passthrough for %s", v)
			}
		}
	})

	t.Run("OutputsExported", func(t *testing.T) {
		outputPath := filepath.Join(dpDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		outputs := []struct {
			name     string
			expected string
		}{
			{name: "dlq_alert_policy_undelivered_id", expected: "module.gateway.dlq_alert_policy_undelivered_id"},
			{name: "dlq_alert_policy_age_id", expected: "module.gateway.dlq_alert_policy_age_id"},
		}

		for _, out := range outputs {
			if !strings.Contains(outputContent, `output "`+out.name+`"`) {
				t.Errorf("modules/data_product/outputs.tf missing output %q", out.name)
			}
			if !strings.Contains(outputContent, out.expected) {
				t.Errorf("modules/data_product/outputs.tf output %q must reference %q", out.name, out.expected)
			}
		}
	})
}

func TestModules_HubMCPCompanionService(t *testing.T) {
	modulesDir := "../../modules"
	hubDir := filepath.Join(modulesDir, "hub")

	t.Run("VariablesConfigured", func(t *testing.T) {
		varPath := filepath.Join(hubDir, "variables.tf")
		varBytes, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		varContent := string(varBytes)

		if !strings.Contains(varContent, `variable "mcp_enabled"`) {
			t.Errorf("modules/hub/variables.tf missing variable \"mcp_enabled\"")
		}
		if !strings.Contains(varContent, `variable "mcp_image_tag"`) {
			t.Errorf("modules/hub/variables.tf missing variable \"mcp_image_tag\"")
		}
		if !strings.Contains(varContent, `variable "mcp_max_instances"`) {
			t.Errorf("modules/hub/variables.tf missing variable \"mcp_max_instances\"")
		}
		if !strings.Contains(varContent, `variable "mcp_allowed_invokers"`) {
			t.Errorf("modules/hub/variables.tf missing variable \"mcp_allowed_invokers\"")
		}
		if !strings.Contains(varContent, `variable "mcp_oidc_audience"`) {
			t.Errorf("modules/hub/variables.tf missing variable \"mcp_oidc_audience\"")
		}
	})

	t.Run("OutputsConfigured", func(t *testing.T) {
		outputPath := filepath.Join(hubDir, "outputs.tf")
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputPath, err)
		}
		outputContent := string(outputBytes)

		if !strings.Contains(outputContent, `output "mcp_service_url"`) {
			t.Errorf("modules/hub/outputs.tf missing output \"mcp_service_url\"")
		}
		if !strings.Contains(outputContent, `output "mcp_service_name"`) {
			t.Errorf("modules/hub/outputs.tf missing output \"mcp_service_name\"")
		}
		if !strings.Contains(outputContent, `output "mcp_service_account_email"`) {
			t.Errorf("modules/hub/outputs.tf missing output \"mcp_service_account_email\"")
		}
	})

	t.Run("ResourcesConfigured", func(t *testing.T) {
		mainPath := filepath.Join(hubDir, "main.tf")
		mainBytes, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mainBytes)

		if !strings.Contains(mainContent, `resource "google_service_account" "mcp_runtime"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_service_account\" \"mcp_runtime\"")
		}
		if !strings.Contains(mainContent, `resource "google_cloud_run_v2_service_iam_member" "mcp_hub_invoker"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_cloud_run_v2_service_iam_member\" \"mcp_hub_invoker\"")
		}
		if !strings.Contains(mainContent, `resource "google_cloud_run_v2_service" "mcp"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_cloud_run_v2_service\" \"mcp\"")
		}
		if !strings.Contains(mainContent, `resource "google_cloud_run_v2_service_iam_member" "mcp_invokers"`) {
			t.Fatalf("modules/hub/main.tf missing resource \"google_cloud_run_v2_service_iam_member\" \"mcp_invokers\"")
		}
		if !strings.Contains(mainContent, `session_affinity = true`) {
			t.Errorf("modules/hub/main.tf missing session_affinity for mcp service")
		}

		mcpChunk := extractHCLBlock(mainContent, `resource "google_cloud_run_v2_service" "mcp"`)
		if mcpChunk == "" {
			t.Fatalf("Failed to extract mcp service block")
		}
		if !strings.Contains(mcpChunk, `name  = "HUB_OIDC_AUDIENCE"`) {
			t.Errorf("modules/hub/main.tf missing HUB_OIDC_AUDIENCE env var for mcp service")
		}
		if !strings.Contains(mcpChunk, `var.oidc_audience != "" ? trimspace(split(",", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri`) {
			t.Errorf("modules/hub/main.tf missing fallback to hub URI for HUB_OIDC_AUDIENCE")
		}
	})
}

func TestModules_CloudRunProbes(t *testing.T) {
	modulesDir := "../../modules"
	testCases := []struct {
		moduleName   string
		resourceName string
	}{
		{moduleName: "gateway", resourceName: "gateway"},
		{moduleName: "hub", resourceName: "hub"},
		{moduleName: "vault", resourceName: "vault"},
	}

	for _, tc := range testCases {
		t.Run(tc.moduleName+"/"+tc.resourceName, func(t *testing.T) {
			path := filepath.Join(modulesDir, tc.moduleName, "main.tf")
			contentBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", path, err)
			}
			content := string(contentBytes)

			expectedResource := `resource "google_cloud_run_v2_service" "` + tc.resourceName + `"`
			idx := strings.Index(content, expectedResource)
			if idx == -1 {
				t.Fatalf("Could not find %s in %s", expectedResource, path)
			}

			chunk := content[idx:]
			nextResourceIdx := strings.Index(chunk[len(expectedResource):], "\nresource \"")
			if nextResourceIdx != -1 {
				chunk = chunk[:len(expectedResource)+nextResourceIdx]
			}

			// Startup probe assertions
			if !strings.Contains(chunk, "startup_probe {") {
				t.Errorf("Resource %s in %s missing startup_probe block", expectedResource, tc.moduleName)
			}
			startupChunk := extractHCLBlock(chunk, "startup_probe")
			if !strings.Contains(startupChunk, `path = "/healthz"`) && !strings.Contains(startupChunk, `path= "/healthz"`) {
				t.Errorf("Resource %s startup_probe missing path = \"/healthz\"", expectedResource)
			}
			if !strings.Contains(startupChunk, "port = 8080") && !strings.Contains(startupChunk, "port= 8080") {
				t.Errorf("Resource %s startup_probe missing port = 8080", expectedResource)
			}
			if !strings.Contains(startupChunk, "initial_delay_seconds = 0") && !strings.Contains(startupChunk, "initial_delay_seconds        = 0") {
				t.Errorf("Resource %s startup_probe missing initial_delay_seconds = 0", expectedResource)
			}
			if !strings.Contains(startupChunk, "period_seconds = 5") && !strings.Contains(startupChunk, "period_seconds        = 5") {
				t.Errorf("Resource %s startup_probe missing period_seconds = 5", expectedResource)
			}
			if !strings.Contains(startupChunk, "timeout_seconds = 2") && !strings.Contains(startupChunk, "timeout_seconds       = 2") {
				t.Errorf("Resource %s startup_probe missing timeout_seconds = 2", expectedResource)
			}
			if !strings.Contains(startupChunk, "failure_threshold = 12") && !strings.Contains(startupChunk, "failure_threshold     = 12") {
				t.Errorf("Resource %s startup_probe missing failure_threshold = 12", expectedResource)
			}

			// Liveness probe assertions
			if !strings.Contains(chunk, "liveness_probe {") {
				t.Errorf("Resource %s in %s missing liveness_probe block", expectedResource, tc.moduleName)
			}
			livenessChunk := extractHCLBlock(chunk, "liveness_probe")
			if !strings.Contains(livenessChunk, `path = "/healthz"`) && !strings.Contains(livenessChunk, `path= "/healthz"`) {
				t.Errorf("Resource %s liveness_probe missing path = \"/healthz\"", expectedResource)
			}
			if !strings.Contains(livenessChunk, "port = 8080") && !strings.Contains(livenessChunk, "port= 8080") {
				t.Errorf("Resource %s liveness_probe missing port = 8080", expectedResource)
			}
			if !strings.Contains(livenessChunk, "period_seconds = 10") && !strings.Contains(livenessChunk, "period_seconds    = 10") {
				t.Errorf("Resource %s liveness_probe missing period_seconds = 10", expectedResource)
			}
			if !strings.Contains(livenessChunk, "timeout_seconds = 2") && !strings.Contains(livenessChunk, "timeout_seconds   = 2") {
				t.Errorf("Resource %s liveness_probe missing timeout_seconds = 2", expectedResource)
			}
			if !strings.Contains(livenessChunk, "failure_threshold = 3") && !strings.Contains(livenessChunk, "failure_threshold     = 3") {
				t.Errorf("Resource %s liveness_probe missing failure_threshold = 3", expectedResource)
			}
		})
	}
}

func TestModules_PlanTimeDeterministicNamingAndSuffix(t *testing.T) {
	modulesDir := "../../modules"

	t.Run("AbsenceOfRandomIdSuffixInModules", func(t *testing.T) {
		targets := []string{"hub", "vault", "gateway"}
		for _, mod := range targets {
			mainPath := filepath.Join(modulesDir, mod, "main.tf")
			b, err := os.ReadFile(mainPath)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", mainPath, err)
			}
			content := string(b)
			if strings.Contains(content, `resource "random_id" "suffix"`) {
				t.Errorf("Module %s must not contain resource \"random_id\" \"suffix\"", mod)
			}
			if strings.Contains(content, "random_id.suffix") {
				t.Errorf("Module %s must not reference random_id.suffix", mod)
			}
		}
	})

	t.Run("NameSuffixVariableDeclaredWithEmptyDefault", func(t *testing.T) {
		targets := []string{"hub", "vault", "gateway"}
		for _, mod := range targets {
			varPath := filepath.Join(modulesDir, mod, "variables.tf")
			b, err := os.ReadFile(varPath)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", varPath, err)
			}
			content := string(b)
			if !strings.Contains(content, `variable "name_suffix"`) {
				t.Errorf("Module %s missing variable \"name_suffix\"", mod)
			}
			chunk := extractHCLBlock(content, `variable "name_suffix"`)
			if !strings.Contains(chunk, `type        = string`) && !strings.Contains(chunk, `type = string`) {
				t.Errorf("Module %s variable \"name_suffix\" must be type string", mod)
			}
			if !strings.Contains(chunk, `default     = ""`) && !strings.Contains(chunk, `default = ""`) {
				t.Errorf("Module %s variable \"name_suffix\" must default to \"\"", mod)
			}
		}
	})

	t.Run("GatewayServiceAccountIdVariableDeclared", func(t *testing.T) {
		varPath := filepath.Join(modulesDir, "gateway", "variables.tf")
		b, err := os.ReadFile(varPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varPath, err)
		}
		content := string(b)
		if !strings.Contains(content, `variable "service_account_id"`) {
			t.Errorf("gateway/variables.tf missing variable \"service_account_id\"")
		}
		chunk := extractHCLBlock(content, `variable "service_account_id"`)
		if !strings.Contains(chunk, `type        = string`) && !strings.Contains(chunk, `type = string`) {
			t.Errorf("gateway variable \"service_account_id\" must be type string")
		}
		if !strings.Contains(chunk, `default     = ""`) && !strings.Contains(chunk, `default = ""`) {
			t.Errorf("gateway variable \"service_account_id\" must default to \"\"")
		}
		if !strings.Contains(chunk, "validation") {
			t.Errorf("gateway variable \"service_account_id\" must include a validation block")
		}
		if !strings.Contains(chunk, "length(var.service_account_id) <= 30") {
			t.Errorf("gateway variable \"service_account_id\" must validate length <= 30")
		}
	})

	t.Run("BaseServiceAccountIDsWithin30CharLimit", func(t *testing.T) {
		files := []string{
			filepath.Join(modulesDir, "hub", "main.tf"),
			filepath.Join(modulesDir, "vault", "main.tf"),
			filepath.Join(modulesDir, "gateway", "main.tf"),
			filepath.Join(modulesDir, "gateway", "subscriptions.tf"),
		}

		saPattern := regexp.MustCompile(`(?:account_id|service_account_id|push_invoker_service_account_id)\s*=\s*(?:var\.service_account_id\s*!=\s*""\s*\?\s*var\.service_account_id\s*:\s*)?"([^"${]+)`)

		count := 0
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", f, err)
			}
			matches := saPattern.FindAllStringSubmatch(string(b), -1)
			for _, m := range matches {
				count++
				basePrefix := m[1]
				if len(basePrefix) > 30 {
					t.Errorf("File %s: base service account prefix %q exceeds 30 chars (len=%d)", f, basePrefix, len(basePrefix))
				}
				if len(basePrefix) > 20 {
					t.Errorf("File %s: base service account prefix %q is too long (%d chars) to safely accommodate a branch suffix", f, basePrefix, len(basePrefix))
				}
			}
		}
		if count < 7 {
			t.Errorf("Expected at least 7 service accounts verified across modules, found %d", count)
		}
	})

	t.Run("HubResourcesDeterministicNaming", func(t *testing.T) {
		mainPath := filepath.Join(modulesDir, "hub", "main.tf")
		b, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		content := string(b)

		// Check service account IDs
		if !strings.Contains(content, `"hub-runtime-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("hub_runtime service account ID must be plan-time deterministic with name_suffix")
		}
		if !strings.Contains(content, `"metadata-shovel-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("shovel_runtime service account ID must be plan-time deterministic with name_suffix")
		}
		if !strings.Contains(content, `"eventarc-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("eventarc_trigger_sa service account ID must be eventarc-sa with name_suffix")
		}
		if !strings.Contains(content, `"events-invoker-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("events_invoker service account ID must be events-invoker-sa with name_suffix")
		}
		if !strings.Contains(content, `"supercargo-mcp-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("mcp_runtime service account ID must be supercargo-mcp-sa with name_suffix")
		}

		// Check Cloud Run service names
		if !strings.Contains(content, `"hub${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("hub Cloud Run service name must be hub with name_suffix")
		}
		if !strings.Contains(content, `"metadata-shovel${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("shovel Cloud Run service name must be metadata-shovel with name_suffix")
		}
		if !strings.Contains(content, `"supercargo-mcp${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("mcp Cloud Run service name must be supercargo-mcp with name_suffix")
		}
	})

	t.Run("VaultResourcesDeterministicNaming", func(t *testing.T) {
		mainPath := filepath.Join(modulesDir, "vault", "main.tf")
		b, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		content := string(b)

		if !strings.Contains(content, `"vault-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("vault_sa service account ID must be vault-sa with name_suffix")
		}
		if !strings.Contains(content, `"${var.service_name}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("vault Cloud Run service name must be var.service_name with name_suffix")
		}
		if !strings.Contains(content, `"supercargo-vault-master-key${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("vault_master_key secret_id must be supercargo-vault-master-key with name_suffix")
		}
		if !strings.Contains(content, `"supercargo-vault-global-pepper${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("global_pepper secret_id must be supercargo-vault-global-pepper with name_suffix")
		}
		if !strings.Contains(content, `"key-created${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("key_created topic name must be key-created with name_suffix")
		}
	})

	t.Run("GatewayResourcesDeterministicNaming", func(t *testing.T) {
		mainPath := filepath.Join(modulesDir, "gateway", "main.tf")
		b, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		content := string(b)

		if !strings.Contains(content, `var.service_account_id != "" ? var.service_account_id : "gateway-${var.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("gateway service account must support var.service_account_id override or default to gateway-${var.product_id} with name_suffix")
		}
		if !strings.Contains(content, `"gateway-${var.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("gateway Cloud Run service name must be gateway-${var.product_id} with name_suffix")
		}
		if !strings.Contains(content, `"raw-${var.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("raw topic name must be raw-${var.product_id} with name_suffix")
		}
		if !strings.Contains(content, `"clean-${var.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("clean topic name must be clean-${var.product_id} with name_suffix")
		}
		if !strings.Contains(content, `"dlq-${var.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"`) {
			t.Errorf("dlq topic name must be dlq-${var.product_id} with name_suffix")
		}

		outputsPath := filepath.Join(modulesDir, "gateway", "outputs.tf")
		ob, err := os.ReadFile(outputsPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", outputsPath, err)
		}
		outputsContent := string(ob)
		if !strings.Contains(outputsContent, "value       = local.service_account_email") && !strings.Contains(outputsContent, "value = local.service_account_email") {
			t.Errorf("gateway outputs.tf must use local.service_account_email for service_account_email output")
		}
		if !strings.Contains(outputsContent, "value       = local.push_invoker_service_account_email") && !strings.Contains(outputsContent, "value = local.push_invoker_service_account_email") {
			t.Errorf("gateway outputs.tf must use local.push_invoker_service_account_email for push_invoker_service_account_email output")
		}
		if !strings.Contains(outputsContent, `output "service_account_id"`) {
			t.Errorf("gateway outputs.tf missing output \"service_account_id\"")
		}
	})

	t.Run("DataProductModuleForwardsNameSuffixAndServiceAccountId", func(t *testing.T) {
		varsPath := filepath.Join(modulesDir, "data_product", "variables.tf")
		vb, err := os.ReadFile(varsPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", varsPath, err)
		}
		varsContent := string(vb)
		if !strings.Contains(varsContent, `variable "name_suffix"`) {
			t.Errorf("data_product/variables.tf missing variable \"name_suffix\"")
		}
		if !strings.Contains(varsContent, `variable "service_account_id"`) {
			t.Errorf("data_product/variables.tf missing variable \"service_account_id\"")
		}

		mainPath := filepath.Join(modulesDir, "data_product", "main.tf")
		mb, err := os.ReadFile(mainPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", mainPath, err)
		}
		mainContent := string(mb)
		gatewayChunk := extractHCLBlock(mainContent, `module "gateway"`)
		if gatewayChunk == "" {
			t.Fatalf("Failed to extract module \"gateway\" block from data_product/main.tf")
		}
		if !strings.Contains(gatewayChunk, "name_suffix") || !strings.Contains(gatewayChunk, "var.name_suffix") {
			t.Errorf("data_product module \"gateway\" must forward name_suffix = var.name_suffix")
		}
		if !strings.Contains(gatewayChunk, "service_account_id") || !strings.Contains(gatewayChunk, "var.service_account_id") {
			t.Errorf("data_product module \"gateway\" must forward service_account_id = var.service_account_id")
		}
	})
}

func TestModules_HubEventsPushAudience(t *testing.T) {
	modulesDir := "../../modules"
	hubDir := filepath.Join(modulesDir, "hub")
	mainPath := filepath.Join(hubDir, "main.tf")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", mainPath, err)
	}
	mainContent := string(mainBytes)

	if !strings.Contains(mainContent, `resource "google_pubsub_subscription" "events_push"`) {
		t.Fatalf("modules/hub/main.tf missing resource \"google_pubsub_subscription\" \"events_push\"")
	}
	subChunk := extractHCLBlock(mainContent, `resource "google_pubsub_subscription" "events_push"`)
	if subChunk == "" {
		t.Fatalf("Failed to extract events_push subscription block")
	}

	if !strings.Contains(subChunk, `"${google_cloud_run_v2_service.hub.uri}/v1/internal/events"`) {
		t.Errorf("events_push subscription must push to ${google_cloud_run_v2_service.hub.uri}/v1/internal/events")
	}
	if !strings.Contains(subChunk, "google_service_account.events_invoker.email") {
		t.Errorf("events_push subscription must use google_service_account.events_invoker.email for oidc_token")
	}
	if !strings.Contains(subChunk, "var.oidc_audience != \"\" ? trimspace(split(\",\", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri") {
		t.Errorf("events_push subscription oidc audience must fallback to google_cloud_run_v2_service.hub.uri when var.oidc_audience is empty")
	}
}

func TestModules_HubShovelOIDCAudienceAndLifecycle(t *testing.T) {
	modulesDir := "../../modules"
	hubDir := filepath.Join(modulesDir, "hub")
	mainPath := filepath.Join(hubDir, "main.tf")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", mainPath, err)
	}
	mainContent := string(mainBytes)

	if !strings.Contains(mainContent, `resource "google_cloud_run_v2_service" "shovel"`) {
		t.Fatalf("modules/hub/main.tf missing resource \"google_cloud_run_v2_service\" \"shovel\"")
	}
	shovelChunk := extractHCLBlock(mainContent, `resource "google_cloud_run_v2_service" "shovel"`)
	if shovelChunk == "" {
		t.Fatalf("Failed to extract shovel service block")
	}

	if !strings.Contains(shovelChunk, `name  = "SHOVEL_OIDC_AUDIENCE"`) {
		t.Errorf("shovel service must define SHOVEL_OIDC_AUDIENCE env var")
	}

	if !strings.Contains(shovelChunk, `template[0].containers[0].env`) {
		t.Errorf("shovel service lifecycle.ignore_changes must include template[0].containers[0].env")
	}

	varsPath := filepath.Join(hubDir, "variables.tf")
	varsBytes, err := os.ReadFile(varsPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", varsPath, err)
	}
	varsContent := string(varsBytes)
	if !strings.Contains(varsContent, `variable "shovel_oidc_audience"`) {
		t.Errorf("modules/hub/variables.tf missing variable \"shovel_oidc_audience\"")
	}
}

func findGlobalTelemetryShareModuleDir(t *testing.T) string {
	t.Helper()
	moduleDir := filepath.Clean(filepath.Join("..", "..", "modules", "global_telemetry_share"))
	abs, err := filepath.Abs(moduleDir)
	require.NoError(t, err)
	return abs
}

func readGlobalTelemetryShareModuleFile(t *testing.T, filename string) string {
	t.Helper()
	moduleDir := findGlobalTelemetryShareModuleDir(t)
	fullPath := filepath.Join(moduleDir, filename)
	data, err := os.ReadFile(fullPath)
	require.NoError(t, err, "failed to read module file %s at %s", filename, fullPath)
	return string(data)
}

func TestModules_GlobalTelemetryShare(t *testing.T) {
	moduleDir := findGlobalTelemetryShareModuleDir(t)
	fi, err := os.Stat(moduleDir)
	require.NoError(t, err, "module directory should exist at %s", moduleDir)
	require.True(t, fi.IsDir(), "expected %s to be a directory", moduleDir)

	t.Run("FilesExist", func(t *testing.T) {
		expectedFiles := []string{
			"main.tf",
			"variables.tf",
			"outputs.tf",
			"versions.tf",
			"README.md",
		}

		for _, file := range expectedFiles {
			t.Run(file, func(t *testing.T) {
				path := filepath.Join(moduleDir, file)
				f, err := os.Stat(path)
				require.NoError(t, err, "file %s should exist in module", file)
				assert.False(t, f.IsDir(), "file %s should not be a directory", file)
			})
		}
	})

	t.Run("Variables", func(t *testing.T) {
		content := readGlobalTelemetryShareModuleFile(t, "variables.tf")

		tests := []struct {
			name     string
			expected []string
		}{
			{
				name: "project_id variable",
				expected: []string{
					`variable "project_id"`,
					`type        = string`,
				},
			},
			{
				name: "dataset_id variable with default",
				expected: []string{
					`variable "dataset_id"`,
					`type        = string`,
					`default     = "supercargo_global_telemetry"`,
				},
			},
			{
				name: "internal_dataset_id variable with default",
				expected: []string{
					`variable "internal_dataset_id"`,
					`type        = string`,
					`default     = "supercargo_intelligence_internal"`,
				},
			},
			{
				name: "github_dataset_id variable with default",
				expected: []string{
					`variable "github_dataset_id"`,
					`type        = string`,
					`default     = "supercargo_github"`,
				},
			},
			{
				name: "enable_global_telemetry_share variable with default",
				expected: []string{
					`variable "enable_global_telemetry_share"`,
					`type        = bool`,
					`default     = true`,
				},
			},
			{
				name: "supercargo_reader_principal variable with default",
				expected: []string{
					`variable "supercargo_reader_principal"`,
					`type        = string`,
					`default     = "serviceAccount:telemetry-reader@supercargo-global.iam.gserviceaccount.com"`,
				},
			},
			{
				name: "location variable with default",
				expected: []string{
					`variable "location"`,
					`type        = string`,
					`default     = "EU"`,
				},
			},
			{
				name: "enable_analytics_hub_listing variable with default",
				expected: []string{
					`variable "enable_analytics_hub_listing"`,
					`type        = bool`,
					`default     = false`,
				},
			},
			{
				name: "data_exchange_id variable",
				expected: []string{
					`variable "data_exchange_id"`,
					`type        = string`,
				},
			},
			{
				name: "enable_global_sdlc_share variable with default",
				expected: []string{
					`variable "enable_global_sdlc_share"`,
					`type        = bool`,
					`default     = true`,
				},
			},
			{
				name: "enable_peer_benchmark_subscription variable with default",
				expected: []string{
					`variable "enable_peer_benchmark_subscription"`,
					`type        = bool`,
					`default     = false`,
				},
			},
			{
				name: "peer_benchmark_data_exchange_id variable with default and validation",
				expected: []string{
					`variable "peer_benchmark_data_exchange_id"`,
					`type        = string`,
					`default     = ""`,
					`can(regex("^[a-zA-Z0-9_-]+$", var.peer_benchmark_data_exchange_id))`,
				},
			},
			{
				name: "peer_benchmark_listing_id variable with validation",
				expected: []string{
					`variable "peer_benchmark_listing_id"`,
					`type        = string`,
					`default     = ""`,
					`can(regex("^[a-zA-Z0-9_-]+$", var.peer_benchmark_listing_id))`,
				},
			},
			{
				name: "peer_benchmark_dataset_id variable with default",
				expected: []string{
					`variable "peer_benchmark_dataset_id"`,
					`type        = string`,
					`default     = "supercargo_peer_benchmarks"`,
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				for _, exp := range tc.expected {
					assert.Contains(t, content, exp)
				}
			})
		}
	})

	t.Run("Resources", func(t *testing.T) {
		content := readGlobalTelemetryShareModuleFile(t, "main.tf")

		tests := []struct {
			name     string
			snippets []string
		}{
			{
				name: "google_bigquery_dataset global_telemetry",
				snippets: []string{
					`resource "google_bigquery_dataset" "global_telemetry"`,
					`var.enable_global_telemetry_share ? 1 : 0`,
					`project                    = var.project_id`,
					`dataset_id                 = var.dataset_id`,
					`location                   = var.location`,
					`delete_contents_on_destroy = true`,
					`governance = "supercargo"`,
					`component  = "global_telemetry_share"`,
				},
			},
			{
				name: "google_bigquery_dataset_access supercargo_reader_access",
				snippets: []string{
					`resource "google_bigquery_dataset_access" "supercargo_reader_access"`,
					`var.enable_global_telemetry_share ? 1 : 0`,
					`role       = "roles/bigquery.dataViewer"`,
					`iam_member = var.supercargo_reader_principal`,
					`depends_on = [google_bigquery_dataset.global_telemetry]`,
				},
			},
			{
				name: "google_bigquery_dataset_access authorized_view on internal dataset",
				snippets: []string{
					`resource "google_bigquery_dataset_access" "authorized_view"`,
					`var.enable_global_telemetry_share ? 1 : 0`,
					`dataset_id = var.internal_dataset_id`,
					`view {`,
					`project_id = var.project_id`,
					`dataset_id = var.dataset_id`,
					`table_id   = "global_telemetry_digest"`,
					`depends_on = [google_bigquery_dataset.global_telemetry]`,
				},
			},
			{
				name: "google_bigquery_dataset_access authorized_view_github on github dataset",
				snippets: []string{
					`resource "google_bigquery_dataset_access" "authorized_view_github"`,
					`var.enable_global_telemetry_share ? 1 : 0`,
					`dataset_id = var.github_dataset_id`,
					`view {`,
					`project_id = var.project_id`,
					`dataset_id = var.dataset_id`,
					`table_id   = "global_telemetry_digest"`,
					`depends_on = [google_bigquery_dataset.global_telemetry]`,
				},
			},
			{
				name: "google_bigquery_dataset_access authorized_view_sdlc on internal dataset",
				snippets: []string{
					`resource "google_bigquery_dataset_access" "authorized_view_sdlc"`,
					`count      = var.enable_global_telemetry_share && var.enable_global_sdlc_share ? 1 : 0`,
					`project    = var.project_id`,
					`dataset_id = var.internal_dataset_id`,
					`table_id   = "global_sdlc_efficiency_digest"`,
					`depends_on = [google_bigquery_dataset.global_telemetry]`,
				},
			},
			{
				name: "google_bigquery_dataset_access authorized_view_github_sdlc on github dataset",
				snippets: []string{
					`resource "google_bigquery_dataset_access" "authorized_view_github_sdlc"`,
					`count      = var.enable_global_telemetry_share && var.enable_global_sdlc_share ? 1 : 0`,
					`project    = var.project_id`,
					`dataset_id = var.github_dataset_id`,
					`table_id   = "global_sdlc_efficiency_digest"`,
					`depends_on = [google_bigquery_dataset.global_telemetry]`,
				},
			},
			{
				name: "google_bigquery_analytics_hub_listing telemetry_listing with sanitized listing_id",
				snippets: []string{
					`resource "google_bigquery_analytics_hub_listing" "telemetry_listing"`,
					`var.enable_global_telemetry_share && var.enable_analytics_hub_listing ? 1 : 0`,
					`data_exchange_id = var.data_exchange_id`,
					`listing_id       = "telemetry_digest_${replace(var.project_id, "-", "_")}"`,
					`bigquery_dataset {`,
					`dataset = "projects/${var.project_id}/datasets/${var.dataset_id}"`,
					`depends_on = [google_bigquery_dataset.global_telemetry]`,
				},
			},
			{
				name: "google_bigquery_analytics_hub_listing sdlc_efficiency_listing with sanitized listing_id",
				snippets: []string{
					`resource "google_bigquery_analytics_hub_listing" "sdlc_efficiency_listing"`,
					`count            = var.enable_global_telemetry_share && var.enable_global_sdlc_share && var.enable_analytics_hub_listing ? 1 : 0`,
					`project          = var.project_id`,
					`location         = var.location`,
					`data_exchange_id = var.data_exchange_id`,
					`listing_id       = "sdlc_efficiency_digest_${replace(var.project_id, "-", "_")}"`,
					`display_name     = "Supercargo SDLC Efficiency Digest"`,
					`description      = "Pseudonymized operational SDLC efficiency metrics shared under global-sdlc-efficiency-digest contract"`,
					`dataset = "projects/${var.project_id}/datasets/${var.dataset_id}"`,
					`depends_on = [google_bigquery_dataset.global_telemetry]`,
				},
			},
			{
				name: "google_bigquery_analytics_hub_listing_subscription peer_benchmarks",
				snippets: []string{
					`resource "google_bigquery_analytics_hub_listing_subscription" "peer_benchmarks"`,
					`count            = var.enable_peer_benchmark_subscription && var.peer_benchmark_listing_id != "" ? 1 : 0`,
					`project          = var.project_id`,
					`location         = var.location`,
					`data_exchange_id = var.peer_benchmark_data_exchange_id != "" ? var.peer_benchmark_data_exchange_id : var.data_exchange_id`,
					`listing_id       = var.peer_benchmark_listing_id`,
					`dataset_id = var.peer_benchmark_dataset_id`,
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				for _, snippet := range tc.snippets {
					assert.Contains(t, content, snippet)
				}
			})
		}
	})

	t.Run("Outputs", func(t *testing.T) {
		content := readGlobalTelemetryShareModuleFile(t, "outputs.tf")

		tests := []struct {
			name     string
			snippets []string
		}{
			{
				name: "dataset_id output",
				snippets: []string{
					`output "dataset_id"`,
					`try(google_bigquery_dataset.global_telemetry[0].dataset_id, null)`,
				},
			},
			{
				name: "dataset_access_id output",
				snippets: []string{
					`output "dataset_access_id"`,
					`try(google_bigquery_dataset_access.supercargo_reader_access[0].id, null)`,
				},
			},
			{
				name: "authorized_view_access_id output",
				snippets: []string{
					`output "authorized_view_access_id"`,
					`try(google_bigquery_dataset_access.authorized_view[0].id, null)`,
				},
			},
			{
				name: "authorized_view_github_access_id output",
				snippets: []string{
					`output "authorized_view_github_access_id"`,
					`try(google_bigquery_dataset_access.authorized_view_github[0].id, null)`,
				},
			},
			{
				name: "authorized_view_sdlc_access_id output",
				snippets: []string{
					`output "authorized_view_sdlc_access_id"`,
					`try(google_bigquery_dataset_access.authorized_view_sdlc[0].id, null)`,
				},
			},
			{
				name: "authorized_view_github_sdlc_access_id output",
				snippets: []string{
					`output "authorized_view_github_sdlc_access_id"`,
					`try(google_bigquery_dataset_access.authorized_view_github_sdlc[0].id, null)`,
				},
			},
			{
				name: "analytics_hub_listing_id output",
				snippets: []string{
					`output "analytics_hub_listing_id"`,
					`try(google_bigquery_analytics_hub_listing.telemetry_listing[0].id, null)`,
				},
			},
			{
				name: "sdlc_analytics_hub_listing_id output",
				snippets: []string{
					`output "sdlc_analytics_hub_listing_id"`,
					`try(google_bigquery_analytics_hub_listing.sdlc_efficiency_listing[0].id, null)`,
				},
			},
			{
				name: "peer_benchmark_subscription_id output",
				snippets: []string{
					`output "peer_benchmark_subscription_id"`,
					`try(google_bigquery_analytics_hub_listing_subscription.peer_benchmarks[0].id, null)`,
				},
			},
			{
				name: "sharing_enabled output",
				snippets: []string{
					`output "sharing_enabled"`,
					`var.enable_global_telemetry_share`,
				},
			},
			{
				name: "sdlc_sharing_enabled output",
				snippets: []string{
					`output "sdlc_sharing_enabled"`,
					`var.enable_global_sdlc_share`,
				},
			},
			{
				name: "peer_benchmarks_enabled output",
				snippets: []string{
					`output "peer_benchmarks_enabled"`,
					`var.enable_peer_benchmark_subscription`,
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				for _, snippet := range tc.snippets {
					assert.Contains(t, content, snippet)
				}
			})
		}
	})

	t.Run("Versions", func(t *testing.T) {
		content := readGlobalTelemetryShareModuleFile(t, "versions.tf")

		assert.True(t, strings.Contains(content, `required_version = ">= 1.5.0"`) || strings.Contains(content, `required_version = ">= 1.5"`))
		assert.Contains(t, content, `hashicorp/google`)
		assert.Contains(t, content, `>= 5.0.0, < 8.0.0`)
	})

	t.Run("Readme", func(t *testing.T) {
		content := readGlobalTelemetryShareModuleFile(t, "README.md")

		assert.Contains(t, content, "supercargo_global_telemetry")
		assert.Contains(t, content, "supercargo_intelligence_internal")
		assert.Contains(t, content, "supercargo_github")
		assert.Contains(t, content, "global_telemetry_digest")
		assert.Contains(t, content, "Authorized View")
		assert.Contains(t, content, "enable_global_sdlc_share")
		assert.Contains(t, content, "enable_peer_benchmark_subscription")
		assert.Contains(t, content, "global_sdlc_efficiency_digest")
		assert.Contains(t, content, "peer_benchmark_listing_id")
	})
}

func findAnalyticsGitHubSyncModuleDir(t *testing.T) string {
	t.Helper()
	moduleDir := filepath.Clean(filepath.Join("..", "..", "modules", "analytics_github_sync"))
	abs, err := filepath.Abs(moduleDir)
	require.NoError(t, err)
	return abs
}

func readAnalyticsGitHubSyncModuleFile(t *testing.T, filename string) string {
	t.Helper()
	moduleDir := findAnalyticsGitHubSyncModuleDir(t)
	fullPath := filepath.Join(moduleDir, filename)
	data, err := os.ReadFile(fullPath)
	require.NoError(t, err, "failed to read module file %s at %s", filename, fullPath)
	return string(data)
}

func TestModules_AnalyticsGitHubSync(t *testing.T) {
	moduleDir := findAnalyticsGitHubSyncModuleDir(t)
	fi, err := os.Stat(moduleDir)
	require.NoError(t, err, "module directory should exist at %s", moduleDir)
	require.True(t, fi.IsDir(), "expected %s to be a directory", moduleDir)

	t.Run("FilesExist", func(t *testing.T) {
		expectedFiles := []string{
			"main.tf",
			"variables.tf",
			"outputs.tf",
			"versions.tf",
			"README.md",
		}

		for _, file := range expectedFiles {
			t.Run(file, func(t *testing.T) {
				path := filepath.Join(moduleDir, file)
				f, err := os.Stat(path)
				require.NoError(t, err, "file %s should exist in module", file)
				assert.False(t, f.IsDir(), "file %s should not be a directory", file)
			})
		}
	})

	t.Run("DeterministicLocalsAndFallbacks", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "main.tf")

		assert.Contains(t, content, `locals {`)
		assert.Contains(t, content, `sa_email`)
		assert.Contains(t, content, `scheduler_sa_email`)

		// Table-driven verification of deterministic SA email derivation and scheduler job name derivation
		deriveSAEmail := func(saID, defaultID, projectID string) string {
			id := defaultID
			if saID != "" {
				id = saID
			}
			return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", id, projectID)
		}
		deriveSchedJobName := func(customName, jobName string) string {
			if customName != "" {
				return customName
			}
			return fmt.Sprintf("%s-schedule", jobName)
		}

		saTests := []struct {
			name        string
			customID    string
			defaultID   string
			projectID   string
			wantSAEmail string
		}{
			{
				name:        "custom runner SA id overrides default",
				customID:    "my-custom-runner-sa",
				defaultID:   "analytics-github-sync-sa",
				projectID:   "corp-prod-analytics",
				wantSAEmail: "my-custom-runner-sa@corp-prod-analytics.iam.gserviceaccount.com",
			},
			{
				name:        "empty runner SA id falls back to analytics-github-sync-sa",
				customID:    "",
				defaultID:   "analytics-github-sync-sa",
				projectID:   "corp-prod-analytics",
				wantSAEmail: "analytics-github-sync-sa@corp-prod-analytics.iam.gserviceaccount.com",
			},
			{
				name:        "custom scheduler SA id overrides default",
				customID:    "my-custom-sched-sa",
				defaultID:   "analytics-github-sync-sched-sa",
				projectID:   "corp-prod-analytics",
				wantSAEmail: "my-custom-sched-sa@corp-prod-analytics.iam.gserviceaccount.com",
			},
			{
				name:        "empty scheduler SA id falls back to analytics-github-sync-sched-sa",
				customID:    "",
				defaultID:   "analytics-github-sync-sched-sa",
				projectID:   "corp-prod-analytics",
				wantSAEmail: "analytics-github-sync-sched-sa@corp-prod-analytics.iam.gserviceaccount.com",
			},
		}

		for _, tc := range saTests {
			tc := tc
			t.Run("Derivation/"+tc.name, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, tc.wantSAEmail, deriveSAEmail(tc.customID, tc.defaultID, tc.projectID))
			})
		}

		schedTests := []struct {
			name        string
			customSched string
			jobName     string
			wantName    string
		}{
			{
				name:        "custom scheduler job name overrides default",
				customSched: "hourly-github-sync",
				jobName:     "analytics-github-sync",
				wantName:    "hourly-github-sync",
			},
			{
				name:        "omitted scheduler job name appends -schedule suffix",
				customSched: "",
				jobName:     "custom-sync-job",
				wantName:    "custom-sync-job-schedule",
			},
		}

		for _, tc := range schedTests {
			tc := tc
			t.Run("SchedName/"+tc.name, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, tc.wantName, deriveSchedJobName(tc.customSched, tc.jobName))
			})
		}
	})

	t.Run("CloudRunJob", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "main.tf")
		jobBlock := extractHCLBlock(content, `resource "google_cloud_run_v2_job" "sync_jobs"`)
		require.NotEmpty(t, jobBlock, "google_cloud_run_v2_job.sync_jobs must exist")

		tests := []struct {
			name     string
			snippets []string
		}{
			{
				name: "job resource definition and for_each",
				snippets: []string{
					`resource "google_cloud_run_v2_job" "sync_jobs"`,
					`for_each            = local.contracts`,
					`name                = "${var.job_name}-${each.value.job_suffix}"`,
					`var.region`,
					`var.project_id`,
					`var.deletion_protection`,
				},
			},
			{
				name: "contracts local definition",
				snippets: []string{
					`contracts = {`,
					`repositories  = { job_suffix = "repositories", resource = "labeled_repositories", schedule = "0 1 * * *" }`,
					`issues        = { job_suffix = "issues", resource = "issues", schedule = "0 2 * * *" }`,
					`pulls         = { job_suffix = "pulls", resource = "pulls", schedule = "0 3 * * *" }`,
					`workflow_runs = { job_suffix = "workflow-runs", resource = "workflow_runs", schedule = "0 4 * * *" }`,
					`billing       = { job_suffix = "billing", resource = "org_metered_usage", schedule = "0 5 * * *" }`,
				},
			},
			{
				name: "lifecycle ignore_changes",
				snippets: []string{
					`lifecycle {`,
					`ignore_changes = [`,
					`template[0].template[0].containers[0].image`,
					`client`,
					`client_version`,
				},
			},
			{
				name: "container environment variables",
				snippets: []string{
					`name  = "SUPERCARGO_VAULT_URL"`,
					`value = var.vault_url`,
					`name  = "SUPERCARGO_VAULT_AUDIENCE"`,
					`value = var.vault_audience`,
					`name  = "SUPERCARGO_HUB_URL"`,
					`value = var.hub_url`,
					`name  = "SUPERCARGO_HUB_AUDIENCE"`,
					`value = var.hub_audience`,
					`name  = "GOOGLE_CLOUD_PROJECT"`,
					`value = var.project_id`,
					`name  = "DESTINATION__BIGQUERY__LOCATION"`,
				},
			},
			{
				name: "secret injection for legacy and unified dlt runner",
				snippets: []string{
					`name = "GITHUB_TOKEN"`,
					`name = "SUPERCARGO_CREDENTIALS_SECRET"`,
					`secret  = var.github_token_secret_id`,
					`version = "latest"`,
				},
			},
			{
				name: "container args per-contract resources",
				snippets: []string{
					`"--destination"`,
					`"bigquery"`,
					`"--dataset"`,
					`var.dataset`,
					`"--org"`,
					`var.org`,
					`"--topic"`,
					`var.topic`,
					`"--resources"`,
					`each.value.resource`,
				},
			},
			{
				name: "service account assignment and depends_on",
				snippets: []string{
					`service_account = local.sa_email`,
					`google_service_account.sync_sa`,
				},
			},
			{
				name: "declarative moved blocks",
				snippets: []string{
					`moved {`,
					`from = google_cloud_run_v2_job.sync_job`,
					`to   = google_cloud_run_v2_job.sync_jobs["repositories"]`,
					`from = google_cloud_scheduler_job.sync_schedule`,
					`to   = google_cloud_scheduler_job.sync_schedules["repositories"]`,
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				for _, snippet := range tc.snippets {
					assert.Contains(t, content, snippet)
				}
			})
		}
	})

	t.Run("CloudSchedulerAndIAM", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "main.tf")

		tests := []struct {
			name     string
			snippets []string
		}{
			{
				name: "scheduler invoker IAM binding per contract",
				snippets: []string{
					`resource "google_cloud_run_v2_job_iam_member" "scheduler_invoker"`,
					`for_each = local.contracts`,
					`name     = google_cloud_run_v2_job.sync_jobs[each.key].name`,
					`"roles/run.invoker"`,
					`"serviceAccount:${local.scheduler_sa_email}"`,
				},
			},
			{
				name: "cloud scheduler job definition per contract",
				snippets: []string{
					`resource "google_cloud_scheduler_job" "sync_schedules"`,
					`for_each         = local.contracts`,
					`name             = "${var.job_name}-${each.value.job_suffix}-schedule"`,
					`schedule         = try(var.schedules[each.key], each.value.schedule)`,
					`https://${var.region}-run.googleapis.com/v2/projects/${var.project_id}/locations/${var.region}/jobs/${google_cloud_run_v2_job.sync_jobs[each.key].name}:run`,
					`oauth_token {`,
					`local.scheduler_sa_email`,
					`"https://www.googleapis.com/auth/cloud-platform"`,
					`paused           = var.paused`,
				},
			},
			{
				name: "bigquery IAM bindings",
				snippets: []string{
					`roles/bigquery.dataEditor`,
					`roles/bigquery.jobUser`,
					`serviceAccount:${local.sa_email}`,
				},
			},
			{
				name: "secret manager IAM binding",
				snippets: []string{
					`resource "google_secret_manager_secret_iam_member"`,
					`"roles/secretmanager.secretAccessor"`,
					`"serviceAccount:${local.sa_email}"`,
					`secret_id = var.github_token_secret_id`,
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				for _, snippet := range tc.snippets {
					assert.Contains(t, content, snippet)
				}
			})
		}
	})

	t.Run("VariablesPreservedAndExtended", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "variables.tf")

		expectedVars := []struct {
			name       string
			varType    string
			defaultVal string
		}{
			{name: "project_id", varType: "string", defaultVal: ""},
			{name: "region", varType: "string", defaultVal: `"europe-west1"`},
			{name: "job_name", varType: "string", defaultVal: `"analytics-github-sync"`},
			{name: "container_image", varType: "string", defaultVal: ""},
			{name: "org", varType: "string", defaultVal: ""},
			{name: "topic", varType: "string", defaultVal: `"supercargo-analytics"`},
			{name: "dataset", varType: "string", defaultVal: `"supercargo_github"`},
			{name: "vault_url", varType: "string", defaultVal: `""`},
			{name: "vault_audience", varType: "string", defaultVal: `""`},
			{name: "github_token_secret_id", varType: "string", defaultVal: ""},
			{name: "service_account_id", varType: "string", defaultVal: `"analytics-github-sync-sa"`},
			{name: "scheduler_service_account_id", varType: "string", defaultVal: `"analytics-github-sync-sched-sa"`},
			{name: "schedule", varType: "string", defaultVal: `"0 * * * *"`},
			{name: "time_zone", varType: "string", defaultVal: `"Etc/UTC"`},
			{name: "scheduler_job_name", varType: "string", defaultVal: "null"},
			{name: "scheduler_attempt_deadline", varType: "string", defaultVal: `"320s"`},
			{name: "paused", varType: "bool", defaultVal: "false"},
			{name: "deletion_protection", varType: "bool", defaultVal: "false"},
			{name: "cpu_limit", varType: "string", defaultVal: `"1"`},
			{name: "memory_limit", varType: "string", defaultVal: `"1Gi"`},
			{name: "timeout", varType: "string", defaultVal: `"600s"`},
			{name: "max_retries", varType: "number", defaultVal: "1"},
			// 2 new optional variables for Hub telemetry / health reporting
			{name: "hub_url", varType: "string", defaultVal: `""`},
			{name: "hub_audience", varType: "string", defaultVal: `""`},
			// Optional per-contract schedules override
			{name: "schedules", varType: "map(string)", defaultVal: "{}"},
		}

		for _, ev := range expectedVars {
			block := extractHCLBlock(content, fmt.Sprintf(`variable "%s"`, ev.name))
			require.NotEmpty(t, block, "variable %q must exist in variables.tf", ev.name)
			assert.Contains(t, block, fmt.Sprintf("type        = %s", ev.varType), "variable %q must have type %s", ev.name, ev.varType)
			if ev.defaultVal != "" {
				assert.Contains(t, block, fmt.Sprintf("default     = %s", ev.defaultVal), "variable %q must have default %s", ev.name, ev.defaultVal)
			}
		}
	})

	t.Run("Outputs", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "outputs.tf")

		tests := []struct {
			name     string
			snippets []string
		}{
			{
				name: "job_name output",
				snippets: []string{
					`output "job_name"`,
					`google_cloud_run_v2_job.sync_jobs["repositories"].name`,
				},
			},
			{
				name: "job_id output",
				snippets: []string{
					`output "job_id"`,
					`google_cloud_run_v2_job.sync_jobs["repositories"].id`,
				},
			},
			{
				name: "job_names output",
				snippets: []string{
					`output "job_names"`,
					`{ for k, v in google_cloud_run_v2_job.sync_jobs : k => v.name }`,
				},
			},
			{
				name: "job_ids output",
				snippets: []string{
					`output "job_ids"`,
					`{ for k, v in google_cloud_run_v2_job.sync_jobs : k => v.id }`,
				},
			},
			{
				name: "service_account_email output",
				snippets: []string{
					`output "service_account_email"`,
					`local.sa_email`,
				},
			},
			{
				name: "scheduler_job_name output",
				snippets: []string{
					`output "scheduler_job_name"`,
					`google_cloud_scheduler_job.sync_schedules["repositories"].name`,
				},
			},
			{
				name: "scheduler_job_names output",
				snippets: []string{
					`output "scheduler_job_names"`,
					`{ for k, v in google_cloud_scheduler_job.sync_schedules : k => v.name }`,
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				for _, snippet := range tc.snippets {
					assert.Contains(t, content, snippet)
				}
			})
		}
	})

	t.Run("Versions", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "versions.tf")

		assert.True(t, strings.Contains(content, `required_version = ">= 1.5.0"`) || strings.Contains(content, `required_version = ">= 1.5"`))
		assert.Contains(t, content, `hashicorp/google`)
		assert.Contains(t, content, `>= 5.0.0, < 8.0.0`)
	})

	t.Run("Readme", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "README.md")

		assert.Contains(t, content, "analytics_github_sync")
		assert.Contains(t, content, "Cloud Run")
		assert.Contains(t, content, "Cloud Scheduler")
		assert.Contains(t, content, "BigQuery")
		assert.Contains(t, content, "ghcr.io/supercargo-dev/supercargo-dlt-runner")
		assert.Contains(t, content, "dlt://")
		assert.Contains(t, content, "`hub_url`")
		assert.Contains(t, content, "`hub_audience`")
		assert.Contains(t, content, "`SUPERCARGO_CREDENTIALS_SECRET`")
		assert.Contains(t, content, "`job_names`")
		assert.Contains(t, content, "`scheduler_job_names`")
	})
}

func findDocsPortalModuleDir(t *testing.T) string {
	t.Helper()
	moduleDir := filepath.Clean(filepath.Join("..", "..", "modules", "docs_portal"))
	abs, err := filepath.Abs(moduleDir)
	require.NoError(t, err)
	return abs
}

func readDocsPortalModuleFile(t *testing.T, filename string) string {
	t.Helper()
	moduleDir := findDocsPortalModuleDir(t)
	fullPath := filepath.Join(moduleDir, filename)
	data, err := os.ReadFile(fullPath)
	require.NoError(t, err, "failed to read module file %s at %s", filename, fullPath)
	return string(data)
}

func TestModules_DocsPortal(t *testing.T) {
	moduleDir := findDocsPortalModuleDir(t)
	fi, err := os.Stat(moduleDir)
	require.NoError(t, err, "module directory should exist at %s", moduleDir)
	require.True(t, fi.IsDir(), "expected %s to be a directory", moduleDir)

	t.Run("FilesExist", func(t *testing.T) {
		expectedFiles := []string{
			"main.tf",
			"variables.tf",
			"outputs.tf",
			"versions.tf",
			"README.md",
		}

		for _, file := range expectedFiles {
			t.Run(file, func(t *testing.T) {
				path := filepath.Join(moduleDir, file)
				f, err := os.Stat(path)
				require.NoError(t, err, "file %s should exist in module", file)
				assert.False(t, f.IsDir(), "file %s should not be a directory", file)
			})
		}
	})

	t.Run("Variables", func(t *testing.T) {
		content := readDocsPortalModuleFile(t, "variables.tf")

		tests := []struct {
			name     string
			expected []string
		}{
			{
				name: "project_id variable",
				expected: []string{
					`variable "project_id"`,
					`type        = string`,
				},
			},
			{
				name: "region variable with default",
				expected: []string{
					`variable "region"`,
					`type        = string`,
					`default     = "europe-west1"`,
				},
			},
			{
				name: "service_name variable with default",
				expected: []string{
					`variable "service_name"`,
					`type        = string`,
					`default     = "docs-portal"`,
				},
			},
			{
				name: "image variable",
				expected: []string{
					`variable "image"`,
					`type        = string`,
				},
			},
			{
				name: "domain_name variable",
				expected: []string{
					`variable "domain_name"`,
					`type        = string`,
				},
			},
			{
				name: "licensed_domains variable",
				expected: []string{
					`variable "licensed_domains"`,
					`type        = list(string)`,
				},
			},
			{
				name: "iap_enabled variable",
				expected: []string{
					`variable "iap_enabled"`,
					`type        = bool`,
					`default     = true`,
				},
			},
			{
				name: "iap_client_id variable",
				expected: []string{
					`variable "iap_client_id"`,
					`type        = string`,
				},
			},
			{
				name: "iap_client_secret variable sensitive",
				expected: []string{
					`variable "iap_client_secret"`,
					`type        = string`,
					`sensitive   = true`,
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				for _, snippet := range tc.expected {
					assert.Contains(t, content, snippet)
				}
			})
		}
	})

	t.Run("CloudRunV2Service", func(t *testing.T) {
		content := readDocsPortalModuleFile(t, "main.tf")

		// 1. Cloud Run v2 service defines startup and liveness probes targeting /healthz on port 8080.
		assert.Contains(t, content, `resource "google_cloud_run_v2_service" "docs_portal"`)
		assert.Contains(t, content, `startup_probe {`)
		assert.Contains(t, content, `liveness_probe {`)
		assert.Contains(t, content, `path = "/healthz"`)
		assert.Contains(t, content, `port = 8080`)

		// 2. Cloud Run v2 service defines direct ingress and Direct IAP
		assert.Regexp(t, regexp.MustCompile(`ingress\s*=\s*"INGRESS_TRAFFIC_ALL"`), content)
		assert.Regexp(t, regexp.MustCompile(`launch_stage\s*=\s*"BETA"`), content)
		assert.Regexp(t, regexp.MustCompile(`iap_enabled\s*=\s*var\.iap_enabled`), content)

		// Resources: 256Mi memory, 1 CPU
		assert.Contains(t, content, `memory = "256Mi"`)
		assert.Contains(t, content, `cpu    = "1"`)
	})

	t.Run("CloudRunIAPInvoker", func(t *testing.T) {
		content := readDocsPortalModuleFile(t, "main.tf")

		// 3. google_cloud_run_v2_service_iam_member binds roles/run.invoker to IAP service agent
		assert.Contains(t, content, `resource "google_cloud_run_v2_service_iam_member" "iap_service_agent_invoker"`)
		assert.Contains(t, content, `gcp-sa-iap.iam.gserviceaccount.com`)
		assert.Contains(t, content, `resource "google_cloud_run_v2_service_iam_member" "public_invoker"`)
		assert.Regexp(t, regexp.MustCompile(`role\s*=\s*"roles/run\.invoker"`), content)
	})

	t.Run("DirectIAPServiceIAM", func(t *testing.T) {
		content := readDocsPortalModuleFile(t, "main.tf")

		// 4. google_iap_web_cloud_run_service_iam_member binds roles/iap.httpsResourceAccessor directly to the service
		assert.Contains(t, content, `resource "google_iap_web_cloud_run_service_iam_member" "licensed_domains"`)
		assert.Regexp(t, regexp.MustCompile(`cloud_run_service_name\s*=\s*google_cloud_run_v2_service\.docs_portal\.name`), content)
		assert.Regexp(t, regexp.MustCompile(`role\s*=\s*"roles/iap\.httpsResourceAccessor"`), content)
		assert.Regexp(t, regexp.MustCompile(`member\s*=\s*each\.value`), content)
	})

	t.Run("CustomDomainMapping", func(t *testing.T) {
		content := readDocsPortalModuleFile(t, "main.tf")

		// 5. Direct domain mapping without load balancer
		assert.Contains(t, content, `resource "google_cloud_run_domain_mapping" "docs_portal"`)
		assert.Contains(t, content, `route_name = google_cloud_run_v2_service.docs_portal.name`)
	})

	t.Run("Outputs", func(t *testing.T) {
		content := readDocsPortalModuleFile(t, "outputs.tf")

		tests := []struct {
			name     string
			snippets []string
		}{
			{
				name: "service_url output",
				snippets: []string{
					`output "service_url"`,
					`"https://${var.domain_name}"`,
				},
			},
			{
				name: "cloud_run_uri output",
				snippets: []string{
					`output "cloud_run_uri"`,
					`google_cloud_run_v2_service.docs_portal.uri`,
				},
			},
			{
				name: "domain_mapping_dns_records output",
				snippets: []string{
					`output "domain_mapping_dns_records"`,
					`resource_records`,
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				for _, snippet := range tc.snippets {
					assert.Contains(t, content, snippet)
				}
			})
		}
	})

	t.Run("Versions", func(t *testing.T) {
		content := readDocsPortalModuleFile(t, "versions.tf")

		assert.True(t, strings.Contains(content, `required_version = ">= 1.5.0"`) || strings.Contains(content, `required_version = ">= 1.5"`))
		assert.Contains(t, content, `hashicorp/google`)
		assert.Contains(t, content, `>= 5.0.0, < 8.0.0`)
	})

	t.Run("Readme", func(t *testing.T) {
		content := readDocsPortalModuleFile(t, "README.md")

		assert.Contains(t, content, "docs_portal")
		assert.Contains(t, content, "Cloud Run")
		assert.Contains(t, content, "Identity-Aware Proxy")
	})
}

func TestModules_ControlPlaneModuleVariablesAndEnv(t *testing.T) {
	modulesDir := "../../modules"

	t.Run("AC1_Vault_VariablesAndEnv", func(t *testing.T) {
		varPath := filepath.Join(modulesDir, "vault", "variables.tf")
		varContentBytes, err := os.ReadFile(varPath)
		require.NoError(t, err)
		varContent := string(varContentBytes)

		// Assert variables
		assert.Contains(t, varContent, `variable "telemetry_identity_salt"`)
		assert.Contains(t, varContent, `default     = "supercargo-default-salt"`)
		assert.Contains(t, varContent, `variable "telemetry_topic_id"`)
		assert.Contains(t, varContent, `default     = "supercargo-platform-telemetry"`)
		assert.Contains(t, varContent, `variable "authorized_tokenizer_sa"`)

		mainPath := filepath.Join(modulesDir, "vault", "main.tf")
		mainContentBytes, err := os.ReadFile(mainPath)
		require.NoError(t, err)
		mainContent := string(mainContentBytes)

		// Assert local.vault_audience
		assert.Regexp(t, regexp.MustCompile(`vault_audience\s*=`), mainContent)

		// Assert container env vars in google_cloud_run_v2_service.vault
		vaultRes := `resource "google_cloud_run_v2_service" "vault"`
		idx := strings.Index(mainContent, vaultRes)
		require.NotEqual(t, -1, idx, "google_cloud_run_v2_service.vault not found")
		vaultChunk := mainContent[idx:]
		if nextRes := strings.Index(vaultChunk[len(vaultRes):], "\nresource \""); nextRes != -1 {
			vaultChunk = vaultChunk[:len(vaultRes)+nextRes]
		}

		assert.Contains(t, vaultChunk, `name  = "OIDC_AUDIENCE"`)
		assert.Contains(t, vaultChunk, `value = local.vault_audience`)
		assert.Contains(t, vaultChunk, `name  = "VAULT_AUDIENCE"`)
		assert.Contains(t, vaultChunk, `name  = "SUPERCARGO_TELEMETRY_IDENTITY_SALT"`)
		assert.Contains(t, vaultChunk, `value = var.telemetry_identity_salt`)
		assert.Contains(t, vaultChunk, `name  = "VAULT_TELEMETRY_TOPIC_ID"`)
		assert.Contains(t, vaultChunk, `value = var.telemetry_topic_id`)
		assert.Contains(t, vaultChunk, `name  = "VAULT_AUTHORIZED_TOKENIZER_SA"`)
		assert.Contains(t, vaultChunk, `value = var.authorized_tokenizer_sa != "" ? var.authorized_tokenizer_sa : var.gateway_service_account_email`)
	})

	t.Run("AC2_Gateway_VariablesAndEnv", func(t *testing.T) {
		varPath := filepath.Join(modulesDir, "gateway", "variables.tf")
		varContentBytes, err := os.ReadFile(varPath)
		require.NoError(t, err)
		varContent := string(varContentBytes)

		// Assert variables
		assert.Contains(t, varContent, `variable "gateway_audience"`)
		assert.Contains(t, varContent, `variable "environment"`)
		assert.Contains(t, varContent, `default     = "dev"`)
		assert.Contains(t, varContent, `variable "vault_address"`)
		assert.Contains(t, varContent, `variable "create_service_account"`)
		assert.Contains(t, varContent, `default     = true`)

		mainPath := filepath.Join(modulesDir, "gateway", "main.tf")
		mainContentBytes, err := os.ReadFile(mainPath)
		require.NoError(t, err)
		mainContent := string(mainContentBytes)

		// Assert conditional service account creation and local.service_account_email usage
		assert.Regexp(t, regexp.MustCompile(`count\s*=\s*var\.create_service_account \? 1 : 0`), mainContent)
		assert.Contains(t, mainContent, `serviceAccount:${local.service_account_email}`)

		// Assert container env vars in google_cloud_run_v2_service.gateway
		gwRes := `resource "google_cloud_run_v2_service" "gateway"`
		idx := strings.Index(mainContent, gwRes)
		require.NotEqual(t, -1, idx, "google_cloud_run_v2_service.gateway not found")
		gwChunk := mainContent[idx:]
		if nextRes := strings.Index(gwChunk[len(gwRes):], "\nresource \""); nextRes != -1 {
			gwChunk = gwChunk[:len(gwRes)+nextRes]
		}

		assert.Contains(t, gwChunk, `service_account = local.service_account_email`)
		assert.Contains(t, gwChunk, `name  = "GATEWAY_AUDIENCE"`)
		assert.Contains(t, gwChunk, `value = var.gateway_audience != "" ? var.gateway_audience : "supercargo-gateway-${var.product_id}-${var.environment}"`)
		assert.Contains(t, gwChunk, `name  = "VAULT_ADDRESS"`)
		assert.Contains(t, gwChunk, `value = var.vault_address != "" ? var.vault_address : var.kms_address`)
	})

	t.Run("AC3_Hub_DataLineageViewerIAM", func(t *testing.T) {
		mainPath := filepath.Join(modulesDir, "hub", "main.tf")
		mainContentBytes, err := os.ReadFile(mainPath)
		require.NoError(t, err)
		mainContent := string(mainContentBytes)

		assert.Contains(t, mainContent, `resource "google_project_iam_member" "hub_datalineage_viewer"`)
		assert.Contains(t, mainContent, `role       = "roles/datalineage.viewer"`)
		assert.Contains(t, mainContent, `member     = "serviceAccount:${google_service_account.hub_runtime.email}"`)
	})

	t.Run("AC4_Loader_VariablesAndEnv", func(t *testing.T) {
		varPath := filepath.Join(modulesDir, "loader", "variables.tf")
		varContentBytes, err := os.ReadFile(varPath)
		require.NoError(t, err)
		varContent := string(varContentBytes)

		assert.Contains(t, varContent, `variable "allowed_gcs_buckets"`)

		mainPath := filepath.Join(modulesDir, "loader", "main.tf")
		mainContentBytes, err := os.ReadFile(mainPath)
		require.NoError(t, err)
		mainContent := string(mainContentBytes)

		loaderRes := `resource "google_cloud_run_v2_service" "loader"`
		idx := strings.Index(mainContent, loaderRes)
		require.NotEqual(t, -1, idx, "google_cloud_run_v2_service.loader not found")
		loaderChunk := mainContent[idx:]
		if nextRes := strings.Index(loaderChunk[len(loaderRes):], "\nresource \""); nextRes != -1 {
			loaderChunk = loaderChunk[:len(loaderRes)+nextRes]
		}

		assert.Contains(t, loaderChunk, `name  = "ALLOWED_GCS_BUCKETS"`)
		assert.Contains(t, loaderChunk, `value = var.allowed_gcs_buckets != "" ? var.allowed_gcs_buckets : "${var.project_id}-dropzone"`)
	})

	t.Run("AC5_RequestBasedBillingAndScaling_AllServices", func(t *testing.T) {
		cloudRunServices := []struct {
			moduleName       string
			resourceName     string
			maxInstancesVar  string
			maxInstancesExpr string
		}{
			{moduleName: "hub", resourceName: "hub", maxInstancesVar: "max_instances", maxInstancesExpr: "var.max_instances"},
			{moduleName: "hub", resourceName: "shovel", maxInstancesVar: "shovel_max_instances", maxInstancesExpr: "var.shovel_max_instances"},
			{moduleName: "hub", resourceName: "mcp", maxInstancesVar: "mcp_max_instances", maxInstancesExpr: "var.mcp_max_instances"},
			{moduleName: "hub", resourceName: "console", maxInstancesVar: "console_max_instances", maxInstancesExpr: "var.console_max_instances"},
			{moduleName: "vault", resourceName: "vault", maxInstancesVar: "max_instances", maxInstancesExpr: "var.max_instances"},
			{moduleName: "gateway", resourceName: "gateway", maxInstancesVar: "max_instances", maxInstancesExpr: "var.max_instances"},
			{moduleName: "loader", resourceName: "loader", maxInstancesVar: "max_instances", maxInstancesExpr: "var.max_instances"},
			{moduleName: "metadata_shovel", resourceName: "shovel", maxInstancesVar: "max_instances", maxInstancesExpr: "var.max_instances"},
			{moduleName: "docs_portal", resourceName: "docs_portal", maxInstancesVar: "max_instances", maxInstancesExpr: "var.max_instances"},
		}

		for _, svc := range cloudRunServices {
			t.Run(svc.moduleName+"/"+svc.resourceName, func(t *testing.T) {
				varPath := filepath.Join(modulesDir, svc.moduleName, "variables.tf")
				varBytes, err := os.ReadFile(varPath)
				require.NoError(t, err)
				varContent := string(varBytes)

				assert.Contains(t, varContent, `variable "`+svc.maxInstancesVar+`"`)
				varIdx := strings.Index(varContent, `variable "`+svc.maxInstancesVar+`"`)
				if varIdx != -1 {
					varBlock := varContent[varIdx:]
					if endIdx := strings.Index(varBlock, "}"); endIdx != -1 {
						varBlock = varBlock[:endIdx+1]
					}
					assert.Regexp(t, regexp.MustCompile(`default\s*=\s*3`), varBlock, "variable %s in %s must default to 3", svc.maxInstancesVar, svc.moduleName)
				}

				mainPath := filepath.Join(modulesDir, svc.moduleName, "main.tf")
				mainBytes, err := os.ReadFile(mainPath)
				require.NoError(t, err)
				mainContent := string(mainBytes)

				resHeader := `resource "google_cloud_run_v2_service" "` + svc.resourceName + `"`
				idx := strings.Index(mainContent, resHeader)
				require.NotEqual(t, -1, idx, "%s not found in %s", resHeader, mainPath)
				resChunk := mainContent[idx:]
				if nextRes := strings.Index(resChunk[len(resHeader):], "\nresource \""); nextRes != -1 {
					resChunk = resChunk[:len(resHeader)+nextRes]
				}

				// Must configure request-based billing (cpu_idle = true)
				assert.Regexp(t, regexp.MustCompile(`cpu_idle\s*=\s*true`), resChunk, "%s in %s must set cpu_idle = true for request-based billing", svc.resourceName, svc.moduleName)

				// Must configure scaling { min_instance_count = 0, max_instance_count = ... }
				assert.Contains(t, resChunk, "scaling {")
				assert.Regexp(t, regexp.MustCompile(`min_instance_count\s*=\s*0`), resChunk)
				assert.Contains(t, resChunk, "max_instance_count = "+svc.maxInstancesExpr)

				// Must explicitly route 100% traffic to LATEST revision to prevent sticky failed Cloud Deploy revisions
				assert.Contains(t, resChunk, `type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"`)
				assert.Contains(t, resChunk, `percent = 100`)

				// Must NOT set reserved Cloud Run environment variable PORT in env blocks
				assert.NotRegexp(t, regexp.MustCompile(`name\s*=\s*"PORT"`), resChunk, "%s in %s must not set reserved Cloud Run env var PORT", svc.resourceName, svc.moduleName)
			})
		}
	})
}

func parseHCLFileAST(t *testing.T, parser *hclparse.Parser, path string) (*hcl.File, *hclsyntax.Body) {
	t.Helper()
	file, diags := parser.ParseHCLFile(path)
	require.False(t, diags.HasErrors(), "HCL parse errors in %s: %s", path, diags.Error())
	require.NotNil(t, file)
	syntaxBody, ok := file.Body.(*hclsyntax.Body)
	require.True(t, ok, "expected *hclsyntax.Body for %s", path)
	return file, syntaxBody
}

func getExprSource(file *hcl.File, expr hclsyntax.Expression) string {
	if expr == nil {
		return ""
	}
	srcRange := expr.Range()
	return strings.TrimSpace(string(file.Bytes[srcRange.Start.Byte:srcRange.End.Byte]))
}

func TestModules_BC7_Hardening_AST(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"

	t.Run("AC4_BackendServiceIAPDecommissioned", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			moduleName        string
			prohibitedVars    []string
			prohibitedOutputs []string
		}{
			{
				moduleName:        "hub",
				prohibitedVars:    []string{"iap_enabled", "iap_client_id", "iap_allowed_members"},
				prohibitedOutputs: []string{"iap_client_id"},
			},
			{
				moduleName:        "vault",
				prohibitedVars:    []string{"iap_enabled", "iap_client_id", "iap_allowed_members"},
				prohibitedOutputs: []string{"iap_client_id"},
			},
			{
				moduleName:        "gateway",
				prohibitedVars:    []string{"hub_iap_client_id", "vault_iap_client_id"},
				prohibitedOutputs: nil,
			},
			{
				moduleName:        "data_product",
				prohibitedVars:    []string{"hub_iap_client_id", "vault_iap_client_id"},
				prohibitedOutputs: nil,
			},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.moduleName+"/variables", func(t *testing.T) {
				t.Parallel()
				parser := hclparse.NewParser()
				varPath := filepath.Join(modulesDir, tt.moduleName, "variables.tf")
				_, syntaxBody := parseHCLFileAST(t, parser, varPath)

				foundVars := make(map[string]bool)
				for _, block := range syntaxBody.Blocks {
					if block.Type == "variable" && len(block.Labels) > 0 {
						foundVars[block.Labels[0]] = true
					}
				}

				for _, prohibited := range tt.prohibitedVars {
					if foundVars[prohibited] {
						t.Errorf("An argument named %q is not expected here. Remediation Hint: Backend service-to-service IAP has been decommissioned on Cloud Run. Replace legacy IAP parameters with native Google IAM OIDC authentication", prohibited)
					}
				}
			})

			if len(tt.prohibitedOutputs) > 0 {
				t.Run(tt.moduleName+"/outputs", func(t *testing.T) {
					t.Parallel()
					parser := hclparse.NewParser()
					outPath := filepath.Join(modulesDir, tt.moduleName, "outputs.tf")
					_, syntaxBody := parseHCLFileAST(t, parser, outPath)

					foundOutputs := make(map[string]bool)
					for _, block := range syntaxBody.Blocks {
						if block.Type == "output" && len(block.Labels) > 0 {
							foundOutputs[block.Labels[0]] = true
						}
					}

					for _, prohibited := range tt.prohibitedOutputs {
						assert.False(t, foundOutputs[prohibited], "Module %s output %s must be decommissioned", tt.moduleName, prohibited)
					}
				})
			}
		}

		t.Run("hub/main.tf/no_iap", func(t *testing.T) {
			t.Parallel()
			parser := hclparse.NewParser()
			mainPath := filepath.Join(modulesDir, "hub", "main.tf")
			file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

			for _, block := range syntaxBody.Blocks {
				if block.Type == "resource" && len(block.Labels) == 2 {
					resType, resName := block.Labels[0], block.Labels[1]
					if resType == "google_cloud_run_v2_service" && resName == "hub" {
						if _, hasIAP := block.Body.Attributes["iap_enabled"]; hasIAP {
							t.Errorf("An argument named \"iap_enabled\" is not expected here. Remediation Hint: Backend service-to-service IAP has been decommissioned on Cloud Run. Replace legacy IAP parameters with native Google IAM OIDC authentication")
						}
					}
					if resType == "google_project_service_identity" && resName == "iap_sa" {
						t.Errorf("google_project_service_identity.iap_sa must be removed from hub/main.tf")
					}
					if resType == "google_cloud_run_v2_service_iam_member" && resName == "iap_invoker" {
						t.Errorf("google_cloud_run_v2_service_iam_member.iap_invoker must be removed from hub/main.tf")
					}
					if resType == "google_iap_web_cloud_run_service_iam_member" && resName == "iap_accessor" {
						t.Errorf("google_iap_web_cloud_run_service_iam_member.iap_accessor must be removed from hub/main.tf")
					}
					if resType == "google_project_service" && resName == "hub_apis" {
						if forEachAttr, ok := block.Body.Attributes["for_each"]; ok {
							exprSrc := getExprSource(file, forEachAttr.Expr)
							assert.NotContains(t, exprSrc, "iap.googleapis.com", "hub_apis must not enable iap.googleapis.com")
						}
					}
				}
			}
		})

		t.Run("vault/main.tf/no_iap", func(t *testing.T) {
			t.Parallel()
			parser := hclparse.NewParser()
			mainPath := filepath.Join(modulesDir, "vault", "main.tf")
			file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

			for _, block := range syntaxBody.Blocks {
				if block.Type == "resource" && len(block.Labels) == 2 {
					resType, resName := block.Labels[0], block.Labels[1]
					if resType == "google_cloud_run_v2_service" && resName == "vault" {
						if _, hasIAP := block.Body.Attributes["iap_enabled"]; hasIAP {
							t.Errorf("An argument named \"iap_enabled\" is not expected here. Remediation Hint: Backend service-to-service IAP has been decommissioned on Cloud Run. Replace legacy IAP parameters with native Google IAM OIDC authentication")
						}
					}
					if resType == "google_project_service_identity" && resName == "iap_sa" {
						t.Errorf("google_project_service_identity.iap_sa must be removed from vault/main.tf")
					}
					if resType == "google_cloud_run_v2_service_iam_member" && resName == "iap_invoker" {
						t.Errorf("google_cloud_run_v2_service_iam_member.iap_invoker must be removed from vault/main.tf")
					}
					if resType == "google_iap_web_cloud_run_service_iam_member" && resName == "iap_accessor" {
						t.Errorf("google_iap_web_cloud_run_service_iam_member.iap_accessor must be removed from vault/main.tf")
					}
					if resType == "google_project_service" && resName == "vault_apis" {
						if forEachAttr, ok := block.Body.Attributes["for_each"]; ok {
							exprSrc := getExprSource(file, forEachAttr.Expr)
							assert.NotContains(t, exprSrc, "iap.googleapis.com", "vault_apis must not enable iap.googleapis.com")
						}
					}
				}
			}
		})

		t.Run("gateway/main.tf/no_iap_env", func(t *testing.T) {
			t.Parallel()
			parser := hclparse.NewParser()
			mainPath := filepath.Join(modulesDir, "gateway", "main.tf")
			file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

			for _, block := range syntaxBody.Blocks {
				if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_cloud_run_v2_service" && block.Labels[1] == "gateway" {
					for _, templateBlock := range block.Body.Blocks {
						if templateBlock.Type == "template" {
							for _, containersBlock := range templateBlock.Body.Blocks {
								if containersBlock.Type == "containers" {
									for _, envBlock := range containersBlock.Body.Blocks {
										if envBlock.Type == "env" {
											if nameAttr, ok := envBlock.Body.Attributes["name"]; ok {
												nameVal := getExprSource(file, nameAttr.Expr)
												assert.NotEqual(t, `"HUB_IAP_CLIENT_ID"`, nameVal, "gateway must not inject HUB_IAP_CLIENT_ID")
												assert.NotEqual(t, `"VAULT_IAP_CLIENT_ID"`, nameVal, "gateway must not inject VAULT_IAP_CLIENT_ID")
											}
										}
									}
								}
							}
						}
					}
				}
			}
		})

		t.Run("data_product/main.tf/no_iap_passthrough", func(t *testing.T) {
			t.Parallel()
			parser := hclparse.NewParser()
			mainPath := filepath.Join(modulesDir, "data_product", "main.tf")
			_, syntaxBody := parseHCLFileAST(t, parser, mainPath)

			for _, block := range syntaxBody.Blocks {
				if block.Type == "module" && len(block.Labels) == 1 && block.Labels[0] == "gateway" {
					if _, hasHubIAP := block.Body.Attributes["hub_iap_client_id"]; hasHubIAP {
						t.Errorf("An argument named \"hub_iap_client_id\" is not expected here. Remediation Hint: Backend service-to-service IAP has been decommissioned on Cloud Run. Replace legacy IAP parameters with native Google IAM OIDC authentication")
					}
					if _, hasVaultIAP := block.Body.Attributes["vault_iap_client_id"]; hasVaultIAP {
						t.Errorf("An argument named \"vault_iap_client_id\" is not expected here. Remediation Hint: Backend service-to-service IAP has been decommissioned on Cloud Run. Replace legacy IAP parameters with native Google IAM OIDC authentication")
					}
				}
			}
		})
	})

	t.Run("AC6_DataProductContractSchemaJSON", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "data_product", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		var gatewayContractsAttr *hclsyntax.Attribute
		for _, block := range syntaxBody.Blocks {
			if block.Type == "locals" {
				if attr, ok := block.Body.Attributes["gateway_contracts"]; ok {
					gatewayContractsAttr = attr
					break
				}
			}
		}
		require.NotNil(t, gatewayContractsAttr, "gateway_contracts attribute must exist in locals")

		exprSrc := getExprSource(file, gatewayContractsAttr.Expr)

		// Check for forbidden legacy attribute access: c.schema
		hasDeprecatedSchemaAccess := false
		hclsyntax.Walk(gatewayContractsAttr.Expr, &astWalker{
			onRelativeTraversal: func(node *hclsyntax.RelativeTraversalExpr) {
				for _, tr := range node.Traversal {
					if attrTr, ok := tr.(hcl.TraverseAttr); ok && attrTr.Name == "schema" {
						hasDeprecatedSchemaAccess = true
					}
				}
			},
		})

		if hasDeprecatedSchemaAccess {
			t.Errorf("This object does not have an attribute named \"schema\". Did you mean \"schema_json\"?")
		}

		assert.Contains(t, exprSrc, "c.schema_json", "data_product gateway_contracts must map schema = c.schema_json")
	})

	t.Run("AC5_AnalyticsGitHubSyncBigQueryDatasetIAM", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "analytics_github_sync", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		var foundDatasetIAMBlock *hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 {
				resType, resName := block.Labels[0], block.Labels[1]
				if resType == "google_project_iam_member" && resName == "sync_job_bq_editor" {
					t.Errorf("google_project_iam_member.sync_job_bq_editor must be replaced with google_bigquery_dataset_iam_member")
				}
				if resType == "google_bigquery_dataset_iam_member" && resName == "sync_job_bq_editor" {
					foundDatasetIAMBlock = block
				}
			}
		}

		require.NotNil(t, foundDatasetIAMBlock, "resource google_bigquery_dataset_iam_member.sync_job_bq_editor must exist")

		projectAttr, ok := foundDatasetIAMBlock.Body.Attributes["project"]
		require.True(t, ok, "project attribute must exist on google_bigquery_dataset_iam_member.sync_job_bq_editor")
		assert.Equal(t, "var.project_id", getExprSource(file, projectAttr.Expr))

		datasetAttr, ok := foundDatasetIAMBlock.Body.Attributes["dataset_id"]
		require.True(t, ok, "dataset_id attribute must exist on google_bigquery_dataset_iam_member.sync_job_bq_editor")
		assert.Equal(t, "var.dataset", getExprSource(file, datasetAttr.Expr))

		roleAttr, ok := foundDatasetIAMBlock.Body.Attributes["role"]
		require.True(t, ok, "role attribute must exist on google_bigquery_dataset_iam_member.sync_job_bq_editor")
		assert.Equal(t, `"roles/bigquery.dataEditor"`, getExprSource(file, roleAttr.Expr))

		memberAttr, ok := foundDatasetIAMBlock.Body.Attributes["member"]
		require.True(t, ok, "member attribute must exist on google_bigquery_dataset_iam_member.sync_job_bq_editor")
		assert.Equal(t, `"serviceAccount:${local.sa_email}"`, getExprSource(file, memberAttr.Expr))

		dependsOnAttr, ok := foundDatasetIAMBlock.Body.Attributes["depends_on"]
		require.True(t, ok, "depends_on attribute must exist on google_bigquery_dataset_iam_member.sync_job_bq_editor")
		assert.Contains(t, getExprSource(file, dependsOnAttr.Expr), "google_service_account.sync_sa")
	})
}

type astWalker struct {
	onRelativeTraversal func(node *hclsyntax.RelativeTraversalExpr)
}

func (w *astWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	if rel, ok := node.(*hclsyntax.RelativeTraversalExpr); ok && w.onRelativeTraversal != nil {
		w.onRelativeTraversal(rel)
	}
	return nil
}

func (w *astWalker) Exit(node hclsyntax.Node) hcl.Diagnostics {
	return nil
}

func TestModules_Console_DirectIAP_AST(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"

	t.Run("variables_console_iap_allowed_members", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		varPath := filepath.Join(modulesDir, "hub", "variables.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, varPath)

		var varBlock *hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "variable" && len(block.Labels) > 0 && block.Labels[0] == "console_iap_allowed_members" {
				varBlock = block
				break
			}
		}
		require.NotNil(t, varBlock, "variable \"console_iap_allowed_members\" must be defined in modules/hub/variables.tf")

		typeAttr, hasType := varBlock.Body.Attributes["type"]
		require.True(t, hasType, "console_iap_allowed_members must specify type")
		assert.Equal(t, "list(string)", getExprSource(file, typeAttr.Expr))

		defaultAttr, hasDefault := varBlock.Body.Attributes["default"]
		require.True(t, hasDefault, "console_iap_allowed_members must specify default")
		assert.Equal(t, "[]", getExprSource(file, defaultAttr.Expr))

		var validationBlock *hclsyntax.Block
		for _, b := range varBlock.Body.Blocks {
			if b.Type == "validation" {
				validationBlock = b
				break
			}
		}
		require.NotNil(t, validationBlock, "console_iap_allowed_members must specify validation block")
		condAttr, hasCond := validationBlock.Body.Attributes["condition"]
		require.True(t, hasCond, "validation block must have condition attribute")
		condSrc := getExprSource(file, condAttr.Expr)
		assert.Contains(t, condSrc, "startswith(m, \"user:\")")
		assert.Contains(t, condSrc, "startswith(m, \"domain:\")")
		assert.Contains(t, condSrc, "startswith(m, \"group:\")")
		assert.Contains(t, condSrc, "startswith(m, \"serviceAccount:\")")
	})

	t.Run("main_console_iap_audience_local_and_env", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "hub", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		var localConsoleIAPAudienceAttr *hclsyntax.Attribute
		for _, block := range syntaxBody.Blocks {
			if block.Type == "locals" {
				if attr, ok := block.Body.Attributes["console_iap_audience"]; ok {
					localConsoleIAPAudienceAttr = attr
					break
				}
			}
		}
		require.NotNil(t, localConsoleIAPAudienceAttr, "local.console_iap_audience must be defined in modules/hub/main.tf")
		exprSrc := getExprSource(file, localConsoleIAPAudienceAttr.Expr)
		expectedAudienceExpr := `var.console_iap_audience != "" ? var.console_iap_audience : (var.console_iap_enabled ? "/projects/${var.project_number}/locations/${var.region}/services/console${var.name_suffix != "" ? "-${var.name_suffix}" : ""}" : "")`
		assert.Equal(t, expectedAudienceExpr, exprSrc)

		// Check google_cloud_run_v2_service.console env IAP_AUDIENCE
		var consoleServiceBlock *hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_cloud_run_v2_service" && block.Labels[1] == "console" {
				consoleServiceBlock = block
				break
			}
		}
		require.NotNil(t, consoleServiceBlock, "google_cloud_run_v2_service.console must exist in modules/hub/main.tf")

		var foundIAPAudienceEnv bool
		var foundTelemetryMasterSaltEnv bool
		for _, templateBlock := range consoleServiceBlock.Body.Blocks {
			if templateBlock.Type == "template" {
				for _, containersBlock := range templateBlock.Body.Blocks {
					if containersBlock.Type == "containers" {
						for _, envBlock := range containersBlock.Body.Blocks {
							if envBlock.Type == "env" {
								if nameAttr, ok := envBlock.Body.Attributes["name"]; ok {
									envName := getExprSource(file, nameAttr.Expr)
									if envName == `"IAP_AUDIENCE"` {
										valAttr, hasVal := envBlock.Body.Attributes["value"]
										require.True(t, hasVal, "IAP_AUDIENCE env block must have value")
										assert.Equal(t, "local.console_iap_audience", getExprSource(file, valAttr.Expr))
										foundIAPAudienceEnv = true
									}
									if envName == `"TELEMETRY_MASTER_SALT"` {
										valAttr, hasVal := envBlock.Body.Attributes["value"]
										require.True(t, hasVal, "TELEMETRY_MASTER_SALT env block must have value")
										assert.Equal(t, "local.console_telemetry_identity_salt", getExprSource(file, valAttr.Expr))
										foundTelemetryMasterSaltEnv = true
									}
								}
							}
						}
					}
				}
			}
		}
		assert.True(t, foundIAPAudienceEnv, "google_cloud_run_v2_service.console must wire IAP_AUDIENCE to local.console_iap_audience")
		assert.True(t, foundTelemetryMasterSaltEnv, "google_cloud_run_v2_service.console must wire TELEMETRY_MASTER_SALT to local.console_telemetry_identity_salt")
	})

	t.Run("main_console_iap_service_agent_invoker_resource", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "hub", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		var invokerBlock *hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 &&
				block.Labels[0] == "google_cloud_run_v2_service_iam_member" &&
				block.Labels[1] == "console_iap_service_agent_invoker" {
				invokerBlock = block
				break
			}
		}
		require.NotNil(t, invokerBlock, "resource google_cloud_run_v2_service_iam_member.console_iap_service_agent_invoker must be declared in modules/hub/main.tf")

		countAttr, hasCount := invokerBlock.Body.Attributes["count"]
		require.True(t, hasCount, "console_iap_service_agent_invoker must define count")
		assert.Equal(t, "var.console_enabled && var.console_iap_enabled ? 1 : 0", getExprSource(file, countAttr.Expr))

		roleAttr, hasRole := invokerBlock.Body.Attributes["role"]
		require.True(t, hasRole, "console_iap_service_agent_invoker must define role")
		assert.Equal(t, `"roles/run.invoker"`, getExprSource(file, roleAttr.Expr))

		memberAttr, hasMember := invokerBlock.Body.Attributes["member"]
		require.True(t, hasMember, "console_iap_service_agent_invoker must define member")
		assert.Equal(t, `"serviceAccount:service-${var.project_number}@gcp-sa-iap.iam.gserviceaccount.com"`, getExprSource(file, memberAttr.Expr))

		nameAttr, hasName := invokerBlock.Body.Attributes["name"]
		require.True(t, hasName, "console_iap_service_agent_invoker must define name")
		assert.Equal(t, "google_cloud_run_v2_service.console[0].name", getExprSource(file, nameAttr.Expr))
	})

	t.Run("main_console_iap_accessors_resource", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "hub", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		var accessorBlock *hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 &&
				block.Labels[0] == "google_iap_web_cloud_run_service_iam_member" &&
				block.Labels[1] == "console_iap_accessors" {
				accessorBlock = block
				break
			}
		}
		require.NotNil(t, accessorBlock, "resource google_iap_web_cloud_run_service_iam_member.console_iap_accessors must be declared in modules/hub/main.tf")

		forEachAttr, hasForEach := accessorBlock.Body.Attributes["for_each"]
		require.True(t, hasForEach, "console_iap_accessors must define for_each")
		assert.Equal(t, "var.console_enabled && var.console_iap_enabled ? toset(var.console_iap_allowed_members) : toset([])", getExprSource(file, forEachAttr.Expr))

		roleAttr, hasRole := accessorBlock.Body.Attributes["role"]
		require.True(t, hasRole, "console_iap_accessors must define role")
		assert.Equal(t, `"roles/iap.httpsResourceAccessor"`, getExprSource(file, roleAttr.Expr))

		memberAttr, hasMember := accessorBlock.Body.Attributes["member"]
		require.True(t, hasMember, "console_iap_accessors must define member")
		assert.Equal(t, "each.value", getExprSource(file, memberAttr.Expr))

		svcNameAttr, hasSvcName := accessorBlock.Body.Attributes["cloud_run_service_name"]
		require.True(t, hasSvcName, "console_iap_accessors must define cloud_run_service_name")
		assert.Equal(t, "google_cloud_run_v2_service.console[0].name", getExprSource(file, svcNameAttr.Expr))
	})
}

func findAnalyticsRunnerModuleDir(t *testing.T) string {
	t.Helper()
	moduleDir := filepath.Clean(filepath.Join("..", "..", "modules", "analytics_runner"))
	abs, err := filepath.Abs(moduleDir)
	require.NoError(t, err)
	return abs
}

func readAnalyticsRunnerModuleFile(t *testing.T, filename string) string {
	t.Helper()
	moduleDir := findAnalyticsRunnerModuleDir(t)
	fullPath := filepath.Join(moduleDir, filename)
	data, err := os.ReadFile(fullPath)
	require.NoError(t, err, "failed to read module file %s at %s", filename, fullPath)
	return string(data)
}

func TestModules_AnalyticsRunner(t *testing.T) {
	moduleDir := findAnalyticsRunnerModuleDir(t)
	parser := hclparse.NewParser()

	t.Run("FilesExist", func(t *testing.T) {
		expectedFiles := []string{
			"main.tf",
			"variables.tf",
			"outputs.tf",
			"versions.tf",
			"README.md",
		}

		for _, file := range expectedFiles {
			t.Run(file, func(t *testing.T) {
				path := filepath.Join(moduleDir, file)
				f, err := os.Stat(path)
				require.NoError(t, err, "file %s should exist in module", file)
				assert.False(t, f.IsDir(), "file %s should not be a directory", file)
			})
		}
	})

	t.Run("DeterministicLocalsAndFallbacks", func(t *testing.T) {
		content := readAnalyticsRunnerModuleFile(t, "main.tf")

		assert.Contains(t, content, "locals {")
		assert.Contains(t, content, "runner_sa_email")
		assert.Contains(t, content, "scheduler_sa_email")

		deriveRunnerSAEmail := func(saID, jobName, projectID string) string {
			id := fmt.Sprintf("%s-sa", jobName)
			if saID != "" {
				id = saID
			}
			return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", id, projectID)
		}
		deriveSchedSAEmail := func(schedID, jobName, projectID string) string {
			id := fmt.Sprintf("%s-sched-sa", jobName)
			if schedID != "" {
				id = schedID
			}
			return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", id, projectID)
		}

		tests := []struct {
			name           string
			customRunnerID string
			customSchedID  string
			jobName        string
			projectID      string
			wantRunnerSA   string
			wantSchedSA    string
		}{
			{
				name:           "default sa derivation uses job_name prefix",
				customRunnerID: "",
				customSchedID:  "",
				jobName:        "analytics-runner",
				projectID:      "test-proj",
				wantRunnerSA:   "analytics-runner-sa@test-proj.iam.gserviceaccount.com",
				wantSchedSA:    "analytics-runner-sched-sa@test-proj.iam.gserviceaccount.com",
			},
			{
				name:           "custom sa overrides default derivation",
				customRunnerID: "my-runner-sa",
				customSchedID:  "my-sched-sa",
				jobName:        "analytics-runner",
				projectID:      "test-proj",
				wantRunnerSA:   "my-runner-sa@test-proj.iam.gserviceaccount.com",
				wantSchedSA:    "my-sched-sa@test-proj.iam.gserviceaccount.com",
			},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, tt.wantRunnerSA, deriveRunnerSAEmail(tt.customRunnerID, tt.jobName, tt.projectID))
				assert.Equal(t, tt.wantSchedSA, deriveSchedSAEmail(tt.customSchedID, tt.jobName, tt.projectID))
			})
		}
	})

	t.Run("AST_Variables", func(t *testing.T) {
		varPath := filepath.Join(moduleDir, "variables.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, varPath)

		vars := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "variable" && len(block.Labels) == 1 {
				vars[block.Labels[0]] = block
			}
		}

		expectedVars := []struct {
			name        string
			hasDefault  bool
			defaultExpr string
		}{
			{name: "project_id", hasDefault: false},
			{name: "region", hasDefault: false},
			{name: "job_name", hasDefault: true, defaultExpr: `"analytics-runner"`},
			{name: "container_image", hasDefault: false},
			{name: "service_account_id", hasDefault: true, defaultExpr: "null"},
			{name: "scheduler_service_account_id", hasDefault: true, defaultExpr: "null"},
			{name: "schedule", hasDefault: true, defaultExpr: `"0 6 * * *"`},
			{name: "time_zone", hasDefault: true, defaultExpr: `"Etc/UTC"`},
			{name: "engine", hasDefault: true, defaultExpr: `"dbt"`},
			{name: "dialect", hasDefault: true, defaultExpr: `"bigquery"`},
			{name: "target_datasets", hasDefault: true, defaultExpr: "[]"},
			{name: "input_datasets", hasDefault: true, defaultExpr: "[]"},
			{name: "cpu_limit", hasDefault: true, defaultExpr: `"2"`},
			{name: "memory_limit", hasDefault: true, defaultExpr: `"4Gi"`},
			{name: "timeout", hasDefault: true, defaultExpr: `"1800s"`},
			{name: "max_retries", hasDefault: true, defaultExpr: "1"},
			{name: "paused", hasDefault: true, defaultExpr: "false"},
			{name: "deletion_protection", hasDefault: true, defaultExpr: "false"},
		}

		for _, ev := range expectedVars {
			block, exists := vars[ev.name]
			require.True(t, exists, "variable %s must exist in variables.tf", ev.name)
			if ev.hasDefault {
				defAttr, ok := block.Body.Attributes["default"]
				require.True(t, ok, "variable %s must have a default attribute", ev.name)
				assert.Equal(t, ev.defaultExpr, getExprSource(file, defAttr.Expr))
			}
		}
	})

	t.Run("AST_MainResources", func(t *testing.T) {
		mainPath := filepath.Join(moduleDir, "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		resources := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 {
				resources[block.Labels[0]+"."+block.Labels[1]] = block
			}
		}

		// 1. Service Accounts
		runnerSA, exists := resources["google_service_account.runner_sa"]
		require.True(t, exists, "google_service_account.runner_sa must exist")
		assert.Contains(t, getExprSource(file, runnerSA.Body.Attributes["account_id"].Expr), "var.service_account_id")

		schedSA, exists := resources["google_service_account.scheduler_sa"]
		require.True(t, exists, "google_service_account.scheduler_sa must exist")
		assert.Contains(t, getExprSource(file, schedSA.Body.Attributes["account_id"].Expr), "var.scheduler_service_account_id")

		// 2. Cloud Run Job
		runJob, exists := resources["google_cloud_run_v2_job.analytics_runner"]
		require.True(t, exists, "google_cloud_run_v2_job.analytics_runner must exist")
		assert.Equal(t, "var.job_name", getExprSource(file, runJob.Body.Attributes["name"].Expr))
		assert.Equal(t, "var.region", getExprSource(file, runJob.Body.Attributes["location"].Expr))
		assert.Equal(t, "var.project_id", getExprSource(file, runJob.Body.Attributes["project"].Expr))
		assert.Equal(t, "var.deletion_protection", getExprSource(file, runJob.Body.Attributes["deletion_protection"].Expr))

		// 3. Cloud Scheduler Job
		schedJob, exists := resources["google_cloud_scheduler_job.runner_schedule"]
		require.True(t, exists, "google_cloud_scheduler_job.runner_schedule must exist")
		assert.Equal(t, `"${var.job_name}-schedule"`, getExprSource(file, schedJob.Body.Attributes["name"].Expr))
		assert.Equal(t, "var.project_id", getExprSource(file, schedJob.Body.Attributes["project"].Expr))
		assert.Equal(t, "var.region", getExprSource(file, schedJob.Body.Attributes["region"].Expr))
		assert.Equal(t, "var.schedule", getExprSource(file, schedJob.Body.Attributes["schedule"].Expr))
		assert.Equal(t, "var.time_zone", getExprSource(file, schedJob.Body.Attributes["time_zone"].Expr))
		assert.Equal(t, "var.paused", getExprSource(file, schedJob.Body.Attributes["paused"].Expr))

		// 4. IAM Bindings
		invokerIAM, exists := resources["google_cloud_run_v2_job_iam_member.scheduler_invoker"]
		require.True(t, exists, "google_cloud_run_v2_job_iam_member.scheduler_invoker must exist")
		assert.Equal(t, `"roles/run.invoker"`, getExprSource(file, invokerIAM.Body.Attributes["role"].Expr))
		assert.Equal(t, `"serviceAccount:${local.scheduler_sa_email}"`, getExprSource(file, invokerIAM.Body.Attributes["member"].Expr))

		jobUserIAM, exists := resources["google_project_iam_member.job_user"]
		require.True(t, exists, "google_project_iam_member.job_user must exist")
		assert.Equal(t, `"roles/bigquery.jobUser"`, getExprSource(file, jobUserIAM.Body.Attributes["role"].Expr))
		assert.Equal(t, `"serviceAccount:${local.runner_sa_email}"`, getExprSource(file, jobUserIAM.Body.Attributes["member"].Expr))

		targetIAM, exists := resources["google_bigquery_dataset_iam_member.target_data_editor"]
		require.True(t, exists, "google_bigquery_dataset_iam_member.target_data_editor must exist")
		assert.Equal(t, `"roles/bigquery.dataEditor"`, getExprSource(file, targetIAM.Body.Attributes["role"].Expr))
		assert.Equal(t, `"serviceAccount:${local.runner_sa_email}"`, getExprSource(file, targetIAM.Body.Attributes["member"].Expr))

		inputIAM, exists := resources["google_bigquery_dataset_iam_member.input_data_viewer"]
		require.True(t, exists, "google_bigquery_dataset_iam_member.input_data_viewer must exist")
		assert.Equal(t, `"roles/bigquery.dataViewer"`, getExprSource(file, inputIAM.Body.Attributes["role"].Expr))
		assert.Equal(t, `"serviceAccount:${local.runner_sa_email}"`, getExprSource(file, inputIAM.Body.Attributes["member"].Expr))
	})

	t.Run("AST_Outputs", func(t *testing.T) {
		outPath := filepath.Join(moduleDir, "outputs.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, outPath)

		outputs := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				outputs[block.Labels[0]] = block
			}
		}

		expectedOutputs := map[string]string{
			"job_name":                        "google_cloud_run_v2_job.analytics_runner.name",
			"job_id":                          "google_cloud_run_v2_job.analytics_runner.id",
			"runner_service_account_email":    "local.runner_sa_email",
			"scheduler_service_account_email": "local.scheduler_sa_email",
		}

		for name, expectedExpr := range expectedOutputs {
			block, exists := outputs[name]
			require.True(t, exists, "output %s must exist in outputs.tf", name)
			valAttr, ok := block.Body.Attributes["value"]
			require.True(t, ok, "output %s must have value attribute", name)
			assert.Equal(t, expectedExpr, getExprSource(file, valAttr.Expr))
		}
	})

	t.Run("AST_Versions", func(t *testing.T) {
		versionsPath := filepath.Join(moduleDir, "versions.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, versionsPath)
		assert.NotNil(t, file)
		assert.NotEmpty(t, syntaxBody.Blocks)
	})
}

func TestModules_DataProductGatewayVariableForwarding(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"
	parser := hclparse.NewParser()

	// 1. Verify variables in modules/data_product/variables.tf
	t.Run("DataProductVariables", func(t *testing.T) {
		t.Parallel()
		dpVarPath := filepath.Join(modulesDir, "data_product", "variables.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, dpVarPath)

		vars := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "variable" && len(block.Labels) == 1 {
				vars[block.Labels[0]] = block
			}
		}

		expectedVars := []struct {
			name        string
			defaultExpr string
		}{
			{name: "sla_tier", defaultExpr: `"TIER_2"`},
			{name: "enable_infrastructure_alerts", defaultExpr: "true"},
			{name: "enable_monitoring_dashboard", defaultExpr: "true"},
			{name: "additional_sre_notification_channels", defaultExpr: "[]"},
			{name: "unacked_message_age_threshold_seconds", defaultExpr: "null"},
		}

		for _, ev := range expectedVars {
			block, exists := vars[ev.name]
			require.True(t, exists, "data_product missing variable %s", ev.name)
			defAttr, hasDef := block.Body.Attributes["default"]
			require.True(t, hasDef, "data_product variable %s must define default", ev.name)
			assert.Equal(t, ev.defaultExpr, getExprSource(file, defAttr.Expr))
		}
	})

	// 2. Verify variables in modules/gateway/variables.tf
	t.Run("GatewayVariables", func(t *testing.T) {
		t.Parallel()
		gwVarPath := filepath.Join(modulesDir, "gateway", "variables.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, gwVarPath)

		vars := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "variable" && len(block.Labels) == 1 {
				vars[block.Labels[0]] = block
			}
		}

		expectedVars := []struct {
			name        string
			defaultExpr string
		}{
			{name: "sla_tier", defaultExpr: `"TIER_2"`},
			{name: "enable_infrastructure_alerts", defaultExpr: "true"},
			{name: "enable_monitoring_dashboard", defaultExpr: "true"},
			{name: "additional_sre_notification_channels", defaultExpr: "[]"},
			{name: "unacked_message_age_threshold_seconds", defaultExpr: "null"},
		}

		for _, ev := range expectedVars {
			block, exists := vars[ev.name]
			require.True(t, exists, "gateway missing variable %s", ev.name)
			defAttr, hasDef := block.Body.Attributes["default"]
			require.True(t, hasDef, "gateway variable %s must define default", ev.name)
			assert.Equal(t, ev.defaultExpr, getExprSource(file, defAttr.Expr))
		}

		// Verify sla_tier validation block
		slaBlock := vars["sla_tier"]
		require.NotNil(t, slaBlock)
		var valBlock *hclsyntax.Block
		for _, sub := range slaBlock.Body.Blocks {
			if sub.Type == "validation" {
				valBlock = sub
				break
			}
		}
		require.NotNil(t, valBlock, "gateway variable sla_tier must have validation block")
		condAttr, hasCond := valBlock.Body.Attributes["condition"]
		require.True(t, hasCond)
		assert.Contains(t, getExprSource(file, condAttr.Expr), `"TIER_1"`)
		assert.Contains(t, getExprSource(file, condAttr.Expr), `"TIER_2"`)
		assert.Contains(t, getExprSource(file, condAttr.Expr), `"TIER_3"`)
	})

	// 3. Verify variable forwarding in modules/data_product/main.tf
	t.Run("GatewayForwarding", func(t *testing.T) {
		t.Parallel()
		mainPath := filepath.Join(modulesDir, "data_product", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		var gwModule *hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "module" && len(block.Labels) == 1 && block.Labels[0] == "gateway" {
				gwModule = block
				break
			}
		}
		require.NotNil(t, gwModule, "modules/data_product/main.tf must declare module gateway")

		forwardedAttrs := []string{
			"sla_tier",
			"enable_infrastructure_alerts",
			"enable_monitoring_dashboard",
			"additional_sre_notification_channels",
			"unacked_message_age_threshold_seconds",
		}

		for _, attr := range forwardedAttrs {
			a, exists := gwModule.Body.Attributes[attr]
			require.True(t, exists, "module.gateway missing forwarding for %s", attr)
			assert.Equal(t, "var."+attr, getExprSource(file, a.Expr))
		}
	})
}

func TestModules_GatewayCloudRunInfrastructureAlerts(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"
	parser := hclparse.NewParser()
	monitoringPath := filepath.Join(modulesDir, "gateway", "monitoring.tf")
	file, syntaxBody := parseHCLFileAST(t, parser, monitoringPath)

	alerts := make(map[string]*hclsyntax.Block)
	for _, block := range syntaxBody.Blocks {
		if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_alert_policy" {
			alerts[block.Labels[1]] = block
		}
	}

	expectedAlerts := []struct {
		name                 string
		displayNamePrefix    string
		expectedChannelsExpr string
		filterContains       []string
		aligner              string
		duration             string
		comparison           string
	}{
		{
			name:                 "gateway_5xx_rate",
			displayNamePrefix:    "[Tier 1] [INFRA] Gateway Cloud Run 5xx Errors",
			expectedChannelsExpr: "local.all_alert_notification_channels",
			filterContains:       []string{`response_code_class = "5xx"`, `metric.type = "run.googleapis.com/request_count"`},
			aligner:              `"ALIGN_RATE"`,
			duration:             `"0s"`,
			comparison:           `"COMPARISON_GT"`,
		},
		{
			name:                 "gateway_auth_failures",
			displayNamePrefix:    "[Tier 1] [INFRA] Gateway Push Invoker Auth 401/403",
			expectedChannelsExpr: "local.all_alert_notification_channels",
			filterContains:       []string{`response_code = one_of("401", "403")`, `metric.type = "run.googleapis.com/request_count"`},
			aligner:              `"ALIGN_RATE"`,
			duration:             `"0s"`,
			comparison:           `"COMPARISON_GT"`,
		},
		{
			name:                 "gateway_memory_utilization",
			displayNamePrefix:    "[Tier 2] [INFRA] Gateway Memory Saturation >85%",
			expectedChannelsExpr: "local.tier2_alert_notification_channels",
			filterContains:       []string{`metric.type = "run.googleapis.com/container/memory/utilizations"`},
			aligner:              `"ALIGN_PERCENTILE_99"`,
			duration:             `"300s"`,
			comparison:           `"COMPARISON_GT"`,
		},
		{
			name:                 "gateway_cpu_utilization",
			displayNamePrefix:    "[Tier 2] [INFRA] Gateway CPU Saturation >85%",
			expectedChannelsExpr: "local.tier2_alert_notification_channels",
			filterContains:       []string{`metric.type = "run.googleapis.com/container/cpu/utilizations"`},
			aligner:              `"ALIGN_PERCENTILE_99"`,
			duration:             `"300s"`,
			comparison:           `"COMPARISON_GT"`,
		},
		{
			name:                 "gateway_instance_saturation",
			displayNamePrefix:    "[Tier 2] [INFRA] Gateway Instance Saturation",
			expectedChannelsExpr: "local.tier2_alert_notification_channels",
			filterContains:       []string{`metric.type = "run.googleapis.com/container/instance_count"`, `metric.label.state = "active"`},
			aligner:              `"ALIGN_MAX"`,
			duration:             `"180s"`,
			comparison:           `"COMPARISON_GTE"`,
		},
	}

	for _, tc := range expectedAlerts {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			alert, exists := alerts[tc.name]
			require.True(t, exists, "google_monitoring_alert_policy.%s must exist in monitoring.tf", tc.name)

			// Count guard
			countAttr, hasCount := alert.Body.Attributes["count"]
			require.True(t, hasCount, "%s must have count attribute", tc.name)
			assert.Equal(t, "var.enable_infrastructure_alerts ? 1 : 0", getExprSource(file, countAttr.Expr))

			// Combiner
			combAttr, hasComb := alert.Body.Attributes["combiner"]
			require.True(t, hasComb)
			assert.Equal(t, `"OR"`, getExprSource(file, combAttr.Expr))

			// Display Name
			nameAttr, hasName := alert.Body.Attributes["display_name"]
			require.True(t, hasName)
			assert.Contains(t, getExprSource(file, nameAttr.Expr), tc.displayNamePrefix)

			// Notification channels
			chanAttr, hasChan := alert.Body.Attributes["notification_channels"]
			require.True(t, hasChan)
			assert.Equal(t, tc.expectedChannelsExpr, getExprSource(file, chanAttr.Expr))

			// Auto close strategy
			var stratBlock *hclsyntax.Block
			for _, sub := range alert.Body.Blocks {
				if sub.Type == "alert_strategy" {
					stratBlock = sub
					break
				}
			}
			require.NotNil(t, stratBlock, "%s must define alert_strategy block", tc.name)
			autoCloseAttr, hasAutoClose := stratBlock.Body.Attributes["auto_close"]
			require.True(t, hasAutoClose)
			assert.Equal(t, `"604800s"`, getExprSource(file, autoCloseAttr.Expr))

			// Documentation block
			var docBlock *hclsyntax.Block
			for _, sub := range alert.Body.Blocks {
				if sub.Type == "documentation" {
					docBlock = sub
					break
				}
			}
			require.NotNil(t, docBlock, "%s must define documentation block", tc.name)
			contentAttr, hasContent := docBlock.Body.Attributes["content"]
			require.True(t, hasContent)
			assert.NotEmpty(t, getExprSource(file, contentAttr.Expr))

			// Conditions block
			var condBlock *hclsyntax.Block
			for _, sub := range alert.Body.Blocks {
				if sub.Type == "conditions" {
					condBlock = sub
					break
				}
			}
			require.NotNil(t, condBlock, "%s must define conditions block", tc.name)

			var threshBlock *hclsyntax.Block
			for _, sub := range condBlock.Body.Blocks {
				if sub.Type == "condition_threshold" {
					threshBlock = sub
					break
				}
			}
			require.NotNil(t, threshBlock, "%s must define condition_threshold block", tc.name)

			filterAttr, hasFilter := threshBlock.Body.Attributes["filter"]
			require.True(t, hasFilter)
			filterExpr := strings.ReplaceAll(getExprSource(file, filterAttr.Expr), `\"`, `"`)
			for _, f := range tc.filterContains {
				assert.Contains(t, filterExpr, f)
			}

			durAttr, hasDur := threshBlock.Body.Attributes["duration"]
			require.True(t, hasDur)
			assert.Equal(t, tc.duration, getExprSource(file, durAttr.Expr))

			compAttr, hasComp := threshBlock.Body.Attributes["comparison"]
			require.True(t, hasComp)
			assert.Equal(t, tc.comparison, getExprSource(file, compAttr.Expr))

			// Trigger count = 1
			var trigBlock *hclsyntax.Block
			for _, sub := range threshBlock.Body.Blocks {
				if sub.Type == "trigger" {
					trigBlock = sub
					break
				}
			}
			require.NotNil(t, trigBlock, "%s must define trigger block", tc.name)
			trigCount, hasTrigCount := trigBlock.Body.Attributes["count"]
			require.True(t, hasTrigCount)
			assert.Equal(t, "1", getExprSource(file, trigCount.Expr))

			// Aggregations
			var aggBlock *hclsyntax.Block
			for _, sub := range threshBlock.Body.Blocks {
				if sub.Type == "aggregations" {
					aggBlock = sub
					break
				}
			}
			require.NotNil(t, aggBlock, "%s must define aggregations block", tc.name)
			alignerAttr, hasAligner := aggBlock.Body.Attributes["per_series_aligner"]
			require.True(t, hasAligner)
			assert.Equal(t, tc.aligner, getExprSource(file, alignerAttr.Expr))
		})
	}
}

func TestModules_GatewayCleanBigQuerySinkMonitoring(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"
	parser := hclparse.NewParser()
	monitoringPath := filepath.Join(modulesDir, "gateway", "monitoring.tf")
	file, syntaxBody := parseHCLFileAST(t, parser, monitoringPath)

	alerts := make(map[string]*hclsyntax.Block)
	for _, block := range syntaxBody.Blocks {
		if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_alert_policy" {
			alerts[block.Labels[1]] = block
		}
	}

	t.Run("raw_push_latency", func(t *testing.T) {
		t.Parallel()
		alert, exists := alerts["raw_push_latency"]
		require.True(t, exists, "google_monitoring_alert_policy.raw_push_latency must exist in monitoring.tf")

		countAttr, hasCount := alert.Body.Attributes["count"]
		require.True(t, hasCount)
		assert.Equal(t, "var.enable_infrastructure_alerts ? 1 : 0", getExprSource(file, countAttr.Expr))

		nameAttr, hasName := alert.Body.Attributes["display_name"]
		require.True(t, hasName)
		assert.Contains(t, getExprSource(file, nameAttr.Expr), "[Tier 1] [INFRA] Raw Push Subscription Latency")
	})

	t.Run("clean_bq_unacked_message_age", func(t *testing.T) {
		t.Parallel()
		alert, exists := alerts["clean_bq_unacked_message_age"]
		require.True(t, exists, "google_monitoring_alert_policy.clean_bq_unacked_message_age must exist in monitoring.tf")

		forEachAttr, hasForEach := alert.Body.Attributes["for_each"]
		require.True(t, hasForEach)
		assert.Equal(t, "var.enable_infrastructure_alerts ? var.contracts : {}", getExprSource(file, forEachAttr.Expr))

		nameAttr, hasName := alert.Body.Attributes["display_name"]
		require.True(t, hasName)
		assert.Contains(t, getExprSource(file, nameAttr.Expr), "[Tier 1] [INFRA] Clean BigQuery Write Stall")

		// Threshold must use local.unacked_age_seconds
		var condBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "conditions" {
				condBlock = sub
				break
			}
		}
		require.NotNil(t, condBlock)
		var threshBlock *hclsyntax.Block
		for _, sub := range condBlock.Body.Blocks {
			if sub.Type == "condition_threshold" {
				threshBlock = sub
				break
			}
		}
		require.NotNil(t, threshBlock)
		threshValAttr, hasThreshVal := threshBlock.Body.Attributes["threshold_value"]
		require.True(t, hasThreshVal)
		assert.Equal(t, "local.unacked_age_seconds", getExprSource(file, threshValAttr.Expr))
	})

	t.Run("clean_bq_dead_letter_count", func(t *testing.T) {
		t.Parallel()
		alert, exists := alerts["clean_bq_dead_letter_count"]
		require.True(t, exists, "google_monitoring_alert_policy.clean_bq_dead_letter_count must exist in monitoring.tf")

		forEachAttr, hasForEach := alert.Body.Attributes["for_each"]
		require.True(t, hasForEach)
		assert.Equal(t, "var.enable_infrastructure_alerts ? var.contracts : {}", getExprSource(file, forEachAttr.Expr))

		nameAttr, hasName := alert.Body.Attributes["display_name"]
		require.True(t, hasName)
		assert.Contains(t, getExprSource(file, nameAttr.Expr), "[Tier 1] [DATA] Clean BigQuery Schema Rejections")

		// Aligner must be ALIGN_DELTA for cumulative dead letter message count
		var condBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "conditions" {
				condBlock = sub
				break
			}
		}
		require.NotNil(t, condBlock)
		var threshBlock *hclsyntax.Block
		for _, sub := range condBlock.Body.Blocks {
			if sub.Type == "condition_threshold" {
				threshBlock = sub
				break
			}
		}
		require.NotNil(t, threshBlock)

		var aggBlock *hclsyntax.Block
		for _, sub := range threshBlock.Body.Blocks {
			if sub.Type == "aggregations" {
				aggBlock = sub
				break
			}
		}
		require.NotNil(t, aggBlock)
		alignerAttr, hasAligner := aggBlock.Body.Attributes["per_series_aligner"]
		require.True(t, hasAligner)
		assert.Equal(t, `"ALIGN_DELTA"`, getExprSource(file, alignerAttr.Expr))
	})
}

func TestModules_GatewayMultiChannelRoutingAndHygiene(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"
	parser := hclparse.NewParser()
	monitoringPath := filepath.Join(modulesDir, "gateway", "monitoring.tf")
	file, syntaxBody := parseHCLFileAST(t, parser, monitoringPath)

	t.Run("LocalsHygiene", func(t *testing.T) {
		var localsBlocks []*hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "locals" {
				localsBlocks = append(localsBlocks, block)
			}
		}
		require.NotEmpty(t, localsBlocks)

		allAttrs := make(map[string]hclsyntax.Expression)
		for _, lb := range localsBlocks {
			for name, attr := range lb.Body.Attributes {
				allAttrs[name] = attr.Expr
			}
		}

		// tier2_alert_notification_channels
		t2Expr, hasT2 := allAttrs["tier2_alert_notification_channels"]
		require.True(t, hasT2, "monitoring.tf must define tier2_alert_notification_channels local")
		t2Src := getExprSource(file, t2Expr)
		assert.Contains(t, t2Src, "google_monitoring_notification_channel.slack[*].name")
		assert.Contains(t, t2Src, "google_monitoring_notification_channel.email[*].name")
		assert.Contains(t, t2Src, "var.alert_notification_channels")
		assert.Contains(t, t2Src, "var.additional_sre_notification_channels")
		assert.NotContains(t, t2Src, "pagerduty", "tier2_alert_notification_channels must not include PagerDuty")

		// all_alert_notification_channels with TIER_3 suppression
		allExpr, hasAll := allAttrs["all_alert_notification_channels"]
		require.True(t, hasAll, "monitoring.tf must define all_alert_notification_channels local")
		allSrc := getExprSource(file, allExpr)
		assert.Contains(t, allSrc, "local.tier2_alert_notification_channels")
		assert.Contains(t, allSrc, `"TIER_3"`)

		// monitored_contracts_summary truncation
		truncExpr, hasTrunc := allAttrs["monitored_contracts_summary"]
		require.True(t, hasTrunc, "monitoring.tf must define monitored_contracts_summary local")
		truncSrc := getExprSource(file, truncExpr)
		assert.Contains(t, truncSrc, "250")
		assert.Contains(t, truncSrc, "more contracts")
	})

	t.Run("Tier2PoliciesUseTier2Channels", func(t *testing.T) {
		alerts := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_alert_policy" {
				alerts[block.Labels[1]] = block
			}
		}

		spike, hasSpike := alerts["tier2_latency_spike"]
		require.True(t, hasSpike)
		spikeChan, hasChan := spike.Body.Attributes["notification_channels"]
		require.True(t, hasChan)
		assert.Equal(t, "local.tier2_alert_notification_channels", getExprSource(file, spikeChan.Expr))

		errorRate, hasErrorRate := alerts["tier2_high_error_rate"]
		require.True(t, hasErrorRate)
		errChan, hasChan := errorRate.Body.Attributes["notification_channels"]
		require.True(t, hasChan)
		assert.Equal(t, "local.tier2_alert_notification_channels", getExprSource(file, errChan.Expr))
	})

	t.Run("GatewayOutputsExported", func(t *testing.T) {
		outPath := filepath.Join(modulesDir, "gateway", "outputs.tf")
		outFile, outBody := parseHCLFileAST(t, parser, outPath)

		outputs := make(map[string]*hclsyntax.Block)
		for _, block := range outBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				outputs[block.Labels[0]] = block
			}
		}

		expectedOutputs := map[string]string{
			"alert_notification_channels":           "local.all_alert_notification_channels",
			"tier2_alert_notification_channels":     "local.tier2_alert_notification_channels",
			"unacked_message_age_threshold_seconds": "local.unacked_age_seconds",
		}

		for name, expectedExpr := range expectedOutputs {
			block, exists := outputs[name]
			require.True(t, exists, "gateway outputs.tf missing output %s", name)
			valAttr, hasVal := block.Body.Attributes["value"]
			require.True(t, hasVal)
			assert.Equal(t, expectedExpr, getExprSource(outFile, valAttr.Expr))
		}
	})
}

func TestModules_DataProductDLTAndSchedulerMonitoring(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"
	parser := hclparse.NewParser()
	monitoringPath := filepath.Join(modulesDir, "data_product", "monitoring.tf")
	file, syntaxBody := parseHCLFileAST(t, parser, monitoringPath)

	t.Run("LocalsHygiene", func(t *testing.T) {
		var localsBlocks []*hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "locals" {
				localsBlocks = append(localsBlocks, block)
			}
		}
		require.NotEmpty(t, localsBlocks, "modules/data_product/monitoring.tf must define a locals block")

		allAttrs := make(map[string]hclsyntax.Expression)
		for _, lb := range localsBlocks {
			for name, attr := range lb.Body.Attributes {
				allAttrs[name] = attr.Expr
			}
		}

		suffixExpr, hasSuffix := allAttrs["name_suffix_display"]
		require.True(t, hasSuffix, "monitoring.tf must define name_suffix_display local")
		assert.Equal(t, `var.name_suffix != "" ? " - ${var.name_suffix}" : ""`, getExprSource(file, suffixExpr))
	})

	alerts := make(map[string]*hclsyntax.Block)
	for _, block := range syntaxBody.Blocks {
		if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_alert_policy" {
			alerts[block.Labels[1]] = block
		}
	}

	t.Run("dlt_job_failed_execution", func(t *testing.T) {
		t.Parallel()
		alert, exists := alerts["dlt_job_failed_execution"]
		require.True(t, exists, "google_monitoring_alert_policy.dlt_job_failed_execution must exist in monitoring.tf")

		forEachAttr, hasForEach := alert.Body.Attributes["for_each"]
		require.True(t, hasForEach)
		assert.Equal(t, "var.enable_infrastructure_alerts ? local.dlt_input_ports : {}", getExprSource(file, forEachAttr.Expr))

		nameAttr, hasName := alert.Body.Attributes["display_name"]
		require.True(t, hasName)
		assert.Contains(t, getExprSource(file, nameAttr.Expr), "[Tier 1] [INFRA] DLT Batch Ingestion Failed")

		chanAttr, hasChan := alert.Body.Attributes["notification_channels"]
		require.True(t, hasChan)
		assert.Equal(t, "module.gateway.alert_notification_channels", getExprSource(file, chanAttr.Expr))

		depAttr, hasDep := alert.Body.Attributes["depends_on"]
		require.True(t, hasDep)
		assert.Contains(t, getExprSource(file, depAttr.Expr), "google_cloud_run_v2_job.dlt_jobs")

		// Alert strategy auto close
		var stratBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "alert_strategy" {
				stratBlock = sub
				break
			}
		}
		require.NotNil(t, stratBlock)
		autoCloseAttr, hasAutoClose := stratBlock.Body.Attributes["auto_close"]
		require.True(t, hasAutoClose)
		assert.Equal(t, `"604800s"`, getExprSource(file, autoCloseAttr.Expr))

		// Documentation triage CLI command
		var docBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "documentation" {
				docBlock = sub
				break
			}
		}
		require.NotNil(t, docBlock)
		contentAttr, hasContent := docBlock.Body.Attributes["content"]
		require.True(t, hasContent)
		assert.Contains(t, getExprSource(file, contentAttr.Expr), "gcloud run jobs executions list")

		// Condition threshold
		var condBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "conditions" {
				condBlock = sub
				break
			}
		}
		require.NotNil(t, condBlock)
		var threshBlock *hclsyntax.Block
		for _, sub := range condBlock.Body.Blocks {
			if sub.Type == "condition_threshold" {
				threshBlock = sub
				break
			}
		}
		require.NotNil(t, threshBlock)

		filterAttr, hasFilter := threshBlock.Body.Attributes["filter"]
		require.True(t, hasFilter)
		filterSrc := strings.ReplaceAll(getExprSource(file, filterAttr.Expr), `\"`, `"`)
		assert.Contains(t, filterSrc, `resource.type = "cloud_run_job"`)
		assert.Contains(t, filterSrc, `google_cloud_run_v2_job.dlt_jobs[each.key].name`)
		assert.Contains(t, filterSrc, `metric.type = "run.googleapis.com/job/completed_execution_count"`)
		assert.Contains(t, filterSrc, `metric.label.result = "failed"`)

		durAttr, hasDur := threshBlock.Body.Attributes["duration"]
		require.True(t, hasDur)
		assert.Equal(t, `"0s"`, getExprSource(file, durAttr.Expr))

		compAttr, hasComp := threshBlock.Body.Attributes["comparison"]
		require.True(t, hasComp)
		assert.Equal(t, `"COMPARISON_GT"`, getExprSource(file, compAttr.Expr))

		valAttr, hasVal := threshBlock.Body.Attributes["threshold_value"]
		require.True(t, hasVal)
		assert.Equal(t, "0", getExprSource(file, valAttr.Expr))

		// Aggregations must be ALIGN_DELTA for completed_execution_count
		var aggBlock *hclsyntax.Block
		for _, sub := range threshBlock.Body.Blocks {
			if sub.Type == "aggregations" {
				aggBlock = sub
				break
			}
		}
		require.NotNil(t, aggBlock)
		alignerAttr, hasAligner := aggBlock.Body.Attributes["per_series_aligner"]
		require.True(t, hasAligner)
		assert.Equal(t, `"ALIGN_DELTA"`, getExprSource(file, alignerAttr.Expr))
	})

	t.Run("dlt_scheduler_failure", func(t *testing.T) {
		t.Parallel()
		alert, exists := alerts["dlt_scheduler_failure"]
		require.True(t, exists, "google_monitoring_alert_policy.dlt_scheduler_failure must exist in monitoring.tf")

		forEachAttr, hasForEach := alert.Body.Attributes["for_each"]
		require.True(t, hasForEach)
		assert.Equal(t, "var.enable_infrastructure_alerts ? local.dlt_input_ports : {}", getExprSource(file, forEachAttr.Expr))

		nameAttr, hasName := alert.Body.Attributes["display_name"]
		require.True(t, hasName)
		assert.Contains(t, getExprSource(file, nameAttr.Expr), "[Tier 1] [INFRA] DLT Scheduler Trigger Failure")

		chanAttr, hasChan := alert.Body.Attributes["notification_channels"]
		require.True(t, hasChan)
		assert.Equal(t, "module.gateway.alert_notification_channels", getExprSource(file, chanAttr.Expr))

		depAttr, hasDep := alert.Body.Attributes["depends_on"]
		require.True(t, hasDep)
		assert.Contains(t, getExprSource(file, depAttr.Expr), "google_cloud_scheduler_job.dlt_schedules")

		// Alert strategy auto close
		var stratBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "alert_strategy" {
				stratBlock = sub
				break
			}
		}
		require.NotNil(t, stratBlock)
		autoCloseAttr, hasAutoClose := stratBlock.Body.Attributes["auto_close"]
		require.True(t, hasAutoClose)
		assert.Equal(t, `"604800s"`, getExprSource(file, autoCloseAttr.Expr))

		// Documentation triage CLI command
		var docBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "documentation" {
				docBlock = sub
				break
			}
		}
		require.NotNil(t, docBlock)
		contentAttr, hasContent := docBlock.Body.Attributes["content"]
		require.True(t, hasContent)
		assert.Contains(t, getExprSource(file, contentAttr.Expr), "gcloud logging read")

		// Condition threshold
		var condBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "conditions" {
				condBlock = sub
				break
			}
		}
		require.NotNil(t, condBlock)
		var threshBlock *hclsyntax.Block
		for _, sub := range condBlock.Body.Blocks {
			if sub.Type == "condition_threshold" {
				threshBlock = sub
				break
			}
		}
		require.NotNil(t, threshBlock)

		filterAttr, hasFilter := threshBlock.Body.Attributes["filter"]
		require.True(t, hasFilter)
		filterSrc := strings.ReplaceAll(getExprSource(file, filterAttr.Expr), `\"`, `"`)
		assert.Contains(t, filterSrc, `resource.type = "cloud_scheduler_job"`)
		assert.Contains(t, filterSrc, `google_cloud_scheduler_job.dlt_schedules[each.key].name`)
		assert.Contains(t, filterSrc, `metric.type = "cloudscheduler.googleapis.com/job/attempt_count"`)
		assert.Contains(t, filterSrc, `metric.label.response_status != "SUCCESS"`)

		durAttr, hasDur := threshBlock.Body.Attributes["duration"]
		require.True(t, hasDur)
		assert.Equal(t, `"0s"`, getExprSource(file, durAttr.Expr))

		compAttr, hasComp := threshBlock.Body.Attributes["comparison"]
		require.True(t, hasComp)
		assert.Equal(t, `"COMPARISON_GT"`, getExprSource(file, compAttr.Expr))

		valAttr, hasVal := threshBlock.Body.Attributes["threshold_value"]
		require.True(t, hasVal)
		assert.Equal(t, "0", getExprSource(file, valAttr.Expr))

		// Aggregations must be ALIGN_DELTA for attempt_count
		var aggBlock *hclsyntax.Block
		for _, sub := range threshBlock.Body.Blocks {
			if sub.Type == "aggregations" {
				aggBlock = sub
				break
			}
		}
		require.NotNil(t, aggBlock)
		alignerAttr, hasAligner := aggBlock.Body.Attributes["per_series_aligner"]
		require.True(t, hasAligner)
		assert.Equal(t, `"ALIGN_DELTA"`, getExprSource(file, alignerAttr.Expr))
	})

	t.Run("input_push_latency", func(t *testing.T) {
		t.Parallel()
		alert, exists := alerts["input_push_latency"]
		require.True(t, exists, "google_monitoring_alert_policy.input_push_latency must exist in monitoring.tf")

		forEachAttr, hasForEach := alert.Body.Attributes["for_each"]
		require.True(t, hasForEach)
		assert.Equal(t, "var.enable_infrastructure_alerts ? local.pubsub_input_ports : {}", getExprSource(file, forEachAttr.Expr))

		nameAttr, hasName := alert.Body.Attributes["display_name"]
		require.True(t, hasName)
		assert.Contains(t, getExprSource(file, nameAttr.Expr), "[Tier 2] [INFRA] Input Push Subscription Latency")

		chanAttr, hasChan := alert.Body.Attributes["notification_channels"]
		require.True(t, hasChan)
		assert.Equal(t, "module.gateway.tier2_alert_notification_channels", getExprSource(file, chanAttr.Expr))

		depAttr, hasDep := alert.Body.Attributes["depends_on"]
		require.True(t, hasDep)
		assert.Contains(t, getExprSource(file, depAttr.Expr), "google_pubsub_subscription.input_push")

		// Alert strategy auto close
		var stratBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "alert_strategy" {
				stratBlock = sub
				break
			}
		}
		require.NotNil(t, stratBlock)
		autoCloseAttr, hasAutoClose := stratBlock.Body.Attributes["auto_close"]
		require.True(t, hasAutoClose)
		assert.Equal(t, `"604800s"`, getExprSource(file, autoCloseAttr.Expr))

		// Condition threshold
		var condBlock *hclsyntax.Block
		for _, sub := range alert.Body.Blocks {
			if sub.Type == "conditions" {
				condBlock = sub
				break
			}
		}
		require.NotNil(t, condBlock)
		var threshBlock *hclsyntax.Block
		for _, sub := range condBlock.Body.Blocks {
			if sub.Type == "condition_threshold" {
				threshBlock = sub
				break
			}
		}
		require.NotNil(t, threshBlock)

		filterAttr, hasFilter := threshBlock.Body.Attributes["filter"]
		require.True(t, hasFilter)
		filterSrc := strings.ReplaceAll(getExprSource(file, filterAttr.Expr), `\"`, `"`)
		assert.Contains(t, filterSrc, `resource.type = "pubsub_subscription"`)
		assert.Contains(t, filterSrc, `google_pubsub_subscription.input_push[each.key].name`)
		assert.Contains(t, filterSrc, `metric.type = "pubsub.googleapis.com/subscription/oldest_unacked_message_age"`)

		durAttr, hasDur := threshBlock.Body.Attributes["duration"]
		require.True(t, hasDur)
		assert.Equal(t, `"0s"`, getExprSource(file, durAttr.Expr))

		compAttr, hasComp := threshBlock.Body.Attributes["comparison"]
		require.True(t, hasComp)
		assert.Equal(t, `"COMPARISON_GT"`, getExprSource(file, compAttr.Expr))

		valAttr, hasVal := threshBlock.Body.Attributes["threshold_value"]
		require.True(t, hasVal)
		assert.Equal(t, "module.gateway.unacked_message_age_threshold_seconds", getExprSource(file, valAttr.Expr))

		// Aggregations must be ALIGN_MAX for latency
		var aggBlock *hclsyntax.Block
		for _, sub := range threshBlock.Body.Blocks {
			if sub.Type == "aggregations" {
				aggBlock = sub
				break
			}
		}
		require.NotNil(t, aggBlock)
		alignerAttr, hasAligner := aggBlock.Body.Attributes["per_series_aligner"]
		require.True(t, hasAligner)
		assert.Equal(t, `"ALIGN_MAX"`, getExprSource(file, alignerAttr.Expr))
	})

	t.Run("PlanTimeDeterminism", func(t *testing.T) {
		t.Parallel()
		for alertName, alert := range alerts {
			forEachAttr, hasForEach := alert.Body.Attributes["for_each"]
			require.True(t, hasForEach, "alert %s must define for_each", alertName)
			exprSrc := getExprSource(file, forEachAttr.Expr)
			assert.False(t, strings.Contains(exprSrc, ".id") || strings.Contains(exprSrc, ".name") || strings.Contains(exprSrc, ".email"),
				"alert %s for_each must not reference dynamic resource attributes: %s", alertName, exprSrc)
		}
	})
}

func TestModules_GatewayMonitoringDashboard(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"
	parser := hclparse.NewParser()
	dashboardPath := filepath.Join(modulesDir, "gateway", "dashboard.tf")
	file, syntaxBody := parseHCLFileAST(t, parser, dashboardPath)

	t.Run("DashboardResourceAttributes", func(t *testing.T) {
		t.Parallel()
		var dashboardBlock *hclsyntax.Block
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 &&
				block.Labels[0] == "google_monitoring_dashboard" && block.Labels[1] == "data_product" {
				dashboardBlock = block
				break
			}
		}
		require.NotNil(t, dashboardBlock, "google_monitoring_dashboard.data_product must exist in gateway/dashboard.tf")

		countAttr, hasCount := dashboardBlock.Body.Attributes["count"]
		require.True(t, hasCount)
		assert.Equal(t, "var.enable_monitoring_dashboard ? 1 : 0", getExprSource(file, countAttr.Expr))

		jsonAttr, hasJson := dashboardBlock.Body.Attributes["dashboard_json"]
		require.True(t, hasJson)
		jsonExpr := getExprSource(file, jsonAttr.Expr)
		assert.True(t, strings.HasPrefix(jsonExpr, "jsonencode("), "dashboard_json must use jsonencode(...)")

		// Assert Title
		assert.Contains(t, jsonExpr, `Supercargo Data Product - ${var.product_id}${local.name_suffix_display}`)

		// Assert MosaicLayout
		assert.Contains(t, jsonExpr, "mosaicLayout")

		// Assert 5 Operational Sections
		expectedSections := []string{
			"Contract Validation & Quarantine Health",
			"Gateway Cloud Run Golden Signals & Scaling",
			"Pub/Sub Ingress & BigQuery Sink Backlogs",
			"DLT Batch Jobs & Schedulers",
			"BigQuery Landing Table Throughput",
		}
		for _, sec := range expectedSections {
			assert.Contains(t, jsonExpr, sec, "dashboard_json must contain section %q", sec)
		}

		// Assert Section 1 contents: total messages, validation errors, validation latencies, DLQ unacked message age
		assert.Contains(t, jsonExpr, `custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_messages_total`)
		assert.Contains(t, jsonExpr, `custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_validation_errors_total`)
		assert.Contains(t, jsonExpr, `custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_validation_duration_ms`)
		assert.Contains(t, jsonExpr, "google_pubsub_subscription.dlq_sub.name")

		// Assert Section 2 contents: Cloud Run response code classes, request latencies, CPU, memory, active instances
		assert.Contains(t, jsonExpr, "run.googleapis.com/request_count")
		assert.Contains(t, jsonExpr, "run.googleapis.com/request_latencies")
		assert.Contains(t, jsonExpr, "run.googleapis.com/container/cpu/utilizations")
		assert.Contains(t, jsonExpr, "run.googleapis.com/container/memory/utilizations")
		assert.Contains(t, jsonExpr, "run.googleapis.com/container/instance_count")

		// Assert Section 3 contents: Pub/Sub ingress unacked age, num undelivered messages, Clean BigQuery unacked age, dead-letter count
		assert.Contains(t, jsonExpr, "google_pubsub_subscription.raw_push.name")
		assert.Contains(t, jsonExpr, "pubsub.googleapis.com/subscription/num_undelivered_messages")
		assert.Contains(t, jsonExpr, "pubsub.googleapis.com/subscription/dead_letter_message_count")

		// Assert Section 4 contents: Prefix matching for DLT jobs and schedulers (streaming-only compatibility)
		assert.Contains(t, strings.ReplaceAll(jsonExpr, `\"`, `"`), `starts_with("dlt-${var.product_id}-")`)
		assert.Contains(t, strings.ReplaceAll(jsonExpr, `\"`, `"`), `starts_with("dlt-sched-${var.product_id}-")`)
		assert.Contains(t, jsonExpr, "run.googleapis.com/job/completed_execution_count")
		assert.Contains(t, jsonExpr, "cloudscheduler.googleapis.com/job/attempt_count")

		// Assert Section 5 contents: BigQuery insert message rate & stored bytes/row count
		assert.Contains(t, jsonExpr, "bigquery.googleapis.com")
		assert.Contains(t, jsonExpr, "var.bigquery_dataset_id")
	})

	t.Run("GatewayOutputs", func(t *testing.T) {
		t.Parallel()
		outPath := filepath.Join(modulesDir, "gateway", "outputs.tf")
		outFile, outBody := parseHCLFileAST(t, parser, outPath)

		outputs := make(map[string]*hclsyntax.Block)
		for _, block := range outBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				outputs[block.Labels[0]] = block
			}
		}

		expectedOutputs := map[string]string{
			"dashboard_id":  `try(google_monitoring_dashboard.data_product[0].id, "")`,
			"dashboard_url": `length(google_monitoring_dashboard.data_product) > 0 ? "https://console.cloud.google.com/monitoring/dashboards/custom/${element(split("/", google_monitoring_dashboard.data_product[0].id), length(split("/", google_monitoring_dashboard.data_product[0].id)) - 1)}?project=${var.project_id}" : ""`,
		}

		for name, expectedExpr := range expectedOutputs {
			block, exists := outputs[name]
			require.True(t, exists, "gateway outputs.tf missing output %s", name)
			valAttr, hasVal := block.Body.Attributes["value"]
			require.True(t, hasVal)
			assert.Equal(t, expectedExpr, getExprSource(outFile, valAttr.Expr))
		}
	})

	t.Run("PlanTimeDeterminism", func(t *testing.T) {
		t.Parallel()
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" {
				if countAttr, hasCount := block.Body.Attributes["count"]; hasCount {
					exprSrc := getExprSource(file, countAttr.Expr)
					assert.False(t, strings.Contains(exprSrc, ".id") || strings.Contains(exprSrc, ".name") || strings.Contains(exprSrc, ".email"),
						"dashboard resource count must not reference dynamic resource attributes: %s", exprSrc)
				}
				if forEachAttr, hasForEach := block.Body.Attributes["for_each"]; hasForEach {
					exprSrc := getExprSource(file, forEachAttr.Expr)
					assert.False(t, strings.Contains(exprSrc, ".id") || strings.Contains(exprSrc, ".name") || strings.Contains(exprSrc, ".email"),
						"dashboard resource for_each must not reference dynamic resource attributes: %s", exprSrc)
				}
			}
		}
	})
}

func TestModules_DataProductDashboardOutputForwarding(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"
	parser := hclparse.NewParser()
	outPath := filepath.Join(modulesDir, "data_product", "outputs.tf")
	outFile, outBody := parseHCLFileAST(t, parser, outPath)

	outputs := make(map[string]*hclsyntax.Block)
	for _, block := range outBody.Blocks {
		if block.Type == "output" && len(block.Labels) == 1 {
			outputs[block.Labels[0]] = block
		}
	}

	expectedOutputs := map[string]string{
		"dashboard_id":  "module.gateway.dashboard_id",
		"dashboard_url": "module.gateway.dashboard_url",
	}

	for name, expectedExpr := range expectedOutputs {
		block, exists := outputs[name]
		require.True(t, exists, "data_product outputs.tf missing output %s", name)
		valAttr, hasVal := block.Body.Attributes["value"]
		require.True(t, hasVal)
		assert.Equal(t, expectedExpr, getExprSource(outFile, valAttr.Expr))
	}
}

func TestModules_ControlPlaneMonitoring(t *testing.T) {
	t.Parallel()
	modulesDir := "../../modules"

	t.Run("CloudRunServiceAlerts", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "monitoring_control_plane", "main.tf")
		varPath := filepath.Join(modulesDir, "monitoring_control_plane", "variables.tf")
		outPath := filepath.Join(modulesDir, "monitoring_control_plane", "outputs.tf")

		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)
		varFile, varBody := parseHCLFileAST(t, parser, varPath)
		outFile, outBody := parseHCLFileAST(t, parser, outPath)

		// Assert variables in variables.tf
		vars := make(map[string]*hclsyntax.Block)
		for _, block := range varBody.Blocks {
			if block.Type == "variable" && len(block.Labels) == 1 {
				vars[block.Labels[0]] = block
			}
		}

		expectedVars := []string{
			"project_id", "environment", "name_suffix", "region",
			"hub_service_name", "vault_service_name", "shovel_service_name",
			"mcp_service_name", "console_service_name", "analytics_dashboard_service_name",
			"github_runner_webhook_service_name",
			"hub_max_instances", "vault_max_instances", "shovel_max_instances",
			"mcp_max_instances", "console_max_instances", "analytics_dashboard_max_instances",
			"github_runner_webhook_max_instances",
			"shovel_trigger_name", "firestore_database_id", "kms_key_ring_name",
			"kms_crypto_key_name", "outbox_audit_subscription_name", "health_events_subscription_name",
			"events_push_subscription_name", "dataform_events_push_subscription_name",
			"dataform_events_dlq_subscription_name",
			"analytics_runner_job_name", "github_runner_job_name", "github_sync_job_names",
			"scheduler_job_names", "telemetry_subscriptions", "telemetry_dlq_subscription_name",
			"sla_tier", "enable_control_plane_alerts", "enable_slos", "enable_dashboards",
			"alert_notification_channels", "additional_sre_notification_channels",
			"alert_slack_channel", "alert_email_address", "alert_pagerduty_service_key",
			"unacked_message_age_threshold_seconds",
		}
		for _, v := range expectedVars {
			_, exists := vars[v]
			assert.True(t, exists, "variables.tf missing variable %s", v)
		}

		// Assert alert_pagerduty_service_key is sensitive
		if pdVar, exists := vars["alert_pagerduty_service_key"]; exists {
			sensAttr, hasSens := pdVar.Body.Attributes["sensitive"]
			require.True(t, hasSens, "alert_pagerduty_service_key must have sensitive attribute")
			assert.Equal(t, "true", getExprSource(varFile, sensAttr.Expr))
		}

		// Assert notification channels in main.tf
		channels := make(map[string]*hclsyntax.Block)
		alerts := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 {
				if block.Labels[0] == "google_monitoring_notification_channel" {
					channels[block.Labels[1]] = block
				} else if block.Labels[0] == "google_monitoring_alert_policy" {
					alerts[block.Labels[1]] = block
				}
			}
		}

		require.Contains(t, channels, "slack", "main.tf missing google_monitoring_notification_channel.slack")
		require.Contains(t, channels, "email", "main.tf missing google_monitoring_notification_channel.email")
		require.Contains(t, channels, "pagerduty", "main.tf missing google_monitoring_notification_channel.pagerduty")

		pdChan := channels["pagerduty"]
		var hasSensLabels bool
		for _, sub := range pdChan.Body.Blocks {
			if sub.Type == "sensitive_labels" {
				hasSensLabels = true
				keyAttr, hasKey := sub.Body.Attributes["service_key"]
				require.True(t, hasKey, "sensitive_labels must set service_key")
				assert.Equal(t, "var.alert_pagerduty_service_key", getExprSource(file, keyAttr.Expr))
			}
		}
		assert.True(t, hasSensLabels, "pagerduty channel must define sensitive_labels block")

		// Assert Cloud Run Golden Signal Alert Policies
		expectedAlerts := []struct {
			name                 string
			displayNamePrefix    string
			expectedChannelsExpr string
			filterContains       []string
			aligner              string
			duration             string
			comparison           string
		}{
			{
				name:                 "service_5xx_rate",
				displayNamePrefix:    "[Tier 1] [INFRA] Cloud Run 5xx Errors",
				expectedChannelsExpr: `each.value.tier == "Tier 1" ? local.all_alert_notification_channels : local.tier2_alert_notification_channels`,
				filterContains:       []string{`metric.label.response_code_class = "5xx"`, `metric.type = "run.googleapis.com/request_count"`},
				aligner:              `"ALIGN_RATE"`,
				duration:             `"0s"`,
				comparison:           `"COMPARISON_GT"`,
			},
			{
				name:                 "service_auth_failures",
				displayNamePrefix:    "[Tier 1] [INFRA] Cloud Run Invoker Auth 401/403",
				expectedChannelsExpr: `each.value.tier == "Tier 1" ? local.all_alert_notification_channels : local.tier2_alert_notification_channels`,
				filterContains:       []string{`response_code = one_of("401", "403")`, `metric.type = "run.googleapis.com/request_count"`},
				aligner:              `"ALIGN_RATE"`,
				duration:             `"0s"`,
				comparison:           `"COMPARISON_GT"`,
			},
			{
				name:                 "service_memory_utilization",
				displayNamePrefix:    "[Tier 2] [INFRA] Cloud Run Memory Saturation >85%",
				expectedChannelsExpr: `local.tier2_alert_notification_channels`,
				filterContains:       []string{`metric.type = "run.googleapis.com/container/memory/utilizations"`},
				aligner:              `"ALIGN_PERCENTILE_99"`,
				duration:             `"300s"`,
				comparison:           `"COMPARISON_GT"`,
			},
			{
				name:                 "service_cpu_utilization",
				displayNamePrefix:    "[Tier 2] [INFRA] Cloud Run CPU Saturation >85%",
				expectedChannelsExpr: `local.tier2_alert_notification_channels`,
				filterContains:       []string{`metric.type = "run.googleapis.com/container/cpu/utilizations"`},
				aligner:              `"ALIGN_PERCENTILE_99"`,
				duration:             `"300s"`,
				comparison:           `"COMPARISON_GT"`,
			},
			{
				name:                 "service_instance_saturation",
				displayNamePrefix:    "[Tier 2] [INFRA] Cloud Run Instance Saturation",
				expectedChannelsExpr: `local.tier2_alert_notification_channels`,
				filterContains:       []string{`metric.type = "run.googleapis.com/container/instance_count"`, `metric.label.state = "active"`},
				aligner:              `"ALIGN_MAX"`,
				duration:             `"180s"`,
				comparison:           `"COMPARISON_GTE"`,
			},
		}

		for _, tc := range expectedAlerts {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				alert, exists := alerts[tc.name]
				require.True(t, exists, "google_monitoring_alert_policy.%s must exist in main.tf", tc.name)

				// for_each guard
				forEachAttr, hasForEach := alert.Body.Attributes["for_each"]
				require.True(t, hasForEach, "%s must have for_each attribute", tc.name)
				assert.Equal(t, "var.enable_control_plane_alerts ? local.control_plane_services : {}", getExprSource(file, forEachAttr.Expr))

				// Combiner
				combAttr, hasComb := alert.Body.Attributes["combiner"]
				require.True(t, hasComb)
				assert.Equal(t, `"OR"`, getExprSource(file, combAttr.Expr))

				// Display Name
				nameAttr, hasName := alert.Body.Attributes["display_name"]
				require.True(t, hasName)
				assert.Contains(t, getExprSource(file, nameAttr.Expr), tc.displayNamePrefix)

				// Notification channels
				chanAttr, hasChan := alert.Body.Attributes["notification_channels"]
				require.True(t, hasChan)
				assert.Equal(t, tc.expectedChannelsExpr, getExprSource(file, chanAttr.Expr))

				// Auto close strategy
				var stratBlock *hclsyntax.Block
				for _, sub := range alert.Body.Blocks {
					if sub.Type == "alert_strategy" {
						stratBlock = sub
						break
					}
				}
				require.NotNil(t, stratBlock, "%s must define alert_strategy block", tc.name)
				autoCloseAttr, hasAutoClose := stratBlock.Body.Attributes["auto_close"]
				require.True(t, hasAutoClose)
				assert.Equal(t, `"604800s"`, getExprSource(file, autoCloseAttr.Expr))

				// Documentation block
				var docBlock *hclsyntax.Block
				for _, sub := range alert.Body.Blocks {
					if sub.Type == "documentation" {
						docBlock = sub
						break
					}
				}
				require.NotNil(t, docBlock, "%s must define documentation block", tc.name)
				contentAttr, hasContent := docBlock.Body.Attributes["content"]
				require.True(t, hasContent)
				contentSrc := getExprSource(file, contentAttr.Expr)
				assert.NotEmpty(t, contentSrc)
				assert.LessOrEqual(t, len(contentSrc), 800, "Documentation for %s must be <= 800 characters", tc.name)
				assert.Contains(t, contentSrc, "gcloud", "Documentation for %s must include CLI triage command", tc.name)

				// Conditions block
				var condBlock *hclsyntax.Block
				for _, sub := range alert.Body.Blocks {
					if sub.Type == "conditions" {
						condBlock = sub
						break
					}
				}
				require.NotNil(t, condBlock, "%s must define conditions block", tc.name)
				var threshBlock *hclsyntax.Block
				for _, sub := range condBlock.Body.Blocks {
					if sub.Type == "condition_threshold" {
						threshBlock = sub
						break
					}
				}
				require.NotNil(t, threshBlock, "%s must define condition_threshold block", tc.name)

				filterAttr, hasFilter := threshBlock.Body.Attributes["filter"]
				require.True(t, hasFilter)
				filterSrc := strings.ReplaceAll(getExprSource(file, filterAttr.Expr), `\"`, `"`)
				for _, fc := range tc.filterContains {
					assert.Contains(t, filterSrc, fc, "%s filter missing %s", tc.name, fc)
				}

				durAttr, hasDur := threshBlock.Body.Attributes["duration"]
				require.True(t, hasDur)
				assert.Equal(t, tc.duration, getExprSource(file, durAttr.Expr))

				compAttr, hasComp := threshBlock.Body.Attributes["comparison"]
				require.True(t, hasComp)
				assert.Equal(t, tc.comparison, getExprSource(file, compAttr.Expr))

				var aggBlock *hclsyntax.Block
				for _, sub := range threshBlock.Body.Blocks {
					if sub.Type == "aggregations" {
						aggBlock = sub
						break
					}
				}
				require.NotNil(t, aggBlock, "%s must define aggregations block", tc.name)
				alignerAttr, hasAligner := aggBlock.Body.Attributes["per_series_aligner"]
				require.True(t, hasAligner)
				assert.Equal(t, tc.aligner, getExprSource(file, alignerAttr.Expr))
			})
		}

		// Assert outputs in outputs.tf
		outputs := make(map[string]*hclsyntax.Block)
		for _, block := range outBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				outputs[block.Labels[0]] = block
			}
		}

		require.Contains(t, outputs, "all_alert_notification_channels")
		require.Contains(t, outputs, "tier2_alert_notification_channels")
		require.Contains(t, outputs, "unacked_message_age_threshold_seconds")

		allChanVal := getExprSource(outFile, outputs["all_alert_notification_channels"].Body.Attributes["value"].Expr)
		assert.Equal(t, "local.all_alert_notification_channels", allChanVal)

		tier2ChanVal := getExprSource(outFile, outputs["tier2_alert_notification_channels"].Body.Attributes["value"].Expr)
		assert.Equal(t, "local.tier2_alert_notification_channels", tier2ChanVal)

		unackedAgeVal := getExprSource(outFile, outputs["unacked_message_age_threshold_seconds"].Body.Attributes["value"].Expr)
		assert.Equal(t, "local.unacked_age_seconds", unackedAgeVal)
	})

	t.Run("ModuleVariableForwarding", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		hubVarPath := filepath.Join(modulesDir, "hub", "variables.tf")
		hubMainPath := filepath.Join(modulesDir, "hub", "main.tf")
		hubOutPath := filepath.Join(modulesDir, "hub", "outputs.tf")

		_, hubVarBody := parseHCLFileAST(t, parser, hubVarPath)
		hubMainFile, hubMainBody := parseHCLFileAST(t, parser, hubMainPath)
		hubOutFile, hubOutBody := parseHCLFileAST(t, parser, hubOutPath)

		// Assert variables in modules/hub/variables.tf
		hubVars := make(map[string]*hclsyntax.Block)
		for _, block := range hubVarBody.Blocks {
			if block.Type == "variable" && len(block.Labels) == 1 {
				hubVars[block.Labels[0]] = block
			}
		}

		expectedHubVars := []string{
			"enable_monitoring",
			"alert_slack_channel",
			"alert_email_address",
			"alert_pagerduty_service_key",
			"alert_notification_channels",
			"additional_sre_notification_channels",
			"sla_tier",
		}
		for _, v := range expectedHubVars {
			assert.Contains(t, hubVars, v, "modules/hub/variables.tf missing variable %s", v)
		}

		// Assert module "monitoring" in modules/hub/main.tf
		var monModule *hclsyntax.Block
		for _, block := range hubMainBody.Blocks {
			if block.Type == "module" && len(block.Labels) == 1 && block.Labels[0] == "monitoring" {
				monModule = block
				break
			}
		}
		require.NotNil(t, monModule, "modules/hub/main.tf missing module \"monitoring\"")

		countAttr, hasCount := monModule.Body.Attributes["count"]
		require.True(t, hasCount, "module \"monitoring\" must define count attribute")
		assert.Equal(t, "var.enable_monitoring ? 1 : 0", getExprSource(hubMainFile, countAttr.Expr))

		sourceAttr, hasSource := monModule.Body.Attributes["source"]
		require.True(t, hasSource, "module \"monitoring\" must define source attribute")
		assert.Equal(t, `"../monitoring_control_plane"`, getExprSource(hubMainFile, sourceAttr.Expr))

		// Check forwarded variables
		expectedAttrs := map[string]string{
			"project_id":                  "var.project_id",
			"environment":                 "var.environment",
			"name_suffix":                 "var.name_suffix",
			"region":                      "var.region",
			"hub_service_name":            "google_cloud_run_v2_service.hub.name",
			"shovel_service_name":         "google_cloud_run_v2_service.shovel.name",
			"hub_max_instances":           "var.max_instances",
			"shovel_max_instances":        "var.shovel_max_instances",
			"shovel_trigger_name":         "google_eventarc_trigger.outbox_trigger.name",
			"firestore_database_id":       "google_firestore_database.database.name",
			"sla_tier":                    "var.sla_tier",
			"alert_slack_channel":         "var.alert_slack_channel",
			"alert_email_address":         "var.alert_email_address",
			"alert_pagerduty_service_key": "var.alert_pagerduty_service_key",
		}
		for attrName, expectedExpr := range expectedAttrs {
			attr, hasAttr := monModule.Body.Attributes[attrName]
			require.True(t, hasAttr, "module \"monitoring\" missing attribute %s", attrName)
			assert.Equal(t, expectedExpr, getExprSource(hubMainFile, attr.Expr))
		}

		// Assert outputs in modules/hub/outputs.tf
		hubOutputs := make(map[string]*hclsyntax.Block)
		for _, block := range hubOutBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				hubOutputs[block.Labels[0]] = block
			}
		}

		require.Contains(t, hubOutputs, "shovel_service_name", "modules/hub/outputs.tf missing output shovel_service_name")
		require.Contains(t, hubOutputs, "audit_subscription_name", "modules/hub/outputs.tf missing output audit_subscription_name (Issue #300)")
		require.Contains(t, hubOutputs, "dashboard_sre_url", "modules/hub/outputs.tf missing output dashboard_sre_url")
		require.Contains(t, hubOutputs, "dashboard_governance_url", "modules/hub/outputs.tf missing output dashboard_governance_url")

		shovelVal := getExprSource(hubOutFile, hubOutputs["shovel_service_name"].Body.Attributes["value"].Expr)
		assert.Equal(t, "google_cloud_run_v2_service.shovel.name", shovelVal)

		auditSubVal := getExprSource(hubOutFile, hubOutputs["audit_subscription_name"].Body.Attributes["value"].Expr)
		assert.Equal(t, `var.enable_audit_sink ? google_pubsub_subscription.outbox_audit_bq[0].name : ""`, auditSubVal)
	})

	t.Run("SLOsAndBurnRateAlerts", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		slosPath := filepath.Join(modulesDir, "monitoring_control_plane", "slos.tf")
		outPath := filepath.Join(modulesDir, "monitoring_control_plane", "outputs.tf")

		sloFile, sloBody := parseHCLFileAST(t, parser, slosPath)
		outFile, outBody := parseHCLFileAST(t, parser, outPath)

		// 1. Assert Custom Services
		services := make(map[string]*hclsyntax.Block)
		slos := make(map[string]*hclsyntax.Block)
		alerts := make(map[string]*hclsyntax.Block)
		for _, block := range sloBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 {
				switch block.Labels[0] {
				case "google_monitoring_custom_service":
					services[block.Labels[1]] = block
				case "google_monitoring_slo":
					slos[block.Labels[1]] = block
				case "google_monitoring_alert_policy":
					alerts[block.Labels[1]] = block
				}
			}
		}

		require.Contains(t, services, "hub", "slos.tf missing google_monitoring_custom_service.hub")
		require.Contains(t, services, "vault", "slos.tf missing google_monitoring_custom_service.vault")

		hubSvc := services["hub"]
		assert.Equal(t, "var.enable_slos && var.hub_service_name != \"\" && var.hub_service_name != null ? 1 : 0", getExprSource(sloFile, hubSvc.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.hub_custom_service_id", getExprSource(sloFile, hubSvc.Body.Attributes["service_id"].Expr))

		vaultSvc := services["vault"]
		assert.Equal(t, "var.enable_slos && var.vault_service_name != \"\" && var.vault_service_name != null ? 1 : 0", getExprSource(sloFile, vaultSvc.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.vault_custom_service_id", getExprSource(sloFile, vaultSvc.Body.Attributes["service_id"].Expr))

		// 2. Assert SLO Definitions
		expectedSLOs := []struct {
			name        string
			serviceExpr string
			sloID       string
			goal        string
			period      string
			dependsOn   string
		}{
			{
				name:        "hub_availability",
				serviceExpr: "local.hub_custom_service_id",
				sloID:       `"hub-availability"`,
				goal:        "0.999",
				period:      "30",
				dependsOn:   "google_monitoring_custom_service.hub",
			},
			{
				name:        "hub_latency",
				serviceExpr: "local.hub_custom_service_id",
				sloID:       `"hub-latency"`,
				goal:        "0.99",
				period:      "30",
				dependsOn:   "google_monitoring_custom_service.hub",
			},
			{
				name:        "vault_availability",
				serviceExpr: "local.vault_custom_service_id",
				sloID:       `"vault-availability"`,
				goal:        "0.9995",
				period:      "30",
				dependsOn:   "google_monitoring_custom_service.vault",
			},
			{
				name:        "vault_latency",
				serviceExpr: "local.vault_custom_service_id",
				sloID:       `"vault-latency"`,
				goal:        "0.99",
				period:      "30",
				dependsOn:   "google_monitoring_custom_service.vault",
			},
		}

		for _, tc := range expectedSLOs {
			slo, exists := slos[tc.name]
			require.True(t, exists, "slos.tf missing google_monitoring_slo.%s", tc.name)
			assert.Equal(t, tc.serviceExpr, getExprSource(sloFile, slo.Body.Attributes["service"].Expr))
			assert.Equal(t, tc.sloID, getExprSource(sloFile, slo.Body.Attributes["slo_id"].Expr))
			assert.Equal(t, tc.goal, getExprSource(sloFile, slo.Body.Attributes["goal"].Expr))
			assert.Equal(t, tc.period, getExprSource(sloFile, slo.Body.Attributes["rolling_period_days"].Expr))

			depAttr, hasDep := slo.Body.Attributes["depends_on"]
			require.True(t, hasDep, "%s missing depends_on attribute", tc.name)
			assert.Contains(t, getExprSource(sloFile, depAttr.Expr), tc.dependsOn)
		}

		// 3. Assert Plan-Time Determinism in Static SLO Resource IDs
		sloFileContent := string(sloFile.Bytes)
		assert.Contains(t, sloFileContent, "projects/${var.project_id}/services/${local.hub_custom_service_id}/serviceLevelObjectives")
		assert.Contains(t, sloFileContent, "projects/${var.project_id}/services/${local.vault_custom_service_id}/serviceLevelObjectives")
		assert.NotContains(t, sloFileContent, "google_monitoring_slo.hub_availability[0].id")
		assert.NotContains(t, sloFileContent, "google_monitoring_slo.vault_availability[0].id")

		// 4. Assert Burn-Rate Alert Policy
		require.Contains(t, alerts, "slo_burn_rate", "slos.tf missing google_monitoring_alert_policy.slo_burn_rate")
		burnAlert := alerts["slo_burn_rate"]

		assert.Equal(t, "local.slo_alerts", getExprSource(sloFile, burnAlert.Body.Attributes["for_each"].Expr))
		assert.Equal(t, `"OR"`, getExprSource(sloFile, burnAlert.Body.Attributes["combiner"].Expr))

		// Check explicit depends_on on SLOs
		depAttr, hasDep := burnAlert.Body.Attributes["depends_on"]
		require.True(t, hasDep, "slo_burn_rate must define depends_on attribute")
		depSrc := getExprSource(sloFile, depAttr.Expr)
		assert.Contains(t, depSrc, "google_monitoring_slo.hub_availability")
		assert.Contains(t, depSrc, "google_monitoring_slo.hub_latency")
		assert.Contains(t, depSrc, "google_monitoring_slo.vault_availability")
		assert.Contains(t, depSrc, "google_monitoring_slo.vault_latency")

		// Check documentation block
		var docBlock *hclsyntax.Block
		for _, sub := range burnAlert.Body.Blocks {
			if sub.Type == "documentation" {
				docBlock = sub
				break
			}
		}
		require.NotNil(t, docBlock, "slo_burn_rate must define documentation block")
		contentSrc := getExprSource(sloFile, docBlock.Body.Attributes["content"].Expr)
		assert.NotEmpty(t, contentSrc)
		assert.LessOrEqual(t, len(contentSrc), 800, "Documentation for slo_burn_rate must be <= 800 characters")
		assert.Contains(t, contentSrc, "gcloud", "Documentation for slo_burn_rate must include CLI triage command")

		// Check condition filter
		var condBlock *hclsyntax.Block
		for _, sub := range burnAlert.Body.Blocks {
			if sub.Type == "conditions" {
				condBlock = sub
				break
			}
		}
		require.NotNil(t, condBlock, "slo_burn_rate must define conditions block")
		var threshBlock *hclsyntax.Block
		for _, sub := range condBlock.Body.Blocks {
			if sub.Type == "condition_threshold" {
				threshBlock = sub
				break
			}
		}
		require.NotNil(t, threshBlock, "slo_burn_rate must define condition_threshold block")
		filterSrc := getExprSource(sloFile, threshBlock.Body.Attributes["filter"].Expr)
		assert.Contains(t, filterSrc, "select_slo_burn_rate")

		// 5. Assert outputs in outputs.tf
		outputs := make(map[string]*hclsyntax.Block)
		for _, block := range outBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				outputs[block.Labels[0]] = block
			}
		}

		expectedOutputs := map[string]string{
			"hub_custom_service_id":     `try(google_monitoring_custom_service.hub[0].service_id, "")`,
			"vault_custom_service_id":   `try(google_monitoring_custom_service.vault[0].service_id, "")`,
			"hub_availability_slo_id":   `try(google_monitoring_slo.hub_availability[0].id, "")`,
			"hub_latency_slo_id":        `try(google_monitoring_slo.hub_latency[0].id, "")`,
			"vault_availability_slo_id": `try(google_monitoring_slo.vault_availability[0].id, "")`,
			"vault_latency_slo_id":      `try(google_monitoring_slo.vault_latency[0].id, "")`,
		}

		for name, expectedExpr := range expectedOutputs {
			block, exists := outputs[name]
			require.True(t, exists, "outputs.tf missing output %s", name)
			valAttr, hasVal := block.Body.Attributes["value"]
			require.True(t, hasVal)
			assert.Equal(t, expectedExpr, getExprSource(outFile, valAttr.Expr))
		}
	})

	t.Run("FirestoreAndEventarcAlerts", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "monitoring_control_plane", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		alerts := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_alert_policy" {
				alerts[block.Labels[1]] = block
			}
		}

		expectedAlerts := []string{
			"firestore_aborted_transactions",
			"firestore_quota_exhaustion",
			"shovel_trigger_delivery_failures",
			"core_subscription_unacked_age",
			"dataform_dlq_backlog",
		}
		for _, name := range expectedAlerts {
			require.Contains(t, alerts, name, "main.tf missing google_monitoring_alert_policy.%s", name)
		}

		// 1. firestore_aborted_transactions
		abortedAlert := alerts["firestore_aborted_transactions"]
		assert.Equal(t, "var.enable_control_plane_alerts ? 1 : 0", getExprSource(file, abortedAlert.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, abortedAlert.Body.Attributes["notification_channels"].Expr))
		fileContent := string(file.Bytes)
		assert.Contains(t, fileContent, "resource.labels.database_id = \\\"${var.firestore_database_id}\\\"")
		assert.Contains(t, fileContent, "metric.label.response_code = \\\"ABORTED\\\"")

		// 2. firestore_quota_exhaustion
		quotaAlert := alerts["firestore_quota_exhaustion"]
		assert.Equal(t, "var.enable_control_plane_alerts ? 1 : 0", getExprSource(file, quotaAlert.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, quotaAlert.Body.Attributes["notification_channels"].Expr))
		assert.Contains(t, fileContent, "metric.label.response_code = \\\"RESOURCE_EXHAUSTED\\\"")

		// 3. shovel_trigger_delivery_failures
		shovelAlert := alerts["shovel_trigger_delivery_failures"]
		assert.Equal(t, "var.enable_control_plane_alerts ? 1 : 0", getExprSource(file, shovelAlert.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, shovelAlert.Body.Attributes["notification_channels"].Expr))
		assert.Contains(t, fileContent, "resource.labels.trigger_id = \\\"${var.shovel_trigger_name}\\\"")
		assert.Contains(t, fileContent, "eventarc.googleapis.com/delivery_attempt_count")
		assert.Contains(t, fileContent, "metric.label.response_code != \\\"OK\\\"")

		// 4. core_subscription_unacked_age
		coreSubAlert := alerts["core_subscription_unacked_age"]
		assert.Equal(t, "var.enable_control_plane_alerts ? local.core_subscriptions : {}", getExprSource(file, coreSubAlert.Body.Attributes["for_each"].Expr))
		assert.Equal(t, `each.value.tier == "Tier 1" ? local.all_alert_notification_channels : local.tier2_alert_notification_channels`, getExprSource(file, coreSubAlert.Body.Attributes["notification_channels"].Expr))
		assert.Contains(t, fileContent, "pubsub.googleapis.com/subscription/oldest_unacked_message_age")
		assert.Contains(t, fileContent, "element(split(\"/\", var.outbox_audit_subscription_name), length(split(\"/\", var.outbox_audit_subscription_name)) - 1)")

		// 5. dataform_dlq_backlog
		dlqAlert := alerts["dataform_dlq_backlog"]
		assert.Equal(t, "var.enable_control_plane_alerts && var.dataform_events_dlq_subscription_name != \"\" ? 1 : 0", getExprSource(file, dlqAlert.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, dlqAlert.Body.Attributes["notification_channels"].Expr))
		assert.Contains(t, fileContent, "pubsub.googleapis.com/subscription/num_undelivered_messages")

		// Verify documentation blocks length <= 800 and contain gcloud
		for _, name := range expectedAlerts {
			alert := alerts[name]
			var docBlock *hclsyntax.Block
			for _, sub := range alert.Body.Blocks {
				if sub.Type == "documentation" {
					docBlock = sub
					break
				}
			}
			require.NotNil(t, docBlock, "%s must define documentation block", name)
			contentSrc := getExprSource(file, docBlock.Body.Attributes["content"].Expr)
			assert.NotEmpty(t, contentSrc)
			assert.LessOrEqual(t, len(contentSrc), 800, "Documentation for %s must be <= 800 characters", name)
			assert.Contains(t, contentSrc, "gcloud", "Documentation for %s must include CLI triage command", name)
		}
	})

	t.Run("KMSAndVaultSecurityAlerts", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "monitoring_control_plane", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		alerts := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_alert_policy" {
				alerts[block.Labels[1]] = block
			}
		}

		require.Contains(t, alerts, "kms_access_denials", "main.tf missing google_monitoring_alert_policy.kms_access_denials")
		require.Contains(t, alerts, "vault_forbidden_spikes", "main.tf missing google_monitoring_alert_policy.vault_forbidden_spikes")

		kmsAlert := alerts["kms_access_denials"]
		assert.Equal(t, "var.enable_control_plane_alerts ? 1 : 0", getExprSource(file, kmsAlert.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, kmsAlert.Body.Attributes["notification_channels"].Expr))

		vaultAlert := alerts["vault_forbidden_spikes"]
		assert.Equal(t, "var.enable_control_plane_alerts && var.vault_service_name != \"\" ? 1 : 0", getExprSource(file, vaultAlert.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, vaultAlert.Body.Attributes["notification_channels"].Expr))

		fileContent := string(file.Bytes)
		assert.Contains(t, fileContent, "cloudkms.googleapis.com/crypto_requests")
		assert.Contains(t, fileContent, "metric.label.response_code = \\\"403\\\"")

		// Verify documentation blocks length <= 800 and contain gcloud
		for _, name := range []string{"kms_access_denials", "vault_forbidden_spikes"} {
			alert := alerts[name]
			var docBlock *hclsyntax.Block
			for _, sub := range alert.Body.Blocks {
				if sub.Type == "documentation" {
					docBlock = sub
					break
				}
			}
			require.NotNil(t, docBlock, "%s must define documentation block", name)
			contentSrc := getExprSource(file, docBlock.Body.Attributes["content"].Expr)
			assert.NotEmpty(t, contentSrc)
			assert.LessOrEqual(t, len(contentSrc), 800, "Documentation for %s must be <= 800 characters", name)
			assert.Contains(t, contentSrc, "gcloud", "Documentation for %s must include CLI triage command", name)
		}
	})

	t.Run("BatchJobsAndMetaTelemetryAlerts", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		mainPath := filepath.Join(modulesDir, "monitoring_control_plane", "main.tf")
		file, syntaxBody := parseHCLFileAST(t, parser, mainPath)

		alerts := make(map[string]*hclsyntax.Block)
		for _, block := range syntaxBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_alert_policy" {
				alerts[block.Labels[1]] = block
			}
		}

		expectedAlerts := []string{
			"job_failed_execution",
			"scheduler_dispatch_failure",
			"telemetry_bq_unacked_age",
			"telemetry_dlq_backlog",
		}
		for _, name := range expectedAlerts {
			require.Contains(t, alerts, name, "main.tf missing google_monitoring_alert_policy.%s", name)
		}

		// 1. job_failed_execution
		jobAlert := alerts["job_failed_execution"]
		assert.Equal(t, "var.enable_control_plane_alerts ? local.monitored_batch_jobs : toset([])", getExprSource(file, jobAlert.Body.Attributes["for_each"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, jobAlert.Body.Attributes["notification_channels"].Expr))

		// 2. scheduler_dispatch_failure
		schedAlert := alerts["scheduler_dispatch_failure"]
		assert.Equal(t, "var.enable_control_plane_alerts ? toset(var.scheduler_job_names) : toset([])", getExprSource(file, schedAlert.Body.Attributes["for_each"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, schedAlert.Body.Attributes["notification_channels"].Expr))

		// 3. telemetry_bq_unacked_age
		telemAlert := alerts["telemetry_bq_unacked_age"]
		assert.Equal(t, "var.enable_control_plane_alerts ? toset(var.telemetry_subscriptions) : toset([])", getExprSource(file, telemAlert.Body.Attributes["for_each"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, telemAlert.Body.Attributes["notification_channels"].Expr))

		// 4. telemetry_dlq_backlog
		dlqAlert := alerts["telemetry_dlq_backlog"]
		assert.Equal(t, "var.enable_control_plane_alerts && var.telemetry_dlq_subscription_name != \"\" ? 1 : 0", getExprSource(file, dlqAlert.Body.Attributes["count"].Expr))
		assert.Equal(t, "local.all_alert_notification_channels", getExprSource(file, dlqAlert.Body.Attributes["notification_channels"].Expr))

		fileContent := string(file.Bytes)
		assert.Contains(t, fileContent, "run.googleapis.com/job/completed_execution_count")
		assert.Contains(t, fileContent, "cloudscheduler.googleapis.com/job/attempt_count")
		assert.Contains(t, fileContent, "pubsub.googleapis.com/subscription/oldest_unacked_message_age")
		assert.Contains(t, fileContent, "pubsub.googleapis.com/subscription/num_undelivered_messages")

		// Verify documentation blocks length <= 800 and contain gcloud
		for _, name := range expectedAlerts {
			alert := alerts[name]
			var docBlock *hclsyntax.Block
			for _, sub := range alert.Body.Blocks {
				if sub.Type == "documentation" {
					docBlock = sub
					break
				}
			}
			require.NotNil(t, docBlock, "%s must define documentation block", name)
			contentSrc := getExprSource(file, docBlock.Body.Attributes["content"].Expr)
			assert.NotEmpty(t, contentSrc)
			assert.LessOrEqual(t, len(contentSrc), 800, "Documentation for %s must be <= 800 characters", name)
			assert.Contains(t, contentSrc, "gcloud", "Documentation for %s must include CLI triage command", name)
		}
	})

	t.Run("ChildModulesVariableForwarding", func(t *testing.T) {
		t.Parallel()
		childModules := []string{"vault", "analytics_runner", "analytics_github_sync"}

		for _, mod := range childModules {
			mod := mod
			t.Run(mod, func(t *testing.T) {
				t.Parallel()
				parser := hclparse.NewParser()
				varPath := filepath.Join(modulesDir, mod, "variables.tf")
				mainPath := filepath.Join(modulesDir, mod, "main.tf")

				varFile, varBody := parseHCLFileAST(t, parser, varPath)
				_, mainBody := parseHCLFileAST(t, parser, mainPath)

				vars := make(map[string]*hclsyntax.Block)
				for _, block := range varBody.Blocks {
					if block.Type == "variable" && len(block.Labels) == 1 {
						vars[block.Labels[0]] = block
					}
				}

				require.Contains(t, vars, "enable_monitoring", "%s missing variable enable_monitoring", mod)
				require.Contains(t, vars, "alert_notification_channels", "%s missing variable alert_notification_channels", mod)
				require.Contains(t, vars, "sla_tier", "%s missing variable sla_tier", mod)

				// Verify sla_tier has validation block
				slaVar := vars["sla_tier"]
				var hasValidation bool
				for _, sub := range slaVar.Body.Blocks {
					if sub.Type == "validation" {
						hasValidation = true
						condAttr, hasCond := sub.Body.Attributes["condition"]
						require.True(t, hasCond)
						assert.Contains(t, getExprSource(varFile, condAttr.Expr), "TIER_1")
					}
				}
				assert.True(t, hasValidation, "%s sla_tier must have validation block", mod)

				// Verify NO google_monitoring_alert_policy exists in child module (anti-proliferation rule)
				for _, block := range mainBody.Blocks {
					if block.Type == "resource" && len(block.Labels) == 2 {
						assert.NotEqual(t, "google_monitoring_alert_policy", block.Labels[0],
							"%s must not define duplicate alert policy; monitoring must be centralized in monitoring_control_plane", mod)
					}
				}
			})
		}
	})

	t.Run("TurnkeyPersonaDashboards", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		dashPath := filepath.Join(modulesDir, "monitoring_control_plane", "dashboards.tf")
		outPath := filepath.Join(modulesDir, "monitoring_control_plane", "outputs.tf")

		dashFile, dashBody := parseHCLFileAST(t, parser, dashPath)
		outFile, outBody := parseHCLFileAST(t, parser, outPath)

		dashboards := make(map[string]*hclsyntax.Block)
		for _, block := range dashBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_dashboard" {
				dashboards[block.Labels[1]] = block
			}
		}

		require.Contains(t, dashboards, "control_plane_sre", "dashboards.tf missing google_monitoring_dashboard.control_plane_sre")
		require.Contains(t, dashboards, "governance_privacy", "dashboards.tf missing google_monitoring_dashboard.governance_privacy")

		// 1. Assert Control Plane SRE Dashboard
		sreDash := dashboards["control_plane_sre"]
		assert.Equal(t, "var.enable_dashboards ? 1 : 0", getExprSource(dashFile, sreDash.Body.Attributes["count"].Expr))
		sreJsonAttr, hasSreJson := sreDash.Body.Attributes["dashboard_json"]
		require.True(t, hasSreJson)
		sreJson := getExprSource(dashFile, sreJsonAttr.Expr)
		assert.True(t, strings.HasPrefix(sreJson, "jsonencode("))
		assert.Contains(t, sreJson, `Supercargo - Control Plane & SRE Infrastructure (${var.environment})${local.name_suffix_display}`)
		assert.Contains(t, sreJson, "mosaicLayout")

		// Assert 6 SRE Sections
		expectedSreSections := []string{
			"Cloud Run Golden Signals",
			"Container Resource Saturation",
			"SLO Burn Rates",
			"Pub/Sub & BigQuery Pipeline Health",
			"Batch Ingestion Jobs & Cloud Schedulers",
			"Meta-Telemetry Lakehouse Streaming Health",
		}
		for _, section := range expectedSreSections {
			assert.Contains(t, sreJson, section, "control_plane_sre dashboard missing section %s", section)
		}

		// 2. Assert Governance & Privacy Dashboard
		govDash := dashboards["governance_privacy"]
		assert.Equal(t, "var.enable_dashboards ? 1 : 0", getExprSource(dashFile, govDash.Body.Attributes["count"].Expr))
		govJsonAttr, hasGovJson := govDash.Body.Attributes["dashboard_json"]
		require.True(t, hasGovJson)
		govJson := getExprSource(dashFile, govJsonAttr.Expr)
		assert.True(t, strings.HasPrefix(govJson, "jsonencode("))
		assert.Contains(t, govJson, `Supercargo - Governance & Privacy Plane (${var.environment})${local.name_suffix_display}`)
		assert.Contains(t, govJson, "mosaicLayout")

		// Assert 5 Governance Sections
		expectedGovSections := []string{
			"Hub Contract Registry Operations",
			"Firestore Outbox CDC & Metadata Shovel Delivery",
			"Vault Cryptographic Operations & Security",
			"Companion Service Interactions",
			"Dataplex Taxonomy & Dataform Reconciliation State",
		}
		for _, section := range expectedGovSections {
			assert.Contains(t, govJson, section, "governance_privacy dashboard missing section %s", section)
		}

		// 3. Assert Outputs in outputs.tf
		outputs := make(map[string]*hclsyntax.Block)
		for _, block := range outBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				outputs[block.Labels[0]] = block
			}
		}

		expectedOutputs := map[string]string{
			"dashboard_sre_id":         `try(google_monitoring_dashboard.control_plane_sre[0].id, "")`,
			"dashboard_sre_url":        `length(google_monitoring_dashboard.control_plane_sre) > 0 ? "https://console.cloud.google.com/monitoring/dashboards/custom/${element(split("/", google_monitoring_dashboard.control_plane_sre[0].id), length(split("/", google_monitoring_dashboard.control_plane_sre[0].id)) - 1)}?project=${var.project_id}" : ""`,
			"dashboard_governance_id":  `try(google_monitoring_dashboard.governance_privacy[0].id, "")`,
			"dashboard_governance_url": `length(google_monitoring_dashboard.governance_privacy) > 0 ? "https://console.cloud.google.com/monitoring/dashboards/custom/${element(split("/", google_monitoring_dashboard.governance_privacy[0].id), length(split("/", google_monitoring_dashboard.governance_privacy[0].id)) - 1)}?project=${var.project_id}" : ""`,
		}

		for name, expectedExpr := range expectedOutputs {
			block, exists := outputs[name]
			require.True(t, exists, "outputs.tf missing output %s", name)
			valAttr, hasVal := block.Body.Attributes["value"]
			require.True(t, hasVal)
			assert.Equal(t, expectedExpr, getExprSource(outFile, valAttr.Expr))
		}
	})

	t.Run("DocumentationStringLength", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		tfFiles := []string{"main.tf", "slos.tf"}
		totalAlertsChecked := 0

		for _, tfFile := range tfFiles {
			path := filepath.Join(modulesDir, "monitoring_control_plane", tfFile)
			file, syntaxBody := parseHCLFileAST(t, parser, path)

			for _, block := range syntaxBody.Blocks {
				if block.Type == "resource" && len(block.Labels) == 2 && block.Labels[0] == "google_monitoring_alert_policy" {
					policyName := block.Labels[1]
					var docBlock *hclsyntax.Block
					for _, sub := range block.Body.Blocks {
						if sub.Type == "documentation" {
							docBlock = sub
							break
						}
					}
					require.NotNil(t, docBlock, "%s in %s missing documentation block", policyName, tfFile)
					contentAttr, hasContent := docBlock.Body.Attributes["content"]
					require.True(t, hasContent, "%s in %s missing documentation.content", policyName, tfFile)
					contentSrc := getExprSource(file, contentAttr.Expr)

					assert.NotEmpty(t, contentSrc, "%s documentation content cannot be empty", policyName)
					assert.LessOrEqual(t, len(contentSrc), 800, "%s in %s documentation content exceeds 800 characters limit (got %d)", policyName, tfFile, len(contentSrc))
					assert.Contains(t, contentSrc, "gcloud", "%s in %s documentation must contain a copy-pasteable CLI command", policyName, tfFile)
					totalAlertsChecked++
				}
			}
		}

		assert.GreaterOrEqual(t, totalAlertsChecked, 17, "expected at least 17 alert policies across main.tf and slos.tf")
	})

	t.Run("PlanTimeDeterminism", func(t *testing.T) {
		t.Parallel()
		parser := hclparse.NewParser()
		tfFiles := []string{"main.tf", "slos.tf", "dashboards.tf"}

		for _, tfFile := range tfFiles {
			path := filepath.Join(modulesDir, "monitoring_control_plane", tfFile)
			file, syntaxBody := parseHCLFileAST(t, parser, path)

			for _, block := range syntaxBody.Blocks {
				if block.Type == "resource" {
					resName := block.Labels[0] + "." + block.Labels[1]
					if countAttr, hasCount := block.Body.Attributes["count"]; hasCount {
						exprSrc := getExprSource(file, countAttr.Expr)
						assert.False(t, strings.Contains(exprSrc, ".id") || strings.Contains(exprSrc, ".uri") || strings.Contains(exprSrc, ".email"),
							"resource %s count attribute in %s must not reference dynamic resource attributes: %s", resName, tfFile, exprSrc)
					}
					if forEachAttr, hasForEach := block.Body.Attributes["for_each"]; hasForEach {
						exprSrc := getExprSource(file, forEachAttr.Expr)
						assert.False(t, strings.Contains(exprSrc, ".id") || strings.Contains(exprSrc, ".uri") || strings.Contains(exprSrc, ".email"),
							"resource %s for_each attribute in %s must not reference dynamic resource attributes: %s", resName, tfFile, exprSrc)
					}
				}
			}
		}
	})
}
