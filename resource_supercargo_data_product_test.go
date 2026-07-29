package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSupercargoDataProduct_overrides(t *testing.T) {
	t.Setenv("SUPERCARGO_MOCK_TOKEN", "admin@example.com|platform-trust")
	manifestPath := filepath.Join(t.TempDir(), "product.yaml")
	manifestContent := `
meta:
  urn: urn:supercargo:hub:product:test-product
  version: v1.0.0
  owner:
    team_name: test-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:test-contract
      version: v1.0.0
    physical:
      bigquery:
        project: manifest-project
        dataset: manifest-dataset
        location: US
`
	err := os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_contract_version" "test" {
  urn          = "urn:supercargo:hub:contract:test-contract"
  version      = "v1.0.0"
  content_hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  commit_sha   = "0123456789abcdef0123456789abcdef01234567"
  data_asset   = "go://github.com/supercargo-dev/core/test"
  schema_json  = jsonencode([
    {
      name = "user_id"
      type = "STRING"
    }
  ])
}

resource "supercargo_data_product" "test" {
  manifest_file           = "` + manifestPath + `"
  project                 = "hcl-project"
  location                = "EU"
  partition_expiration_ms = 2592000000 # 30 days
  depends_on              = [supercargo_contract_version.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supercargo_data_product.test", "project", "hcl-project"),
					resource.TestCheckResourceAttr("supercargo_data_product.test", "location", "EU"),
					resource.TestCheckResourceAttr("supercargo_data_product.test", "partition_expiration_ms", "2592000000"),
					resource.TestCheckResourceAttr("supercargo_data_product.test", "service_identities.gateway", "serviceAccount:gateway-test-product@hcl-project.iam.gserviceaccount.com"),
					resource.TestCheckResourceAttr("supercargo_data_product.test", "service_identities.shovel", "serviceAccount:shovel-test-product@hcl-project.iam.gserviceaccount.com"),
				),
			},
			{
				// Step 2: Update with SLA
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_contract_version" "test" {
  urn          = "urn:supercargo:hub:contract:test-contract"
  version      = "v1.0.0"
  content_hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  commit_sha   = "0123456789abcdef0123456789abcdef01234567"
  data_asset   = "go://github.com/supercargo-dev/core/test"
  schema_json  = jsonencode([
    {
      name = "user_id"
      type = "STRING"
    }
  ])
}

resource "supercargo_data_product" "test" {
  manifest_file = "` + manifestPath + `"
  project       = "hcl-project"
  sla = {
    tier      = "SLA_TIER_1_MISSION_CRITICAL"
    freshness = "5m"
  }
  depends_on    = [supercargo_contract_version.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supercargo_data_product.test", "sla.tier", "SLA_TIER_1_MISSION_CRITICAL"),
					resource.TestCheckResourceAttr("supercargo_data_product.test", "sla.freshness", "5m"),
				),
			},
			{
				// Step 3: Attempt invalid SLA tier
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_data_product" "test" {
  manifest_file = "` + manifestPath + `"
  sla = {
    tier = "INVALID_TIER"
  }
}
`,
				ExpectError: regexp.MustCompile("Invalid SLA Tier"),
			},
			{
				// Step 4: Attempt invalid partitioning field
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_contract_version" "test" {
  urn          = "urn:supercargo:hub:contract:test-contract"
  version      = "v1.0.0"
  content_hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  commit_sha   = "0123456789abcdef0123456789abcdef01234567"
  data_asset   = "go://github.com/supercargo-dev/core/test"
  schema_json  = jsonencode([
    {
      name = "user_id"
      type = "STRING"
    }
  ])
}

resource "supercargo_data_product" "test" {
  manifest_file      = "` + manifestPath + `"
  partitioning_field = "non_existent_field"
  depends_on         = [supercargo_contract_version.test]
}
`,
				ExpectError: regexp.MustCompile("partitioning field .* not found"),
			},
		},
	})
}
