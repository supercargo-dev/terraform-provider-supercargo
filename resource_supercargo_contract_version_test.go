package main

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSupercargoContractVersion_compatibility(t *testing.T) {
	t.Setenv("SUPERCARGO_MOCK_TOKEN", "admin@example.com|platform-trust")
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
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supercargo_contract_version.test", "version", "v1.0.0"),
				),
			},
			{
				// Step 2: Attempt breaking change (remove field)
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_contract_version" "test" {
  urn          = "urn:supercargo:hub:contract:test-contract"
  version      = "v1.1.0"
  content_hash = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
  commit_sha   = "abcdef0123456789abcdef0123456789abcdef01"
  data_asset   = "go://github.com/supercargo-dev/core/test"
  schema_json  = jsonencode([
    {
      name = "new_field"
      type = "STRING"
    }
  ])
}
`,
				ExpectError: regexp.MustCompile("Breaking Change Detected"),
			},
		},
	})
}
