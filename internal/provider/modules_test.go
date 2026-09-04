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
		if !strings.Contains(chunk, `[Tier 1] DLQ Quarantined Messages Backlog (${var.product_id}) - ${random_id.suffix.hex}`) {
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
		if !strings.Contains(chunk, `[Tier 1] DLQ Message Latency Exceeded (${var.product_id}) - ${random_id.suffix.hex}`) {
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

func TestModules_DLQMonitoring(t *testing.T) {
	TestModules_GatewayDLQMonitoring(t)
}
