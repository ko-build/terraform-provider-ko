terraform {
  required_providers {
    ko = {
      source  = "chainguard-dev/ko"
      version = "0.0.2"
    }
    google = {
      source  = "hashicorp/google"
      version = "4.26.0"
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
  importpath = "github.com/chainguard-dev/terraform-provider-ko/cmd/test"
}

resource "google_cloud_run_service" "svc" {
  name     = "terraformed"
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
