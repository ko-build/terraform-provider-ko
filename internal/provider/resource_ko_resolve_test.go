package provider

import (
	"fmt"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccResourceKoResolve(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()
	t.Setenv("KO_DOCKER_REPO", repo.String())
	imageRefRE := regexp.MustCompile("^image: " + repo.String() + "/github.com/google/ko/test@sha256:")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["../../testdata/simple.yaml"]
				  recursive = false
                }
                `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.0", imageRefRE),
				),
			},
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["../../testdata/multi.yaml"]
				  recursive = false
                }
                `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.0", regexp.MustCompile("0: "+repo.String()+"/github.com/google/ko/test@sha256:")),
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.1", regexp.MustCompile("1: "+repo.String()+"/github.com/google/ko/test@sha256:")),
				),
			},
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["../../testdata/k8s.yaml"]
				  recursive = false
                }
                `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.0", regexp.MustCompile(fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: kodata
  namespace: default
spec:
  containers:
  - image: %s/github.com/google/ko/test@sha256:.+
    name: obiwan
`, repo.String())),
					)),
			},
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["../../testdata/recursive"]
				  recursive = true
			    }
			    `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.0", regexp.MustCompile("^a: "+repo.String()+"/github.com/google/ko/test@sha256:")),
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.1", regexp.MustCompile("^b: "+repo.String()+"/github.com/google/ko/test@sha256:")),
				),
			},
		},
	})
}
