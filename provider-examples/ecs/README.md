# Example: Deploying to ECS

This example uses the [`aws`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) provider and its [`ecs`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ecs_cluster) resources.

Then `terraform init` to install the necessary providers.

Then `terraform apply` to build and deploy the example app into a new ECS cluster and service.

Since Fargate needs to know what subnet to host things on, the terraform variable `subnet` must be specified (e.g. `TF_VAR_subnet`) or you will be prompted for it.

When complete, your service will run in a new ECS cluster named `tf-cluster`.

To clean up created resources, `terraform destroy`.
