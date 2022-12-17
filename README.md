# Terraform Provider for `ko`

🚨 **This is a work in progress.** 🚨

https://registry.terraform.io/providers/chainguard-dev/ko

## Usage

This provides a `ko_image` resource that will build the referenced Go application specified by the `importpath`, push an image to the configured container repository, and make the image's reference available to other Terraform resources.

```
provider "ko" {}

resource "ko_image" "example" {
  importpath = "github.com/chainguard-dev/terraform-provider-ko/cmd/test"
}
```

See provider examples:

- [Google Cloud Run](./provider-examples/cloudrun/README.md)
- [fly.io](./provider-examples/fly.io/README.md)
- [Kubernetes](./provider-examples/kubernetes/README.md)

The image will be rebuilt every time it's _referenced_, and will only report as having changed if the image that was built was different since the last time the image resource was read.

This means that `terraform plan` will rebuild all referenced images, but only show diffs if rebuilds resulted in new images since last time the plan was made.
