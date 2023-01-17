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
  name                 = "github.com/ko-build/terraform-provider-ko/provider-examples/apprunner"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = false
  }
}

resource "ko_image" "image" {
  repo        = aws_ecr_repository.foo.repository_url
  base_image  = "cgr.dev/chainguard/static:latest-glibc"
  importpath  = "github.com/ko-build/terraform-provider-ko/cmd/test"
  working_dir = path.module
  // Disable SBOM generation due to
  // https://github.com/ko-build/ko/issues/878
  sbom = "none"
}

resource "aws_iam_role" "apprunner_access_role" {
  name = "apprunner_access_role"

  assume_role_policy = jsonencode({
    "Version": "2012-10-17",
    "Statement": [
      {
        "Effect": "Allow",
        "Principal": {
          "Service": "build.apprunner.amazonaws.com"
        },
        "Action": "sts:AssumeRole"
      }
    ]
  })

  inline_policy {
    name   = "can-pull-ecr"
    policy = jsonencode({
      "Version": "2012-10-17",
      "Statement": [
        {
          "Action": [
            "ecr:GetDownloadUrlForLayer",
            "ecr:BatchGetImage",
            "ecr:DescribeImages",
            "ecr:GetAuthorizationToken",
            "ecr:BatchCheckLayerAvailability",
          ],
          "Resource": "*",
          "Effect": "Allow"
        }
      ]
    })
  }
}

resource "aws_apprunner_service" "example" {
  service_name = "example"

  source_configuration {
    authentication_configuration {
      access_role_arn = aws_iam_role.apprunner_access_role.arn
    }

    image_repository {
      image_identifier      = ko_image.image.image_ref
      image_repository_type = "ECR"
    }
    auto_deployments_enabled = false
  }

  tags = {
    Name = "example-apprunner-service"
  }
}

output "apprunner-url" {
  value = aws_apprunner_service.example.service_url
}
