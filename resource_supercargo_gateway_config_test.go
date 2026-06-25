package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSupercargoGatewayConfig_hash(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "product.yaml")
	manifestContent1 := `
meta:
  urn: urn:supercargo:hub:product:gateway-product
  version: v1.0.0
  owner:
    team_name: test-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:test-contract
      version: v1.0.0
`
	manifestContent2 := `
meta:
  urn: urn:supercargo:hub:product:gateway-product
  version: v1.1.0
  owner:
    team_name: test-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
    contract:
      urn: urn:supercargo:hub:contract:test-contract
      version: v1.1.0
`
	err := os.WriteFile(manifestPath, []byte(manifestContent1), 0644)
	if err != nil {
		t.Fatal(err)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Step 1: Initial hash
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_gateway_config" "test" {
  manifest_file = "` + manifestPath + `"
  hub_address   = "https://hub.example.com"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("supercargo_gateway_config.test", "config_hash"),
				),
			},
			{
				// Step 2: Update manifest, check hash changes
				PreConfig: func() {
					if err := os.WriteFile(manifestPath, []byte(manifestContent2), 0644); err != nil {
						t.Fatal(err)
					}
				},
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_gateway_config" "test" {
  manifest_file = "` + manifestPath + `"
  hub_address   = "https://hub.example.com"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("supercargo_gateway_config.test", "config_hash"),
				),
			},
		},
	})
}
