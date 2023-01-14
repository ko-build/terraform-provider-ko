# Example: Deploying to AWS App Runner

This example uses the [`aws`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) provider and its [`apprunner`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/apprunner_service) resources.

Then `terraform init` to install the necessary providers.

Then `terraform apply` to build and deploy the example app into a new AppRunner service.

To clean up created resources, `terraform destroy`.
