package provider

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"github.com/chrismellard/docker-credential-acr-env/pkg/credhelper"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/github"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands/options"
	"github.com/google/ko/pkg/publish"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &BuildResource{}
var _ resource.ResourceWithImportState = &BuildResource{}

func NewBuildResource() resource.Resource {
	return &BuildResource{}
}

// BuildResource defines the resource implementation.
type BuildResource struct {
	popts Opts
}

// BuildResourceModel describes the resource data model.
type BuildResourceModel struct {
	// Inputs
	Importpath types.String `tfsdk:"importpath"`
	WorkingDir types.String `tfsdk:"working_dir"`
	Platforms  types.List   `tfsdk:"platforms"`
	BaseImage  types.String `tfsdk:"base_image"`
	SBOM       types.String `tfsdk:"sbom"`
	Repo       types.String `tfsdk:"repo"`

	// Computed attributes
	ID       types.String `tfsdk:"id"`
	ImageRef types.String `tfsdk:"image_ref"`

	// Set based on provider opts, see update() below.
	repo     string
	bare     bool
	keychain authn.Keychain
	version  string
}

func (r *BuildResourceModel) update(popts Opts) {
	r.version = popts.version
	if !r.Repo.IsNull() { // ko_build.repo being set takes precedence over provider opts, and makes it bare
		r.repo = r.Repo.ValueString()
		r.bare = true
	} else if popts.repo != "" { // provider opts takes precedent over env var
		r.repo = popts.repo
	} else { // env var is last resort
		r.repo = os.Getenv("KO_DOCKER_REPO")
	}

	if popts.auth != nil {
		r.keychain = authn.NewMultiKeychain(staticKeychain{
			repo: r.repo,
			b:    popts.auth,
		}, keychain)
	} else {
		r.keychain = keychain
	}
}

func (r *BuildResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_build"
}

