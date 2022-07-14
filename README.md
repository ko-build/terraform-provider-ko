# Terraform Provider for `ko`

🚨 **This is a work in progress.** 🚨

https://registry.terraform.io/providers/chainguard-dev/ko

## Usage

To use this provider to build and image and deploy it to Cloud Run:

```
terraform {
  required_providers {
    ko = {
      source  = "chainguard-dev/ko"
      version = "0.0.2" // Or whatever release
    }
    google = {
      source  = "hashicorp/google"
      version = "4.26.0" // Or whatever release
    }
  }
}

provider "ko" {}

variable "project" {
  type = string
}

provider "google" {
  project = var.project
}

resource "ko_image" "test" {
  // This is a simple HTTP server.
  importpath = "github.com/chainguard-dev/terraform-provider-ko/cmd/test"
}

resource "google_cloud_run_service" "svc" {
  name = "terraformed"
  location = "us-east4"
  template {
    spec {
      containers {
        image = ko_image.test.image_ref
      }
    }
  }
  traffic {
    percent         = 100
    latest_revision = true
  }
}
```

(See docs for [`google_cloud_run_service`](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_service))

Setup your environment with:

```
export KO_DOCKER_REPO=gcr.io/[MY-PROJECT]
gcloud auth login
gcloud auth application-default login
terraform init
```

Then you can build and apply this change with:

```
terraform apply -var project=[MY-PROJECT]
```

In this case, the image will be rebuilt every time it's _referenced_, and will only report as having changed if the image that was built was different since the last time the image resource was read.

This means that `terraform plan` will rebuild all referenced images, but only show diffs if rebuilds resulted in new images since last time the plan was made.
