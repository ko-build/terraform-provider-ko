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

data "ko_dry_resolve" "example" {
  filenames = ["../../../testdata/k8s.yaml"]
  recursive = false
}

resource "ko_resolve" "example" {
  filenames = ["../../../testdata/k8s.yaml"]
  recursive = false
}

output "id" {
  value = ko_resolve.example.id
}

output "manifests" {
  value = ko_resolve.example.manifests
}

provider "kubernetes" {
  config_path    = "~/.kube/config"
  config_context = "k3d-k3s-default"
}

resource "kubernetes_manifest" "name" {
  # count    = length(data.ko_dry_resolve.example)
  # manifest = yamldecode(ko_resolve.example.manifests[count.index])

  count    = length(data.ko_dry_resolve.example.manifests)
  manifest = yamldecode(ko_resolve.example.manifests[count.index])
}

# output "dry" {
#   value = data.ko_dry_resolve.example.manifests
# }
