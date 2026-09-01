package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModules_CloudRunLifecycleDecoupling(t *testing.T) {
	modulesDir := "../../modules"
	cloudRunModules := []struct {
		moduleName   string
		resourceName string
	}{
		{moduleName: "hub", resourceName: "hub"},
		{moduleName: "hub", resourceName: "shovel"},
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

	t.Run("GatewayContractsLocalComputation", func(t *testing.T) {
		if !strings.Contains(mainContent, "gateway_contracts =") {
			t.Errorf("data_product/main.tf missing gateway_contracts local definition")
		}
		if !strings.Contains(mainContent, "supercargo_data_product.this.contracts") {
			t.Errorf("data_product/main.tf gateway_contracts must reference supercargo_data_product.this.contracts")
		}
		if !strings.Contains(mainContent, "contracts = local.gateway_contracts") {
			t.Errorf("data_product/main.tf module.gateway must pass contracts = local.gateway_contracts")
		}
	})

	t.Run("GatewayDependsOnDataProduct", func(t *testing.T) {
		gatewayIdx := strings.Index(mainContent, `module "gateway"`)
		if gatewayIdx == -1 {
			t.Fatalf("module gateway not found in %s", mainPath)
		}
		gatewayChunk := mainContent[gatewayIdx:]
		if !strings.Contains(gatewayChunk, "supercargo_data_product.this") {
			t.Errorf("module gateway must depend on supercargo_data_product.this")
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
		if !strings.Contains(outputContent, "supercargo_data_product.this.contracts") {
			t.Errorf("data_product/outputs.tf output \"contracts\" value must be supercargo_data_product.this.contracts")
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
		if !strings.Contains(outputContent, "google_service_account.push_invoker.email") {
			t.Errorf("modules/gateway/outputs.tf push_invoker_service_account_email must reference google_service_account.push_invoker.email")
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
