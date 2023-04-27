package provider

import (
	"fmt"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccResourceKoBuild(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()
	t.Setenv("KO_DOCKER_REPO", repo.String())
	imageRefRE := regexp.MustCompile("^" + repo.String() + "/github.com/ko-build/terraform-provider-ko/cmd/test@sha256:")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
			resource "ko_build" "foo" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			}
			`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
					// TODO: add a test that there's no terraform diff if the image hasn't changed.
					// TODO: add a test that there's a terraform diff if the image has changed.
					// TODO: add a test covering what happens if the build fails for any reason.
				),
			},
			{
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
			},
			{
				// TODO: Building in this way means the image_ref will be different, and not include the importpath component.
				Config: `
			resource "ko_build" "foo" {
			  importpath = "."
			  working_dir = "../../cmd/test"
			}
			`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_build.foo", "image_ref",
						regexp.MustCompile("^"+repo.String()+"@sha256:")),
				),
			},
			{
				Config: `
			resource "ko_build" "foo" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			  platforms = ["linux/amd64", "linux/arm64"]
			}
			`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
					// TODO: Check that the image has the two platforms.
				),
			},
			{
				Config: `
			resource "ko_build" "foo" {
			  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			  platforms = ["all"]
			}
			`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
					// TODO: Check that the image has multiple platforms.
				),
			}},
	})

	for sbom := range validTypes {
		resource.Test(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
}

func TestAccResourceKoBuild_ImageRepo(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()
	t.Setenv("KO_DOCKER_REPO", repo.String())
	imageRefRE := regexp.MustCompile("^" + repo.String() + "/configured-in-resource@sha256:")

	// Test that the repo attribute of the ko_build resource is respected, and
	// the returned image_ref's repo is exactly the configured repo, without
	// the importpath appended.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
		resource "ko_build" "foo" {
			importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
			repo = "%s/configured-in-resource"
		}
		`, repo.String()),
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
			),
		}},
	})
}

func TestAccResourceKoBuild_ProviderRepo(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()
	t.Setenv("KO_DOCKER_REPO", repo.String())
	imageRefRE := regexp.MustCompile("^" + repo.String() + "/configured-in-provider/github.com/ko-build/terraform-provider-ko/cmd/test@sha256:")

	// Test that the repo attribute of the provider is respected, and overrides
	// the KO_DOCKER_REPO.
	// When configured in the provider, the importpath is appended to the image ref.
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ko": providerserver.NewProtocol6WithError(&Provider{
				repo: repo.String() + "/configured-in-provider",
			}),
		},
		Steps: []resource.TestStep{{
			Config: `
		resource "ko_build" "foo" {
			importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
		}
		`,
			Check: resource.ComposeTestCheckFunc(
				resource.TestMatchResourceAttr("ko_build.foo", "image_ref", imageRefRE),
			),
		}},
	})
}
