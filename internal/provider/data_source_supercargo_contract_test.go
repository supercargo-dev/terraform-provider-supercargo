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
				// Step 1: Create initial version
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_contract_version" "test" {
  urn          = "` + contractURN + `"
  version      = "v1.0.0"
  content_hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  commit_sha   = "0123456789abcdef0123456789abcdef01234567"
  data_asset   = "go://github.com/supercargo-dev/core/test"
  schema_json  = jsonencode([
    {
      name = "user_id"
      type = "STRING"
      mode = "REQUIRED"
    },
    {
      name = "profile"
      type = "STRUCT"
      fields = [
        {
          name = "email"
          type = "STRING"
        }
      ]
    }
  ])
}

data "supercargo_contract" "specific" {
  id      = supercargo_contract_version.test.urn
  version = "v1.0.0"
}

data "supercargo_contract" "latest" {
  id = supercargo_contract_version.test.urn
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
