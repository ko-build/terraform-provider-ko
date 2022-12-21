terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "4.46.0"
    }
    ko = {
      source  = "chainguard-dev/ko"
      version = "0.0.2"
    }
  }
}

provider "google" {
  project = var.project
  region  = var.region
}

variable "project" {}

variable "region" {
  default = "us-central1"
}

variable "service" {
  default = "tf-ko-example"
}

provider "ko" {}

resource "ko_image" "example" {
  importpath = "github.com/chainguard-dev/terraform-provider-ko/cmd/test"
}

resource "google_cloud_run_service" "default" {
  name     = var.service
  location = var.region

  template {
    spec {
      containers {
        image = ko_image.example.image_ref
      }
    }
  }

  traffic {
    percent         = 100
    latest_revision = true
  }
}

data "google_iam_policy" "noauth" {
  binding {
    role = "roles/run.invoker"
    members = [
      "allUsers",
    ]
  }
}

resource "google_cloud_run_service_iam_policy" "noauth" {
  location = google_cloud_run_service.default.location
  project  = google_cloud_run_service.default.project
  service  = google_cloud_run_service.default.name

  policy_data = data.google_iam_policy.noauth.policy_data
}

output "url" {
  value = google_cloud_run_service.default.status[0].url
}
