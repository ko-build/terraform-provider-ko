package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceKoImage(t *testing.T) {
	koDockerRepo := os.Getenv("KO_DOCKER_REPO")
	if koDockerRepo == "" {
		t.Fatal("KO_DOCKER_REPO is not set")
	}
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: testAccResourceKoImage,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr(
					"ko_image.foo", "image_ref", regexp.MustCompile("^"+koDockerRepo+"/github.com/imjasonh/ko-terraform-provider@sha256:")),
			),
		}},
		// TODO: add a test that there's no terraform diff if the image hasn't changed.
		// TODO: add a test that there's a terraform diff if the image has changed.
		// TODO: add a test covering what happens if the build fails for any reason.
	})
}

const testAccResourceKoImage = `
resource "ko_image" "foo" {
  importpath = "github.com/imjasonh/ko-terraform-provider"
}
`
