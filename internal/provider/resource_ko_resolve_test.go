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

	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["../../testdata/simple.yaml"]
				  recursive = false
                }
                `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.0", regexp.MustCompile("^image: "+url+"/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:")),
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
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.0", regexp.MustCompile("^0: "+url+"/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:")),
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.1", regexp.MustCompile("^1: "+url+"/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:")),
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
        - image: %s/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:.+
          name: obiwan
`, url)),
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
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.0", regexp.MustCompile("^a: "+url+"/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:")),
					resource.TestMatchResourceAttr("ko_resolve.foo", "manifests.1", regexp.MustCompile("^b: "+url+"/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:")),
				),
			},
		},
	})
}
