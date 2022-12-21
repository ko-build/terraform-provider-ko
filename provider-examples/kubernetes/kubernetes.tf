terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "2.16.1"
    }
    ko = {
      source  = "chainguard-dev/ko"
      version = "0.0.2"
    }
  }
}

// See https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs
provider "kubernetes" {
  config_path = "~/.kube/config"
}

provider "ko" {}

resource "ko_image" "example" {
  importpath = "github.com/chainguard-dev/terraform-provider-ko/cmd/test"
}

resource "kubernetes_namespace" "ns" {
  metadata {
    name = "tf-ko-example"
  }
}

resource "kubernetes_deployment" "deploy" {
  metadata {
    name      = "deployment"
    namespace = kubernetes_namespace.ns.metadata[0].name
  }
  spec {
    replicas = 3

    selector {
      match_labels = {
        app = "tf-ko-example"
      }
    }

    template {
      metadata {
        labels = {
          app = "tf-ko-example"
        }
      }

      spec {
        container {
          image = ko_image.example.image_ref
          name  = "app"
        }
      }
    }
  }
}
