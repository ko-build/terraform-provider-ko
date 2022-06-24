package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceKoImage(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: testAccResourceKoImage,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr(
					"ko_image.foo", "image_ref", regexp.MustCompile("^gcr.io/jason-chainguard/github.com/imjasonh/ko-terraform-provider@sha256:")),
			),
		}},
	})
}

const testAccResourceKoImage = `
resource "ko_image" "foo" {
  importpath = "github.com/imjasonh/ko-terraform-provider"
}
`
