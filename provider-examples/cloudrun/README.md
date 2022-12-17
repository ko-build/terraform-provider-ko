# Example: Deploying to Google Cloud Run

This example is based on the `noauth` example: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_service#example-usage---cloud-run-service-noauth

To start, [install `gcloud`](https://cloud.google.com/sdk/docs/install/), and log in:

```
gcloud auth login
gcloud auth application-default login
```

Then `terraform init` to install the necessary providers.

Then `terraform apply` and provide your project name to build and deploy the example app to Cloud Run.

When complete, your service will be available at the output URL.

To clean up created resources, `terraform destroy`.
