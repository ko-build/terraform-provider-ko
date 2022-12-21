# Example: Deploying to Kubernetes

This example uses the [`kubernetes`](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs) provider and its [`kubernetes_deployment`](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/deployment) resource.

To start, set up a Kubernetes cluster.

Then `terraform init` to install the necessary providers.

Then `terraform apply` to build and deploy the example app to the current Kubernetes context.

When complete, your deployment will start in the `tf-ko-example` namespace.

To clean up created resources, `terraform destroy`.