func (r *BuildResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"importpath": schema.StringAttribute{
				Description:   "import path to build",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"working_dir": schema.StringAttribute{
				Description:   "working directory to build from",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"platforms": schema.ListAttribute{
				Description:   "platforms to build for",
				Optional:      true,
				Computed:      true,
				ElementType:   basetypes.StringType{},
				Default:       listdefault.StaticValue(types.ListValueMust(basetypes.StringType{}, []attr.Value{basetypes.NewStringValue("linux/amd64")})),
				PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
				// TODO: validate platforms here.
			},
			"base_image": schema.StringAttribute{
				Description:   "base image to use",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(defaultBaseImage),
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"sbom": schema.StringAttribute{
				Description:   "The SBOM media type to use (none will disable SBOM synthesis and upload, also supports: spdx, cyclonedx, go.version-m).",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("spdx"),
				Validators:    []validator.String{sbomValidator{}},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"repo": schema.StringAttribute{
				Description:   "Container repository to publish images to. If set, this overrides the provider's docker_repo, and the image name will be exactly the specified `repo`, without the importpath appended.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},

			"image_ref": schema.StringAttribute{
				Description: "The image reference of the built image.",
				Computed:    true,
			},
			"id": schema.StringAttribute{
				Description: "The ID of the built image.",
				Computed:    true,
			},
		},
	}
}

func (r *BuildResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	popts, ok := req.ProviderData.(Opts)
	if !ok {
		resp.Diagnostics.AddError("Client Error", "invalid provider data")
		return
	}
	r.popts = popts
}

func (r *BuildResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.update(r.popts)

	digest, diags := doBuild(ctx, *data)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	data.ID = types.StringValue(digest)
	data.ImageRef = types.StringValue(digest)

	tflog.Trace(ctx, "created a resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.update(r.popts)

	digest, diags := doBuild(ctx, *data)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	if digest != data.ImageRef.ValueString() {
		data.ID = types.StringValue("")
		data.ImageRef = types.StringValue("")
	} else {
		data.ID = types.StringValue(digest)
		data.ImageRef = types.StringValue(digest)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.update(r.popts)

	digest, diags := doBuild(ctx, *data)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	data.ID = types.StringValue(digest)
	data.ImageRef = types.StringValue(digest)

	tflog.Trace(ctx, "updated a resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BuildResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *BuildResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: If we ever want to delete the image from the registry, we can do it here.
}

func (r *BuildResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

const (
	defaultBaseImage = "cgr.dev/chainguard/static"
)

var validTypes = map[string]struct{}{
	"spdx":         {},
	"cyclonedx":    {},
	"go.version-m": {},
	"none":         {},
}

var (
	amazonKeychain authn.Keychain = authn.NewKeychainFromHelper(ecr.NewECRHelper(ecr.WithLogger(io.Discard)))
	azureKeychain  authn.Keychain = authn.NewKeychainFromHelper(credhelper.NewACRCredentialsHelper())
	keychain                      = authn.NewMultiKeychain(
		authn.DefaultKeychain,
		amazonKeychain,
		google.Keychain,
		github.Keychain,
		azureKeychain,
	)
)

func makeBuilder(ctx context.Context, data BuildResourceModel) (*build.Caching, diag.Diagnostics) {
	var platforms []string
	if data.Platforms.IsNull() {
		platforms = []string{"linux/amd64"}
	} else {
		if diag := data.Platforms.ElementsAs(ctx, &platforms, false); diag.HasError() {
			return nil, diag
		}
	}

	bo := []build.Option{
		build.WithPlatforms(platforms...),
		build.WithBaseImages(func(ctx context.Context, s string) (name.Reference, build.Result, error) {
			baseImage := data.BaseImage.ValueString()
			ref, err := name.ParseReference(baseImage)
			if err != nil {
				return nil, nil, err
			}

			if cached, found := baseImages.Load(baseImage); found {
				return ref, cached.(build.Result), nil
			}

			desc, err := remote.Get(ref,
				// TODO(jason): Using the context here causes a "context canceled" error getting images from a base index.
				// remote.WithContext(ctx),
				remote.WithAuthFromKeychain(data.keychain),
				remote.WithUserAgent(fmt.Sprintf("terraform-provider-ko/%s", data.version)),
			)
			if err != nil {
				return nil, nil, err
			}
			if desc.MediaType.IsImage() {
				img, err := desc.Image()
				baseImages.Store(baseImage, img)
				return ref, img, err
			}
			if desc.MediaType.IsIndex() {
				idx, err := desc.ImageIndex()
				baseImages.Store(baseImage, idx)
				return ref, idx, err
			}
			return nil, nil, fmt.Errorf("unexpected base image media type: %s", desc.MediaType)
		}),
	}
	switch data.SBOM.ValueString() {
	case "spdx":
		bo = append(bo, build.WithSPDX(data.version))
	case "cyclonedx":
		bo = append(bo, build.WithCycloneDX())
	case "go.version-m":
		bo = append(bo, build.WithGoVersionSBOM())
	case "none":
		bo = append(bo, build.WithDisabledSBOM())
	default:
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("invalid sbom type", data.SBOM.ValueString())}
	}

	b, err := build.NewGo(ctx, data.WorkingDir.ValueString(), bo...)
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("build.NewGo", err.Error())}
	}
	dig, err := build.NewCaching(b)
	if err != nil {
		return nil, diag.Diagnostics{diag.NewErrorDiagnostic("build.NewCaching", err.Error())}
	}
	return dig, nil
}

var baseImages sync.Map // Cache of base image lookups.

func doBuild(ctx context.Context, data BuildResourceModel) (string, diag.Diagnostics) {
	if data.repo == "" {
		return "", diag.Diagnostics{diag.NewErrorDiagnostic("Client Error",
			"one of KO_DOCKER_REPO env var, or provider `repo`, or ko_build resource `repo` must be set")}
	}
	b, diags := makeBuilder(ctx, data)
	if diags.HasError() {
		return "", diags
	}
	r, err := b.Build(ctx, data.Importpath.ValueString())
	if err != nil {
		return "", diag.Diagnostics{diag.NewErrorDiagnostic("build", err.Error())}
	}

	p, err := publish.NewDefault(data.repo,
		publish.WithAuthFromKeychain(data.keychain),
		publish.WithUserAgent(fmt.Sprintf("terraform-provider-ko/%s", data.version)),
		publish.WithNamer(options.MakeNamer(&options.PublishOptions{
			DockerRepo:          data.repo,
			Bare:                data.bare,
			PreserveImportPaths: !data.bare,
			BaseImportPaths:     false,
		})),
	)
	if err != nil {
		return "", diag.Diagnostics{diag.NewErrorDiagnostic("publish.NewDefault", err.Error())}
	}
	ref, err := p.Publish(ctx, r, data.Importpath.ValueString())
	if err != nil {
		return "", diag.Diagnostics{diag.NewErrorDiagnostic("publish", err.Error())}
	}
	return ref.String(), nil
}

type staticKeychain struct {
	repo string
	b    *authn.Basic
}

func (k staticKeychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
	ref, err := name.ParseReference(k.repo)
	if err != nil {
		return nil, err
	}
	if r.RegistryStr() == ref.Context().RegistryStr() {
		return staticAuthenticator{k.b}, nil
	}
	return authn.Anonymous, nil
}

type staticAuthenticator struct{ b *authn.Basic }

func (a staticAuthenticator) Authorization() (*authn.AuthConfig, error) {
	return &authn.AuthConfig{
		Username: a.b.Username,
		Password: a.b.Password,
	}, nil
}
