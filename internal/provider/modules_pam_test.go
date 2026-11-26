package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModules_DataProduct_PAMEntitlementsAndConditions(t *testing.T) {
	t.Parallel()

	dpDir := filepath.Join("..", "..", "modules", "data_product")
	parser := hclparse.NewParser()

	versionsFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "versions.tf"))
	require.False(t, diags.HasErrors(), "failed parsing versions.tf: %v", diags)
	versionsStr := string(versionsFile.Bytes)

	mainFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "main.tf"))
	require.False(t, diags.HasErrors(), "failed parsing main.tf: %v", diags)
	mainBody := mainFile.Body.(*hclsyntax.Body)

	pamFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "pam.tf"))
	require.False(t, diags.HasErrors(), "failed parsing pam.tf: %v", diags)
	pamBody := pamFile.Body.(*hclsyntax.Body)
	pamStr := string(pamFile.Bytes)

	varsFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "variables.tf"))
	require.False(t, diags.HasErrors(), "failed parsing variables.tf: %v", diags)
	varsBody := varsFile.Body.(*hclsyntax.Body)

	outputsFile, diags := parser.ParseHCLFile(filepath.Join(dpDir, "outputs.tf"))
	require.False(t, diags.HasErrors(), "failed parsing outputs.tf: %v", diags)
	outputsBody := outputsFile.Body.(*hclsyntax.Body)

	t.Run("versions.tf pins hashicorp/google >= 6.0.0", func(t *testing.T) {
		assert.Contains(t, versionsStr, "hashicorp/google")
		assert.Contains(t, versionsStr, ">= 6.0.0")
	})

	t.Run("main.tf public_output_views filters non-empty view_query", func(t *testing.T) {
		var found bool
		for _, block := range mainBody.Blocks {
			if block.Type == "locals" {
				if attr, ok := block.Body.Attributes["public_output_views"]; ok {
					found = true
					expr := exprToString(mainFile.Bytes, attr.Expr)
					assert.Contains(t, expr, `lookup(v, "materialization", "view") == "view"`)
					assert.Contains(t, expr, `trimspace(lookup(v, "view_query", "")) != ""`)
				}
			}
		}
		assert.True(t, found, "local.public_output_views must be defined in main.tf")
	})

	t.Run("pam.tf declares port_jit PAM entitlement with unstructured justification, manual_approvals, and precondition", func(t *testing.T) {
		var found bool
		for _, block := range pamBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 &&
				block.Labels[0] == "google_privileged_access_manager_entitlement" &&
				block.Labels[1] == "port_jit" {
				found = true
				assert.Contains(t, exprToString(pamFile.Bytes, block.Body.Attributes["for_each"].Expr), "local.pam_eligible_outputs")
				assert.Contains(t, exprToString(pamFile.Bytes, block.Body.Attributes["parent"].Expr), `"projects/${var.project_id}"`)
				assert.Contains(t, exprToString(pamFile.Bytes, block.Body.Attributes["location"].Expr), "var.pam_location")
				assert.Contains(t, exprToString(pamFile.Bytes, block.Body.Attributes["entitlement_id"].Expr), "each.value.entitlement_id")
				assert.Contains(t, exprToString(pamFile.Bytes, block.Body.Attributes["max_request_duration"].Expr), "var.pam_max_request_duration")

				var hasUnstructured, hasEligibleUsers, hasGCPAccess, hasManualApprovals, hasPrecondition bool
				for _, sub := range block.Body.Blocks {
					switch sub.Type {
					case "requester_justification_config":
						for _, inner := range sub.Body.Blocks {
							if inner.Type == "unstructured" {
								hasUnstructured = true
							}
						}
					case "eligible_users":
						hasEligibleUsers = true
						assert.Contains(t, exprToString(pamFile.Bytes, sub.Body.Attributes["principals"].Expr), "var.pam_eligible_consumers")
					case "privileged_access":
						for _, gcp := range sub.Body.Blocks {
							if gcp.Type == "gcp_iam_access" {
								hasGCPAccess = true
								assert.Contains(t, exprToString(pamFile.Bytes, gcp.Body.Attributes["resource_type"].Expr), "cloudresourcemanager.googleapis.com/Project")
								assert.Contains(t, exprToString(pamFile.Bytes, gcp.Body.Attributes["resource"].Expr), "//cloudresourcemanager.googleapis.com/projects/${var.project_id}")
							}
						}
					case "approval_workflow":
						for _, ma := range sub.Body.Blocks {
							if ma.Type == "manual_approvals" {
								hasManualApprovals = true
								assert.Contains(t, exprToString(pamFile.Bytes, ma.Body.Attributes["require_approver_justification"].Expr), "true")
							}
						}
					case "lifecycle":
						for _, pre := range sub.Body.Blocks {
							if pre.Type == "precondition" {
								hasPrecondition = true
								assert.Contains(t, exprToString(pamFile.Bytes, pre.Body.Attributes["condition"].Expr), "length(var.pam_approvers) > 0")
							}
						}
					}
				}
				assert.True(t, hasUnstructured, "port_jit must require unstructured justification")
				assert.True(t, hasEligibleUsers, "port_jit must configure eligible_users")
				assert.True(t, hasGCPAccess, "port_jit must configure privileged_access.gcp_iam_access")
				assert.True(t, hasManualApprovals, "port_jit must configure approval_workflow.manual_approvals")
				assert.True(t, hasPrecondition, "port_jit must enforce lifecycle precondition on var.pam_approvers")
			}
		}
		assert.True(t, found, "google_privileged_access_manager_entitlement.port_jit must be declared in pam.tf")
		assert.Contains(t, pamStr, "roles/bigquery.dataViewer")
		assert.Contains(t, pamStr, "roles/cloudkms.cryptoKeyDecrypter")
		assert.Contains(t, pamStr, "var.enable_reidentified_kms_pam")
		assert.Contains(t, pamStr, `trimprefix(var.master_key_uri, "gcp-kms://")`)
	})

	t.Run("pam.tf declares google_bigquery_table_iam_member.conditional_consumers and zero dataset condition blocks", func(t *testing.T) {
		var foundTableMember bool
		for _, block := range pamBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 &&
				block.Labels[0] == "google_bigquery_table_iam_member" &&
				block.Labels[1] == "conditional_consumers" {
				foundTableMember = true
				assert.Contains(t, exprToString(pamFile.Bytes, block.Body.Attributes["table_id"].Expr), "google_bigquery_table.outputs[each.value.output_key].table_id")
				var hasCond bool
				for _, sub := range block.Body.Blocks {
					if sub.Type == "condition" {
						hasCond = true
						assert.Contains(t, exprToString(pamFile.Bytes, sub.Body.Attributes["title"].Expr), "supercargo-jit-ttl-${each.value.grant_key}")
						assert.Contains(t, exprToString(pamFile.Bytes, sub.Body.Attributes["expression"].Expr), `request.time < timestamp("${each.value.expiration}")`)
					}
				}
				assert.True(t, hasCond, "conditional_consumers must include condition block")
			}
		}
		assert.True(t, foundTableMember, "google_bigquery_table_iam_member.conditional_consumers must be declared in pam.tf")

		// Ensure zero google_bigquery_dataset_iam_member resources declare condition blocks across all .tf files
		tfFiles, err := filepath.Glob(filepath.Join(dpDir, "*.tf"))
		require.NoError(t, err)
		for _, tfPath := range tfFiles {
			f, fDiags := parser.ParseHCLFile(tfPath)
			require.False(t, fDiags.HasErrors(), "failed parsing %s: %v", tfPath, fDiags)
			body := f.Body.(*hclsyntax.Body)
			for _, block := range body.Blocks {
				if block.Type == "resource" && len(block.Labels) >= 1 && block.Labels[0] == "google_bigquery_dataset_iam_member" {
					for _, sub := range block.Body.Blocks {
						assert.NotEqual(t, "condition", sub.Type, "google_bigquery_dataset_iam_member in %s must not contain condition block", tfPath)
					}
				}
			}
		}
	})

	t.Run("variables.tf enforces strict validations and avoids master_kms_key_id and vault_search_hash_pam_requesters", func(t *testing.T) {
		varsByName := make(map[string]*hclsyntax.Block)
		for _, block := range varsBody.Blocks {
			if block.Type == "variable" && len(block.Labels) == 1 {
				varsByName[block.Labels[0]] = block
			}
		}

		assert.NotContains(t, varsByName, "master_kms_key_id")
		assert.NotContains(t, varsByName, "vault_search_hash_pam_requesters")

		requiredVars := []string{
			"enable_pam_entitlements",
			"pam_location",
			"pam_max_request_duration",
			"pam_eligible_consumers",
			"pam_approvers",
			"enable_reidentified_kms_pam",
			"conditional_consumer_grants",
		}
		for _, name := range requiredVars {
			require.Contains(t, varsByName, name, "variables.tf missing %s", name)
		}

		durBytes := exprToString(varsFile.Bytes, varsByName["pam_max_request_duration"].Body.Blocks[0].Body.Attributes["condition"].Expr)
		assert.Contains(t, durBytes, "900")
		assert.Contains(t, durBytes, "3600")

		for _, iamVar := range []string{"pam_eligible_consumers", "pam_approvers"} {
			cond := exprToString(varsFile.Bytes, varsByName[iamVar].Body.Blocks[0].Body.Attributes["condition"].Expr)
			assert.Contains(t, cond, "^(user|group|serviceAccount|domain):")
		}

		grantsCond := exprToString(varsFile.Bytes, varsByName["conditional_consumer_grants"].Body.Blocks[0].Body.Attributes["condition"].Expr)
		assert.Contains(t, grantsCond, "roles/bigquery.dataViewer")
		assert.Contains(t, grantsCond, "roles/bigquery.metadataViewer")
		assert.Contains(t, grantsCond, "^(user|group|serviceAccount|domain):")
		assert.Contains(t, grantsCond, "[0-9]{4}-[0-9]{2}-[0-9]{2}T")
	})

	t.Run("outputs.tf exposes pam_entitlement_ids, pam_gcloud_commands, and pam_console_urls", func(t *testing.T) {
		outputs := make(map[string]bool)
		for _, block := range outputsBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				outputs[block.Labels[0]] = true
			}
		}
		assert.True(t, outputs["pam_entitlement_ids"])
		assert.True(t, outputs["pam_gcloud_commands"])
		assert.True(t, outputs["pam_console_urls"])
	})
}

