terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
    }
    ko = {
      source  = "ko-build/ko"
    }
  }
}

// See https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs
provider "kubernetes" {
  config_path = "~/.kube/config"
}

provider "ko" {}

resource "ko_build" "example" {
  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
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
          image = ko_build.example.image_ref
          name  = "app"
        }
      }
    }
  }
}
