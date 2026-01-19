# Test ko provider locally

Test local changes to the terraform-provider-ko against a real registry.

## Prerequisites

The test uses ttl.sh, a free ephemeral container registry that requires no authentication. Images pushed there expire automatically.

## Steps

1. Build the provider and install to local mirror:
   ```bash
   go build -o ~/.terraform.d/plugins/registry.terraform.io/ko-build/ko/0.0.100/darwin_arm64/terraform-provider-ko .
   ```

2. Create a temporary test directory with a minimal Terraform config:
   ```bash
   TEST_DIR=$(mktemp -d)
   cat > "$TEST_DIR/main.tf" << 'EOF'
   terraform {
     required_providers {
       ko = {
         source  = "ko-build/ko"
         version = "0.0.100"
       }
     }
   }

   provider "ko" {
     repo = "ttl.sh/terraform-provider-ko-test"
   }

   resource "ko_build" "test" {
     importpath = "github.com/google/ko"
   }

   output "image_ref" {
     value = ko_build.test.image_ref
   }
   EOF
   echo "Test directory: $TEST_DIR"
   ```

3. Initialise Terraform:
   ```bash
   cd "$TEST_DIR" && rm -f .terraform.lock.hcl && terraform init
   ```

4. Run terraform plan:
   ```bash
   cd "$TEST_DIR" && terraform plan -out tf.out
   ```

5. If the plan looks good, apply it:
   ```bash
   cd "$TEST_DIR" && terraform apply tf.out
   ```

6. Verify the image was pushed by checking the output shows a valid image reference.

7. Clean up (removes test directory and provider from local mirror):
   ```bash
   rm -rf "$TEST_DIR"
   rm -f ~/.terraform.d/plugins/registry.terraform.io/ko-build/ko/0.0.100/darwin_arm64/terraform-provider-ko
   ```

## Debugging

To see detailed provider logs, set `TF_LOG=TRACE`:
```bash
cd "$TEST_DIR" && TF_LOG=TRACE terraform apply -auto-approve 2>&1 | tee /tmp/ko-provider-test.log
```

Then examine the logs:
```bash
less /tmp/ko-provider-test.log
```