func TestModules_Vault_SearchHashPAM(t *testing.T) {
	t.Parallel()

	vaultDir := filepath.Join("..", "..", "modules", "vault")
	parser := hclparse.NewParser()

	versionsPath := filepath.Join(vaultDir, "versions.tf")
	_, err := os.Stat(versionsPath)
	require.NoError(t, err, "modules/vault/versions.tf must exist")

	versionsFile, diags := parser.ParseHCLFile(versionsPath)
	require.False(t, diags.HasErrors(), "failed parsing vault versions.tf: %v", diags)
	assert.Contains(t, string(versionsFile.Bytes), "hashicorp/google")
	assert.Contains(t, string(versionsFile.Bytes), ">= 6.0.0")

	mainFile, diags := parser.ParseHCLFile(filepath.Join(vaultDir, "main.tf"))
	require.False(t, diags.HasErrors(), "failed parsing vault main.tf: %v", diags)
	mainBody := mainFile.Body.(*hclsyntax.Body)
	mainStr := string(mainFile.Bytes)

	varsFile, diags := parser.ParseHCLFile(filepath.Join(vaultDir, "variables.tf"))
	require.False(t, diags.HasErrors(), "failed parsing vault variables.tf: %v", diags)
	varsBody := varsFile.Body.(*hclsyntax.Body)

	outputsFile, diags := parser.ParseHCLFile(filepath.Join(vaultDir, "outputs.tf"))
	require.False(t, diags.HasErrors(), "failed parsing vault outputs.tf: %v", diags)
	outputsBody := outputsFile.Body.(*hclsyntax.Body)

	t.Run("vault Cloud Run container wires SearchHash RBAC and justification env vars", func(t *testing.T) {
		assert.Contains(t, mainStr, `"VAULT_ENFORCE_SEARCH_JUSTIFICATION"`)
		assert.Contains(t, mainStr, `"ENFORCE_SEARCH_JUSTIFICATION"`)
		assert.Contains(t, mainStr, `"VAULT_SEARCH_HASH_ENFORCE_RBAC"`)
		assert.Contains(t, mainStr, `"VAULT_SEARCH_HASH_ALLOWED_ROLES"`)
	})

	t.Run("vault main.tf declares vault_search_hash_jit PAM entitlement", func(t *testing.T) {
		var found bool
		for _, block := range mainBody.Blocks {
			if block.Type == "resource" && len(block.Labels) == 2 &&
				block.Labels[0] == "google_privileged_access_manager_entitlement" &&
				block.Labels[1] == "vault_search_hash_jit" {
				found = true
				assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["for_each"].Expr), "var.enable_search_hash_pam")
				assert.Contains(t, exprToString(mainFile.Bytes, block.Body.Attributes["max_request_duration"].Expr), "var.search_hash_pam_max_duration")

				var hasUnstructured, hasManualApprovals, hasPrecondition, hasRunInvoker bool
				for _, sub := range block.Body.Blocks {
					switch sub.Type {
					case "requester_justification_config":
						for _, inner := range sub.Body.Blocks {
							if inner.Type == "unstructured" {
								hasUnstructured = true
							}
						}
					case "privileged_access":
						for _, gcp := range sub.Body.Blocks {
							if gcp.Type == "gcp_iam_access" {
								for _, rb := range gcp.Body.Blocks {
									if rb.Type == "role_bindings" {
										if roleAttr, ok := rb.Body.Attributes["role"]; ok &&
											exprToString(mainFile.Bytes, roleAttr.Expr) == `"roles/run.invoker"` {
											hasRunInvoker = true
											cond := exprToString(mainFile.Bytes, rb.Body.Attributes["condition_expression"].Expr)
											assert.Contains(t, cond, "google_cloud_run_v2_service.vault.name")
										}
									}
								}
							}
						}
					case "approval_workflow":
						for _, ma := range sub.Body.Blocks {
							if ma.Type == "manual_approvals" {
								hasManualApprovals = true
							}
						}
					case "lifecycle":
						for _, pre := range sub.Body.Blocks {
							if pre.Type == "precondition" {
								hasPrecondition = true
								assert.Contains(t, exprToString(mainFile.Bytes, pre.Body.Attributes["condition"].Expr), "length(var.search_hash_pam_approvers) > 0")
							}
						}
					}
				}
				assert.True(t, hasUnstructured)
				assert.True(t, hasRunInvoker)
				assert.True(t, hasManualApprovals)
				assert.True(t, hasPrecondition)
			}
		}
		assert.True(t, found, "google_privileged_access_manager_entitlement.vault_search_hash_jit must be declared")
	})

	t.Run("vault variables.tf and outputs.tf expose SearchHash PAM contract", func(t *testing.T) {
		varsByName := make(map[string]bool)
		for _, block := range varsBody.Blocks {
			if block.Type == "variable" && len(block.Labels) == 1 {
				varsByName[block.Labels[0]] = true
			}
		}
		for _, v := range []string{
			"enable_search_hash_pam",
			"search_hash_pam_location",
			"search_hash_pam_max_duration",
			"search_hash_pam_requesters",
			"search_hash_pam_approvers",
			"enforce_search_justification",
			"search_hash_allowed_roles",
		} {
			assert.True(t, varsByName[v], "vault variables.tf missing %s", v)
		}

		outputs := make(map[string]bool)
		for _, block := range outputsBody.Blocks {
			if block.Type == "output" && len(block.Labels) == 1 {
				outputs[block.Labels[0]] = true
			}
		}
		assert.True(t, outputs["search_hash_pam_entitlement_id"])
		assert.True(t, outputs["search_hash_pam_gcloud_command"])
		assert.True(t, outputs["search_hash_pam_console_url"])
	})
}
