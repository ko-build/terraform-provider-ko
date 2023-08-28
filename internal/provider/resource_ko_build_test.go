package provider

import (
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestAccResourceKoBuild(t *testing.T) {
	// Setup a local registry and have tests push to that.
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	parts := strings.Split(srv.URL, ":")
	url := fmt.Sprintf("localhost:%s/test", parts[len(parts)-1])
	t.Setenv("KO_DOCKER_REPO", url)

	imageRefRE := regexp.MustCompile("^" + url + "/github.com/ko-build/terraform-provider-ko/cmd/test@sha256:")

	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: `
			resource "ko_build" "foo" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			  sbom = "spdx"
			}
			`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
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
			resource "ko_build" "base" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			}
			resource "ko_build" "top" {
				importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
				base_image = "${ko_build.base.image_ref}"
			}
			`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.top", "image_ref", imageRefRE),
				resource.TestMatchResourceAttr("ko_build.top", "base_image", imageRefRE),
				resource.TestMatchResourceAttr("ko_build.base", "image_ref", imageRefRE),
				// TODO(jason): Check that top's base_image attr matches base's image_ref exactly.
			),
		}},
	})

	// Test that working_dir can be set.
	// TODO: Setting the importpath as "." means the image gets pushed as $KO_DOCKER_REPO exactly,
	// and we probably want to expand this to be the full resolved importpath of the package.
	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: `
			resource "ko_build" "foo" {
			  importpath = "."
			  working_dir = "../../cmd/test"
			}
			`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.foo", "image_ref", regexp.MustCompile("^"+url+"@sha256:")),
				// TODO(jason): Check that top's base_image attr matches base's image_ref exactly.
			),
		}},
	})

	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: `
			resource "ko_build" "foo" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			  platforms = ["linux/amd64", "linux/arm64"]
			}
			`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
			),
		}},
	})

	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: `
			resource "ko_build" "foo" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			  platforms = ["all"]
			}
			`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
			),
		}},
	})

	for _, sbom := range []string{"spdx", "cyclonedx", "go.version-m", "none"} {
		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{{
				Config: fmt.Sprintf(`
			resource "ko_build" "foo" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			  sbom = %q
			}
			`, sbom),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
				),
			}},
		})
	}

	t.Run("SOURCE_DATE_EPOCH", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", "1234567890")
		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{{
				Config: `resource "ko_build" "foo" {
					importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
				}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
				),
			}},
		})
	})
	t.Run("SOURCE_DATE_EPOCH_failure", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", "abc123")
		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{{
				Config: `resource "ko_build" "foo" {
					importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
				}`,
				ExpectError: regexp.MustCompile("should be the number of seconds since"),
			}},
		})
	})
}

func TestAccResourceKoBuild_ImageRepo(t *testing.T) {
	// Setup a local registry and have tests push to that.
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	parts := strings.Split(srv.URL, ":")
	url := fmt.Sprintf("localhost:%s/test", parts[len(parts)-1])
	t.Setenv("KO_DOCKER_REPO", url)

	// Test that the repo attribute of the ko_build resource is respected, and
	// the returned image_ref's repo is exactly the configured repo, without
	// the importpath appended.
	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
		resource "ko_build" "foo" {
			importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			repo = "%s/configured-in-resource"
		}
		`, url),
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.foo", "image_ref", regexp.MustCompile("^"+url+"/configured-in-resource@sha256:")),
			),
		}},
	})
}

func TestAccResourceKoBuild_ProviderRepo(t *testing.T) {
	// Setup a local registry and have tests push to that.
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	parts := strings.Split(srv.URL, ":")
	url := fmt.Sprintf("localhost:%s/test", parts[len(parts)-1])
	t.Setenv("KO_DOCKER_REPO", url)

	var providerConfigured = map[string]func() (*schema.Provider, error){
		"ko": func() (*schema.Provider, error) { //nolint: unparam
			p := New("dev")()
			p.Schema["repo"].Default = url + "/configured-in-provider"
			return p, nil
		},
	}

	// Test that the repo attribute of the provider is respected, and overrides
	// the KO_DOCKER_REPO.
	// When configured in the provider, the importpath is appended to the image ref.
	resource.Test(t, resource.TestCase{
		ProviderFactories: providerConfigured,
		Steps: []resource.TestStep{{
			Config: `
		resource "ko_build" "foo" {
			importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
		}
		`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.foo", "image_ref", regexp.MustCompile("^"+url+"/configured-in-provider/github.com/ko-build/terraform-provider-ko/cmd/test@sha256:")),
			),
		}},
	})
}
