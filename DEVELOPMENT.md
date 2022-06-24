To run tests:

```
TF_ACC=1 go test ./internal/provider/...
```

---

Iterating locally:

This relies on https://www.terraform.io/cli/config/config-file#implied-local-mirror-directories

```
KO_DOCKER_REPO=gcr.io/jason-chainguard 
rm .terraform.lock.hcl && \
    go build -o ~/.terraform.d/plugins/registry.terraform.io/imjasonh/ko/0.0.100/darwin_arm64/terraform-provider-ko && \
    terraform init && \
    terraform apply -var project=jason-chainguard
```

Also update `version = "0.0.0"` in the .tf file.

This builds the provider code into the correct local mirror location, installs the provider using that location, 