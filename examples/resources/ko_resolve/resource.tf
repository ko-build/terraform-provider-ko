provider "ko" {
  docker_repo = "ttl.sh/booker"
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
