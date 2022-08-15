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
  filenames = ["../../../testdata"]
  recursive = true
}

output "id" {
  value = data.ko_dry_resolve.example.id
}

output "manifests" {
  value = data.ko_dry_resolve.example.manifests
}
