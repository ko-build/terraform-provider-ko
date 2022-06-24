# Terraform Provider for `ko`

This is a work in progress.

The intention is to be able to define `ko` builds like this:

```
resource "ko_image" "foo" {
  importpath = "github.com/myrepo/foo"
}
```

And reference the built image by digest in other resources like this:

```
resource "google_cloud_run_service" "svc" {
  template {
    spec {
      containers {
        image = ko_image.foo.image_ref
      }
    }
  }
```

---

To test:

```
TF_ACC=1 go test ./internal/provider/...
```