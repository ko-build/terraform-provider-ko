package provider

import (
	"fmt"
	"net/http/httptest"
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
				  filenames = ["testdata/simple.yaml"]
				  recursive = false
                }
                `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("ko_resolve.foo", "manifests.0", fmt.Sprintf("image: %s/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:209aff1f4cc28fedaca03eecedccde10351d91f3f1fdc3f618129630a3ed2a33\n", url)),
				),
			},
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["testdata/multi.yaml"]
				  recursive = false
                }
                `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("ko_resolve.foo", "manifests.0", fmt.Sprintf("0: %s/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:209aff1f4cc28fedaca03eecedccde10351d91f3f1fdc3f618129630a3ed2a33\n", url)),
					resource.TestCheckResourceAttr("ko_resolve.foo", "manifests.1", fmt.Sprintf("1: %s/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:209aff1f4cc28fedaca03eecedccde10351d91f3f1fdc3f618129630a3ed2a33\n", url)),
				),
			},
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["testdata/k8s.yaml"]
				  recursive = false
                }
                `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("ko_resolve.foo", "manifests.0", fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
    name: kodata
    namespace: default
spec:
    containers:
        - image: %s/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:209aff1f4cc28fedaca03eecedccde10351d91f3f1fdc3f618129630a3ed2a33
          name: obiwan
`, url),
					)),
			},
			{
				Config: `
                resource "ko_resolve" "foo" {
				  filenames = ["testdata/recursive"]
				  recursive = true
			    }
			    `,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("ko_resolve.foo", "manifests.0", fmt.Sprintf("a: %s/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:209aff1f4cc28fedaca03eecedccde10351d91f3f1fdc3f618129630a3ed2a33\n", url)),
					resource.TestCheckResourceAttr("ko_resolve.foo", "manifests.1", fmt.Sprintf("b: %s/test-46c4b272b3716c422d5ff6dfc7547fa9@sha256:209aff1f4cc28fedaca03eecedccde10351d91f3f1fdc3f618129630a3ed2a33\n", url)),
				),
			},
		},
	})
}
