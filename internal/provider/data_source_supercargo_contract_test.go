package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSupercargoContractDataSource_basic(t *testing.T) {
	contractURN := "urn:supercargo:hub:contract:ds-test-contract-unique"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

data "supercargo_contract" "specific" {
  id      = "` + contractURN + `"
  version = "v1.0.0"
}

data "supercargo_contract" "latest" {
  id = "` + contractURN + `"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Check specific version data source
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "version", "v1.0.0"),
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "urn", contractURN),
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "fields.0.name", "user_id"),
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "fields.0.type", "STRING"),
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "fields.0.mode", "REQUIRED"),
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "fields.1.name", "profile"),
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "fields.1.type", "RECORD"),
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "fields.2.name", "profile.email"),
					resource.TestCheckResourceAttr("data.supercargo_contract.specific", "fields.2.type", "STRING"),

					// Check latest version data source
					resource.TestCheckResourceAttr("data.supercargo_contract.latest", "version", "v1.0.0"),
					resource.TestCheckResourceAttr("data.supercargo_contract.latest", "urn", contractURN),
				),
			},
		},
	})
}
