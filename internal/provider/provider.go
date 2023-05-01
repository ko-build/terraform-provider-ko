package provider

import (
	"context"
	"strings"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/validators"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ provider.Provider = &Provider{}

type Provider struct {
	version string
	repo    string
}

// ProviderModel describes the provider data model.
type ProviderModel struct { //nolint:revive
	Repo      types.String `tfsdk:"repo"`
	BasicAuth types.String `tfsdk:"basic_auth"`

	// TODO: default base image
	// TODO: default platforms
}

func (p *Provider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ko"
	resp.Version = p.version
}

func (p *Provider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"repo": schema.StringAttribute{
				Description: "Container repository to publish images to. Defaults to `KO_DOCKER_REPO` env var",
				Optional:    true,
				Validators:  []validator.String{validators.RepoValidator{}},
			},
			"basic_auth": schema.StringAttribute{
				Description: "Basic auth to use to authorize requests",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}
func (p *Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// This is only for testing, so we can inject provider config
	if p.repo != "" {
		data.Repo = basetypes.NewStringValue(p.repo)
	}

	opts := Opts{
		repo:    data.Repo.ValueString(),
		version: p.version,
	}

	if basic := data.BasicAuth.ValueString(); basic != "" {
		user, pass, ok := strings.Cut(basic, ":")
		if !ok {
			resp.Diagnostics.AddError("Client Error", "basic_auth must be in the form of `user:pass`")
			return
		}
		opts.auth = &authn.Basic{
			Username: user,
			Password: pass,
		}
	}

	resp.ResourceData = opts
	resp.DataSourceData = opts
}

func (p *Provider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewBuildResource,
		NewResolveResource,
	}
}

func (p *Provider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &Provider{
			version: version,
		}
	}
}

type Opts struct {
	version string
	repo    string
	auth    *authn.Basic
}
