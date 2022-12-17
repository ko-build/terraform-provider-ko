# Generating docs

**The repo must be checked out into a directory named `terraform-provider-ko`.**

After that, `go generate ./...`

# Running acceptance tests

```
TF_ACC=1 go test ./internal/provider/...
```

---

# Iterating locally

This relies on https://www.terraform.io/cli/config/config-file#implied-local-mirror-directories

```
rm .terraform.lock.hcl && \
    go build -o ~/.terraform.d/plugins/registry.terraform.io/chainguard-dev/ko/0.0.100/darwin_arm64/terraform-provider-ko && \
    terraform init && \
    terraform apply
```

Also update `version = "0.0.0"` in the .tf file.

This builds the provider code into the correct local mirror location, installs the provider using that location,

Don't forget to delete the provider from the local mirror if you want to use the released provider later.
