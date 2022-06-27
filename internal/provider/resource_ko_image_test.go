package provider

import (
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccResourceKoImage(t *testing.T) {
	// Setup a local registry and have tests push to that.
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	parts := strings.Split(srv.URL, ":")
	url := fmt.Sprintf("localhost:%s", parts[len(parts)-1])
	t.Setenv("KO_DOCKER_REPO", url)

	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: testAccResourceKoImage,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr(
					"ko_image.foo", "image_ref", regexp.MustCompile("^"+url+"/github.com/imjasonh/terraform-provider-ko/cmd/test@sha256:")),
			),
		}},
		// TODO: add a test that there's no terraform diff if the image hasn't changed.
		// TODO: add a test that there's a terraform diff if the image has changed.
		// TODO: add a test covering what happens if the build fails for any reason.
	})
}

const testAccResourceKoImage = `
resource "ko_image" "foo" {
  importpath = "github.com/imjasonh/terraform-provider-ko/cmd/test"
}
`
