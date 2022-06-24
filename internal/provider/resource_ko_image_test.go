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
					"ko_image.foo", "image_ref", regexp.MustCompile("^my-repo@sha256:abc$")),
			),
		}, {
			Config: testAccResourceKoImage,
			PreConfig: func() {
				changed = true
			},
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr(
					"ko_image.foo", "image_ref", regexp.MustCompile("^my-repo@sha256:abc$")),
			),
		}},
	})
}

const testAccResourceKoImage = `
resource "ko_image" "foo" {
	importpath = "github.com/google/ko"
}
`
