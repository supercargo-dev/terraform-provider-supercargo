package main

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSupercargoIngestionGateway_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "supercargo" {
  hub_address = "localhost:50051"
}

resource "supercargo_ingestion_gateway" "test" {
  contract_id      = "urn:supercargo:hub:contract:test-contract"
  contract_version = "v1.0.0"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("supercargo_ingestion_gateway.test", "id", "gateway-urn:supercargo:hub:contract:test-contract"),
				),
			},
		},
	})
}
