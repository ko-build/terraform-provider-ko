# Example: Deploying to AWS Lambda

This example uses the [`aws`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs) provider and its [`lambda`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lambda_function) resources.

Then `terraform init` to install the necessary providers.

Then `terraform apply` to build and deploy the example app into a new lambda function.

To clean up created resources, `terraform destroy`.
