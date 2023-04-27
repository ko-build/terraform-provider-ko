package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// sbomValidator is a string validator that checks that the string is a valid SBOM type.
type sbomValidator struct{}

var _ validator.String = sbomValidator{}

func (v sbomValidator) Description(context.Context) string             { return "value must be a valid SBOM type" }
func (v sbomValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v sbomValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()

	if _, found := validTypes[val]; !found {
		resp.Diagnostics.AddError("Client Error", "Invalid sbom type")
	}
}
