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
  name                 = "github.com/ko-build/terraform-provider-ko/provider-examples/lambda"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = false
  }
}

resource "ko_build" "image" {
  repo        = aws_ecr_repository.foo.repository_url
  importpath  = "github.com/ko-build/terraform-provider-ko/cmd/test-lambda"
  working_dir = path.module
  // Disable SBOM generation due to
  // https://github.com/ko-build/ko/issues/878
  sbom = "none"
}

resource "aws_iam_role" "lambda_access_role" {
  name = "lambda_access_role"

  assume_role_policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Principal" : {
          "Service" : "lambda.amazonaws.com"
        },
        "Action" : "sts:AssumeRole"
      }
    ]
  })
}

resource "aws_lambda_function" "example" {
  function_name = "terraform-provider-ko-sample"
  role          = aws_iam_role.lambda_access_role.arn
  package_type  = "Image"
  image_uri     = ko_build.image.image_ref

  tags = {
    Name = "example-lambda-function"
  }
}
