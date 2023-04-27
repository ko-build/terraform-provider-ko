provider "ko" {
  repo = "ttl.sh/tf-ko"
}

resource "ko_build" "example" {
  importpath = "github.com/google/ko/test"
}

output "image_ref" {
  value = ko_build.example.image_ref
}

resource "ko_resolve" "example" {
  filenames = ["../../../testdata/k8s.yaml"]
  recursive = false
}

output "manifests" {
  value = ko_resolve.example.manifests
}
