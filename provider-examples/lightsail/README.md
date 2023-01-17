# Example: Deploying to AWS Lightsail

This example uses the [`aws`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) provider and its [`lightsail`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lightsail_container_service) resources.

Then `terraform init` to install the necessary providers.

Then `terraform apply` to build and deploy the example app into a new Lightsail container service.

To clean up created resources, `terraform destroy`.
