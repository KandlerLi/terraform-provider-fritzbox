package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/KandlerLi/terraform-provider-fritzbox/internal/provider"
)

// Set by goreleaser at release time.
var version = "dev"

func main() {
	debug := flag.Bool("debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/KandlerLi/fritzbox",
		Debug:   *debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
