package provider

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
		if !strings.Contains(subChunk, "var.oidc_audience != \"\" ? var.oidc_audience : google_cloud_run_v2_service.hub.uri") {
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
		if !strings.Contains(mcpChunk, `var.oidc_audience != "" ? var.oidc_audience : google_cloud_run_v2_service.hub.uri`) {
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
	if !strings.Contains(subChunk, "var.oidc_audience != \"\" ? var.oidc_audience : google_cloud_run_v2_service.hub.uri") {
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
				name: "analytics_hub_listing_id output",
				snippets: []string{
					`output "analytics_hub_listing_id"`,
					`try(google_bigquery_analytics_hub_listing.telemetry_listing[0].id, null)`,
				},
			},
			{
				name: "sharing_enabled output",
				snippets: []string{
					`output "sharing_enabled"`,
					`var.enable_global_telemetry_share`,
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

	t.Run("Variables", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "variables.tf")

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
				name: "job_name variable with default",
				expected: []string{
					`variable "job_name"`,
					`type        = string`,
					`default     = "analytics-github-sync"`,
				},
			},
			{
				name: "container_image variable",
				expected: []string{
					`variable "container_image"`,
					`type        = string`,
				},
			},
			{
				name: "org variable",
				expected: []string{
					`variable "org"`,
					`type        = string`,
				},
			},
			{
				name: "topic variable with default",
				expected: []string{
					`variable "topic"`,
					`type        = string`,
					`default     = "supercargo-analytics"`,
				},
			},
			{
				name: "dataset variable with default",
				expected: []string{
					`variable "dataset"`,
					`type        = string`,
					`default     = "supercargo_github"`,
				},
			},
			{
				name: "vault_url variable",
				expected: []string{
					`variable "vault_url"`,
					`type        = string`,
				},
			},
			{
				name: "vault_audience variable",
				expected: []string{
					`variable "vault_audience"`,
					`type        = string`,
				},
			},
			{
				name: "github_token_secret_id variable",
				expected: []string{
					`variable "github_token_secret_id"`,
					`type        = string`,
				},
			},
			{
				name: "deletion_protection variable",
				expected: []string{
					`variable "deletion_protection"`,
					`type        = bool`,
					`default     = false`,
				},
			},
			{
				name: "schedule variable",
				expected: []string{
					`variable "schedule"`,
					`type        = string`,
				},
			},
			{
				name: "time_zone variable",
				expected: []string{
					`variable "time_zone"`,
					`type        = string`,
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

	t.Run("DeterministicLocals", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "main.tf")

		assert.Contains(t, content, `locals {`)
		assert.Contains(t, content, `sa_email`)
		assert.Contains(t, content, `"${var.service_account_id != null && var.service_account_id != "" ? var.service_account_id : "analytics-github-sync-sa"}@${var.project_id}.iam.gserviceaccount.com"`)
		assert.Contains(t, content, `scheduler_sa_email`)
		assert.Contains(t, content, `"${var.scheduler_service_account_id != null && var.scheduler_service_account_id != "" ? var.scheduler_service_account_id : "analytics-github-sync-sched-sa"}@${var.project_id}.iam.gserviceaccount.com"`)
	})

	t.Run("CloudRunJob", func(t *testing.T) {
		content := readAnalyticsGitHubSyncModuleFile(t, "main.tf")

		tests := []struct {
			name     string
			snippets []string
		}{
			{
				name: "job resource definition",
				snippets: []string{
					`resource "google_cloud_run_v2_job" "sync_job"`,
					`var.job_name`,
					`var.region`,
					`var.project_id`,
					`var.deletion_protection`,
				},
			},
			{
				name: "lifecycle ignore_changes",
				snippets: []string{
					`lifecycle {`,
					`ignore_changes = [`,
					`template[0].template[0].containers[0].image,`,
					`client,`,
					`client_version,`,
				},
			},
			{
				name: "container environment variables",
				snippets: []string{
					`name  = "SUPERCARGO_VAULT_URL"`,
					`value = var.vault_url`,
					`name  = "SUPERCARGO_VAULT_AUDIENCE"`,
					`value = var.vault_audience`,
					`name  = "GOOGLE_CLOUD_PROJECT"`,
					`value = var.project_id`,
					`name  = "DESTINATION__BIGQUERY__LOCATION"`,
					`name = "GITHUB_TOKEN"`,
					`secret_key_ref {`,
					`secret  = var.github_token_secret_id`,
					`version = "latest"`,
				},
			},
			{
				name: "service account assignment",
				snippets: []string{
					`service_account = local.sa_email`,
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
				name: "scheduler invoker IAM binding",
				snippets: []string{
					`resource "google_cloud_run_v2_job_iam_member" "scheduler_invoker"`,
					`"roles/run.invoker"`,
					`"serviceAccount:${local.scheduler_sa_email}"`,
				},
			},
			{
				name: "cloud scheduler job definition",
				snippets: []string{
					`resource "google_cloud_scheduler_job" "sync_schedule"`,
					`https://${var.region}-run.googleapis.com/v2/projects/${var.project_id}/locations/${var.region}/jobs/${google_cloud_run_v2_job.sync_job.name}:run`,
					`oauth_token {`,
					`local.scheduler_sa_email`,
					`"https://www.googleapis.com/auth/cloud-platform"`,
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
		content := readAnalyticsGitHubSyncModuleFile(t, "outputs.tf")

		tests := []struct {
			name     string
			snippets []string
		}{
			{
				name: "job_name output",
				snippets: []string{
					`output "job_name"`,
					`google_cloud_run_v2_job.sync_job.name`,
				},
			},
			{
				name: "job_id output",
				snippets: []string{
					`output "job_id"`,
					`google_cloud_run_v2_job.sync_job.id`,
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
					`google_cloud_scheduler_job.sync_schedule.name`,
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
	})
}
