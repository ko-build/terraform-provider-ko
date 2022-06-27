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

	imageRefRE := regexp.MustCompile("^" + url + "/github.com/imjasonh/terraform-provider-ko/cmd/test@sha256:")

	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: `
			resource "ko_image" "foo" {
			  importpath = "github.com/imjasonh/terraform-provider-ko/cmd/test"
			}
			`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_image.foo", "image_ref", imageRefRE),
			),
		}},
		// TODO: add a test that there's no terraform diff if the image hasn't changed.
		// TODO: add a test that there's a terraform diff if the image has changed.
		// TODO: add a test covering what happens if the build fails for any reason.
	})

	// This tests building an image and using it as a base image for another image.
	// Mostly just to prove we can.
	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: `
			resource "ko_image" "base" {
			  importpath = "github.com/imjasonh/terraform-provider-ko/cmd/test"
			}
			resource "ko_image" "top" {
				importpath = "github.com/imjasonh/terraform-provider-ko/cmd/test"
				base_image = "${ko_image.base.image_ref}"
			}
			`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_image.top", "image_ref", imageRefRE),
				resource.TestMatchResourceAttr("ko_image.top", "base_image", imageRefRE),
				resource.TestMatchResourceAttr("ko_image.base", "image_ref", imageRefRE),
				// TODO(jason): Check that top's base_image attr matches base's image_ref exactly.
			),
		}},
	})
}
