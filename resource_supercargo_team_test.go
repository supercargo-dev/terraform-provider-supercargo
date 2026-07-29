package main

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSupercargoTeam_basic(t *testing.T) {
	t.Setenv("SUPERCARGO_MOCK_TOKEN", "admin@example.com|platform-trust")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_team" "test" {
  name        = "test-team"
  data_asset  = "go://github.com/supercargo-dev/core/test"
  members     = ["user@example.com"]
  metadata    = {
    cost_center = "1234"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supercargo_team.test", "name", "test-team"),
					resource.TestCheckResourceAttr("supercargo_team.test", "urn", "urn:supercargo:hub:team:test-team"),
					resource.TestCheckResourceAttr("supercargo_team.test", "members.0", "user@example.com"),
					resource.TestCheckResourceAttr("supercargo_team.test", "metadata.cost_center", "1234"),
				),
			},
		},
	})
}
