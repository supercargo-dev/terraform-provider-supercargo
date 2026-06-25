package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSupercargoE2E_rollingUpdate(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "product.yaml")

	config1 := `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_team" "test" {
  name        = "e2e-team"
  data_asset  = "go://github.com/supercargo-dev/core/test"
}

resource "supercargo_contract_version" "test" {
  urn          = "urn:supercargo:hub:contract:e2e-contract"
  version      = "v1.0.0"
  content_hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  commit_sha   = "0123456789abcdef0123456789abcdef01234567"
  data_asset   = "go://github.com/supercargo-dev/core/test"
  schema_json  = jsonencode([
    {
      name = "id"
      type = "STRING"
    }
  ])
}

resource "supercargo_data_product" "test" {
  manifest_file = "` + manifestPath + `"
  depends_on    = [supercargo_team.test, supercargo_contract_version.test]
}

resource "supercargo_gateway_config" "test" {
  manifest_file = "` + manifestPath + `"
  hub_address   = "https://hub.example.com"
}

# In a real scenario, this would be a google_cloud_run_v2_service
# We'll use a terraform_data resource to simulate the dependency on the hash
resource "terraform_data" "gateway_trigger" {
  input = supercargo_gateway_config.test.config_hash
}
`

	manifestContent1 := `
meta:
  urn: urn:supercargo:hub:product:e2e-product
  version: v1.0.0
  owner:
    team_name: e2e-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:e2e-contract
      version: v1.0.0
`

	manifestContent2 := `
meta:
  urn: urn:supercargo:hub:product:e2e-product
  version: v1.1.0
  owner:
    team_name: e2e-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:e2e-contract
      version: v1.0.0
`

	err := os.WriteFile(manifestPath, []byte(manifestContent1), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("supercargo_gateway_config.test", "config_hash"),
					resource.TestCheckResourceAttrPair("terraform_data.gateway_trigger", "input", "supercargo_gateway_config.test", "config_hash"),
				),
			},
			{
				// Update manifest, verify hash change propagates to the trigger
				PreConfig: func() {
					if err := os.WriteFile(manifestPath, []byte(manifestContent2), 0644); err != nil {
						t.Fatal(err)
					}
				},
				Config: config1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("supercargo_gateway_config.test", "config_hash"),
					resource.TestCheckResourceAttrPair("terraform_data.gateway_trigger", "input", "supercargo_gateway_config.test", "config_hash"),
				),
			},
		},
	})
}
