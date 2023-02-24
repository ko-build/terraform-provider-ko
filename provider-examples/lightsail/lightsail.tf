terraform {
  required_providers {
    ko = {
      source = "ko-build/ko"
    }
  }
}

variable "region" {
  default = "us-west-2"
}

provider "aws" {
  region = var.region
}

provider "ko" {
  // This is added as a check that `repo` works below, it should never be used.
  docker_repo = "example.com"
}

resource "aws_ecr_repository" "foo" {
  name                 = "github.com/ko-build/terraform-provider-ko/provider-examples/lightsail"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = false
  }
}

resource "ko_build" "image" {
  repo        = aws_ecr_repository.foo.repository_url
  base_image  = "cgr.dev/chainguard/static:latest-glibc"
  importpath  = "github.com/ko-build/terraform-provider-ko/cmd/test"
  working_dir = path.module
  // Disable SBOM generation due to
  // https://github.com/ko-build/ko/issues/878
  sbom = "none"
}

resource "aws_lightsail_container_service" "example" {
  name  = "terraform-provider-ko"
  power = "nano"
  scale = 1

  private_registry_access {
    ecr_image_puller_role {
      is_active = true
    }
  }

  tags = {
    Name = "example-lightsail-service"
  }
}

resource "aws_ecr_repository_policy" "lightsail_ecr_download" {
  repository = aws_ecr_repository.foo.name

  policy = jsonencode({
    "Version": "2012-10-17",
    "Statement": [
      {
        "Sid": "AllowLightsailPull",
        "Effect": "Allow",
        "Principal": {
          "AWS": "${aws_lightsail_container_service.example.private_registry_access[0].ecr_image_puller_role[0].principal_arn}"
        },
        "Action": [
          "ecr:BatchGetImage",
          "ecr:GetDownloadUrlForLayer"
        ]
      }
    ]
  })
}

resource "aws_lightsail_container_service_deployment_version" "example" {
  container {
    container_name = "hello-world"
    image          = ko_build.image.image_ref

    ports = {
      8080 = "HTTP"
    }
  }

  public_endpoint {
    container_name = "hello-world"
    container_port = 8080

    health_check {
      healthy_threshold   = 2
      unhealthy_threshold = 2
      timeout_seconds     = 2
      interval_seconds    = 5
      path                = "/"
      success_codes       = "200-499"
    }
  }

  service_name = aws_lightsail_container_service.example.name
}

output "url" {
  value = aws_lightsail_container_service.example.url
}
