terraform {
  required_providers {
    ko = {
      source  = "unicorn/fart/ko"
      version = "0.1.0"
    }
  }
}

provider "ko" {
  docker_repo = "ttl.sh/booker"
}

provider "kubernetes" {
  config_path    = "~/.kube/config"
  config_context = "k3d-k3s-default"
}

resource "ko_resolve" "example" {
  filenames = ["../../../internal/provider/testdata/k8s.yaml"]
  recursive = false
}

output "id" {
  value = ko_resolve.example.id
}

output "manifests" {
  value = ko_resolve.example.manifests
}

# resource "kubernetes_manifest" "these" {
#   count = length(ko_resolve.example.manifests)
#
#   manifest = yamldecode(ko_resolve.example.manifests[count.index])
#
#   wait {
#     fields = {
#       "status.phase" = "Running"
#     }
#   }
# }
