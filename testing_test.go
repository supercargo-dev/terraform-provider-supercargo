package main

import (
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func init() {
	if os.Getenv("SUPERCARGO_MOCK_TOKEN") == "" {
		os.Setenv("SUPERCARGO_MOCK_TOKEN", "admin@example.com|platform-trust")
	}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"supercargo": providerserver.NewProtocol6WithError(New()),
}
