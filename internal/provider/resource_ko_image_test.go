package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceKoImage(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: `
			resource "ko_image" "foo" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			  sbom = "spdx"
			}
			`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_image.foo", "image_ref", regexp.MustCompile("^$")),
			),
		}},
	})
}
