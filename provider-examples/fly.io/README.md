# Example: Deploying to fly.io

This example is based on https://fly.io/docs/app-guides/terraform-iac-getting-started/

To start, [install the `flyctl` CLI](https://fly.io/docs/hands-on/install-flyctl/), log in, and create an app.

Then `terraform init` to install the necessary providers.

Then `terraform apply` and provide your app name to build and deploy the example app to Fly.io in two locations.

When complete, your app will be available `https://<your-app>.fly.dev`.

To clean up created resources, `terraform destroy`.
