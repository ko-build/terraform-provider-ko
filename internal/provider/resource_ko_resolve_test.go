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

func TestAccResourceKoResolve(t *testing.T) {
	// Setup a local registry and have tests push to that.
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	parts := strings.Split(srv.URL, ":")
	url := fmt.Sprintf("localhost:%s/test", parts[len(parts)-1])
	t.Setenv("KO_DOCKER_REPO", url)

	imageRefRE := regexp.MustCompile("^" + url + "/github.com/chainguard-dev/terraform-provider-ko/cmd/test@sha256:")

	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["testdata/"]
				  recursive = true
				  platforms = ["amd64", "arm64"]
                }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_image.foo", "image_ref", imageRefRE),
				),
			},
		},
	})
}
