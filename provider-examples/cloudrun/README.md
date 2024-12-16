# Example: Deploying to Google Cloud Run

This example uses the [`google`](https://registry.terraform.io/providers/hashicorp/google/latest/docs) provider and its [`cloud_run_service`](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_service) resource.

To start, `terraform init` to install the necessary providers.

Then `terraform apply` to build and deploy the example app to Cloud Run.
You will be prompted for your GCP project.

> Note: Cloud Run requires that images are pushed to Google Artifact Registry to be deployed to Cloud Run.

When complete, your service will be named `tf-ko-example`.

To clean up created resources, `terraform destroy`.
