# Terraform Provider for `ko`

🚨 **This is a work in progress.** 🚨

https://registry.terraform.io/providers/imjasonh/ko

## Usage

```
terraform {
  required_providers {
    ko = {
      source = "imjasonh/ko"
      version = "0.0.1-pre-3" # Or whatever release.
    }
  }
}

provider "ko" {}

resource "ko_image" "foo" {
  importpath = "github.com/imjasonh/terraform-provider-ko"
}
```

And reference the built image by digest in other resources like this:

```
resource "google_cloud_run_service" "svc" {
  template {
    spec {
      containers {
        image = ko_image.foo.image_ref
      }
    }
  }
}
```

(See docs for [`google_cloud_run_service`](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_service))

Then you can build and apply this change with:

```
terraform apply
```

In this case, the image will be rebuilt every time it's _referenced_, and will only report as having changed if the image changed since the last time the image resource was read.

This means that `terraform plan` will rebuild all referenced images, but only show diffs if rebuilds resulted in new images since last time the plan was made.

---

To test:

```
TF_ACC=1 go test ./internal/provider/...
```