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

variable "subnet" {
  type = string
}

provider "aws" {
  region = var.region
}

provider "ko" {
  // This is added as a check that `repo` works below, it should never be used.
  docker_repo = "example.com"
}

resource "aws_ecr_repository" "foo" {
  name                 = "github.com/ko-build/terraform-provider-ko/provider-examples/ecs"
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

resource "aws_iam_role" "example" {
  name = "terraform-ecs-ko"

  assume_role_policy = jsonencode({
    "Version": "2012-10-17",
    "Statement": [
      {
        "Effect": "Allow",
        "Principal": {
          "Service": "ecs.amazonaws.com"
        },
        "Action": "sts:AssumeRole"
      },{
        "Effect": "Allow",
        "Principal": {
          "Service": "ecs-tasks.amazonaws.com"
        },
        "Action": "sts:AssumeRole"
      }
    ]
  })
}

// From https://aws.amazon.com/premiumsupport/knowledge-center/ecs-tasks-pull-images-ecr-repository/
resource "aws_iam_role_policy_attachment" "allow_ecs" {
  role       = aws_iam_role.example.id
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_ecs_cluster" "cluster" {
  name = "tf-cluster"
}

resource "aws_ecs_service" "foo" {
  name            = "foo"
  cluster         = aws_ecs_cluster.cluster.id
  task_definition = aws_ecs_task_definition.foo.arn
  desired_count   = 2
  capacity_provider_strategy {
    base              = 1
    capacity_provider = "FARGATE"
    weight            = 100
  }
  network_configuration {
    assign_public_ip = true
    subnets = [var.subnet]
  }
}

resource "aws_ecs_task_definition" "foo" {
  family                   = "foo"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.example.arn
  cpu                      = 1024
  memory                   = 2048
  container_definitions    = jsonencode([
    {
      "name": "foo",
      "image": ko_build.image.image_ref,
      "cpu": 1024,
      "memory": 2048,
      "essential": true
    }
  ])
}

resource "aws_ecs_cluster_capacity_providers" "cluster" {
  cluster_name = aws_ecs_cluster.cluster.name
  capacity_providers = ["FARGATE"]
  default_capacity_provider_strategy {
    base              = 1
    weight            = 100
    capacity_provider = "FARGATE"
  }
}

