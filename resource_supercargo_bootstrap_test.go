package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSupercargoBootstrap_teamAndProduct(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "product.yaml")
	manifestContent := `
meta:
  urn: urn:supercargo:hub:product:bootstrap-product
  version: v1.0.0
  owner:
    team_name: bootstrap-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:test-contract
      version: v1.0.0
`
	err := os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Step 1: Create Team and Product together
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_team" "test" {
  name        = "bootstrap-team"
  data_asset  = "go://github.com/supercargo-dev/core/test"
}

resource "supercargo_data_product" "test" {
  manifest_file = "` + manifestPath + `"
  depends_on    = [supercargo_team.test]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supercargo_team.test", "name", "bootstrap-team"),
					resource.TestCheckResourceAttr("supercargo_data_product.test", "urn", "urn:supercargo:hub:product:bootstrap-product"),
				),
			},
		},
	})
}
