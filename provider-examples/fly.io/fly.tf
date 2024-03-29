// https://fly.io/docs/app-guides/terraform-iac-getting-started/

terraform {
  required_providers {
    fly = {
      source = "fly-apps/fly"
    }
    ko = {
      source  = "ko-build/ko"
    }
  }
}

variable "app" {
  type = string
}

provider "ko" {}

resource "ko_build" "example" {
  importpath = "github.com/ko-build/terraform-provider-ko/cmd/test"
}

resource "fly_app" "example" {
  name = var.app
  org  = "personal"
}

resource "fly_ip" "ip" {
  app        = fly_app.example.name
  type       = "v4"
}

resource "fly_ip" "ipv6" {
  app        = fly_app.example.name
  type       = "v6"
}

resource "fly_machine" "machine" {
  for_each = toset(["ewr", "lax"])
  app    = var.app
  region = each.value
  name   = "${fly_app.example.name}-${each.value}"
  image  = ko_build.example.image_ref
  services = [
    {
      ports = [
        {
          port     = 443
          handlers = ["tls", "http"]
        },
        {
          port     = 80
          handlers = ["http"]
        }
      ]
      "protocol" : "tcp",
      "internal_port" : 80
    },
  ]
  cpus = 1
  memorymb = 256
}
