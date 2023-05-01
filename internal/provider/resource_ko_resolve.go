package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands"
	"github.com/google/ko/pkg/commands/options"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"gopkg.in/yaml.v2"
)

var _ resource.Resource = &ResolveResource{}
var _ resource.ResourceWithImportState = &ResolveResource{}

func NewResolveResource() resource.Resource {
	return &ResolveResource{}
}

// ResolveResource defines the resource implementation.
type ResolveResource struct {
	popts Opts
}

// ResolveResourceModel describes the resource data model.
type ResolveResourceModel struct {
	// Inputs
	Filenames  types.List   `tfsdk:"filenames"`
	Recursive  types.Bool   `tfsdk:"recursive"`
	Push       types.Bool   `tfsdk:"push"`
	Selector   types.String `tfsdk:"selector"`
	Platforms  types.List   `tfsdk:"platforms"`
	SBOM       types.String `tfsdk:"sbom"`
	BaseImage  types.String `tfsdk:"base_image"`
	Tags       types.List   `tfsdk:"tags"`
	WorkingDir types.String `tfsdk:"working_dir"`

	// Computed attributes
	ID        types.String `tfsdk:"id"`
	Manifests types.List   `tfsdk:"manifests"`

	// Set based on provider opts, see update() below.
	repo     string
	keychain authn.Keychain
	version  string
}

func (r *ResolveResourceModel) update(popts Opts) {
	r.version = popts.version
	r.repo = popts.repo
	if r.repo == "" { // env var is last resort
		r.repo = os.Getenv("KO_DOCKER_REPO")
	}

	// TODO: Because of how ko's PublishOptions are defined, we can't
	// actually inject this keychain, so basic_auth is effectively unused for ko_resolve.
	if popts.auth != nil {
		r.keychain = authn.NewMultiKeychain(staticKeychain{
			repo: r.repo,
			b:    popts.auth,
		}, keychain)
	} else {
		r.keychain = keychain
	}
}

func (r *ResolveResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resolve"
}

func (r *ResolveResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"filenames": schema.ListAttribute{
				Description:   "Filenames, directories, or URLs to files to use to create the resource",
				Required:      true,
				ElementType:   basetypes.StringType{},
				PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
			},
			"recursive": schema.BoolAttribute{
				Description:   "Process the directory used in -f, --filename recursively. Useful when you want to manage related manifests organized within the same directory.",
				Optional:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"push": schema.BoolAttribute{
				Description:   "Push images to KO_DOCKER_REPO",
				Optional:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"selector": schema.StringAttribute{
				Description:   "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"platforms": schema.ListAttribute{
				Description:   "Platforms to build for, comma separated. e.g. linux/amd64,linux/arm64",
				Optional:      true,
				Computed:      true,
				ElementType:   basetypes.StringType{},
				Default:       listdefault.StaticValue(types.ListValueMust(basetypes.StringType{}, []attr.Value{basetypes.NewStringValue("linux/amd64")})),
				PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
			},
			"sbom": schema.StringAttribute{
				Description:   "The SBOM media type to use (none will disable SBOM synthesis and upload, also supports: spdx, cyclonedx, go.version-m).",
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("spdx"),
				Validators:    []validator.String{sbomValidator{}},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"base_image": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(defaultBaseImage),
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"tags": schema.ListAttribute{
				Description:   "Tags to apply to the image, comma separated. e.g. latest,1.0.0",
				Optional:      true,
				ElementType:   basetypes.StringType{},
				PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
			},
			"working_dir": schema.StringAttribute{
				Description:   "The working directory to use for the build context.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			// TODO(jason): add "repo" to match ko_build, with same defaulting logic.

			"id": schema.StringAttribute{
				Description: "The ID of the resource.",
				Computed:    true,
			},
			"manifests": schema.ListAttribute{
				Description: "The manifests created by the resource.",
				Computed:    true,
				ElementType: basetypes.StringType{},
			},
		},
	}
}

func (r *ResolveResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ResolveResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *ResolveResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.update(r.popts)

	res, diag := NewResolver(ctx, data)
	resp.Diagnostics.Append(diag...)
	if diag.HasError() {
		return
	}

	resolved, err := res.Resolve(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Resolve Error", err.Error())
		return
	}

	mfs := make([]attr.Value, len(resolved.Manifests))
	for i, m := range resolved.Manifests {
		mfs[i] = basetypes.NewStringValue(m)
	}
	data.Manifests, diag = basetypes.NewListValue(basetypes.StringType{}, mfs)
	resp.Diagnostics.Append(diag...)
	if diag.HasError() {
		return
	}
	data.ID = basetypes.NewStringValue(resolved.ID)

	tflog.Trace(ctx, "created a resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ResolveResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *ResolveResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.update(r.popts)

	res, diag := NewResolver(ctx, data)
	resp.Diagnostics.Append(diag...)
	if diag.HasError() {
		return
	}
	res.po.Tags = []string{} // IMPORTANT: Don't tag on reads!

	resolved, err := res.Resolve(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Resolve Error", err.Error())
		return
	}

	mfs := make([]attr.Value, len(resolved.Manifests))
	for i, m := range resolved.Manifests {
		mfs[i] = basetypes.NewStringValue(m)
	}
	data.Manifests, diag = basetypes.NewListValue(basetypes.StringType{}, mfs)
	resp.Diagnostics.Append(diag...)
	if diag.HasError() {
		return
	}
	data.ID = basetypes.NewStringValue(resolved.ID)

	tflog.Trace(ctx, "created a resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ResolveResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *ResolveResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.update(r.popts)

	res, diag := NewResolver(ctx, data)
	resp.Diagnostics.Append(diag...)
	if diag.HasError() {
		return
	}

	resolved, err := res.Resolve(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Resolve Error", err.Error())
		return
	}

	mfs := make([]attr.Value, len(resolved.Manifests))
	for i, m := range resolved.Manifests {
		mfs[i] = basetypes.NewStringValue(m)
	}
	data.Manifests, diag = basetypes.NewListValue(basetypes.StringType{}, mfs)
	resp.Diagnostics.Append(diag...)
	if diag.HasError() {
		return
	}
	data.ID = basetypes.NewStringValue(resolved.ID)

	tflog.Trace(ctx, "created a resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ResolveResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *ResolveResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: If we ever want to delete images from the registry, we can do it here.
}

func (r *ResolveResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

type ResolverInt interface {
	Resolve(ctx context.Context) (*Resolved, error)
}

type Resolved struct {
	ID        string
	Manifests []string
}

type Resolver struct {
	bo *options.BuildOptions
	po *options.PublishOptions
	fo *options.FilenameOptions
	so *options.SelectorOptions
}

func NewResolver(ctx context.Context, data *ResolveResourceModel) (*Resolver, diag.Diagnostics) {
	var platforms, tags, filenames []string
	if diag := data.Platforms.ElementsAs(ctx, &platforms, false); diag.HasError() {
		return nil, diag
	}
	if diag := data.Tags.ElementsAs(ctx, &tags, false); diag.HasError() {
		return nil, diag
	}
	if diag := data.Filenames.ElementsAs(ctx, &filenames, false); diag.HasError() {
		return nil, diag
	}
	r := &Resolver{
		bo: &options.BuildOptions{
			WorkingDirectory: data.WorkingDir.ValueString(),
			BaseImage:        data.BaseImage.ValueString(),
			SBOM:             data.SBOM.ValueString(),
			Platforms:        platforms,
		},
		po: &options.PublishOptions{
			Push:       data.Push.ValueBool(),
			Tags:       tags,
			DockerRepo: data.repo,
			UserAgent:  fmt.Sprintf("terraform-provider-ko/%s", data.version),
			// The default Namer will be used, producing images named like "app-<md5" for compatibility with Dockerhub.
		},
		fo: &options.FilenameOptions{
			Filenames: filenames,
			Recursive: data.Recursive.ValueBool(),
		},
		so: &options.SelectorOptions{
			Selector: data.Selector.ValueString(),
		},
	}
	return r, nil
}

func (r *Resolver) Resolve(ctx context.Context) (*Resolved, error) {
	builder, err := commands.NewBuilder(ctx, r.bo)
	if err != nil {
		return nil, err
	}

	cacheBuilder, err := build.NewCaching(builder)
	if err != nil {
		return nil, err
	}

	publisher, err := commands.NewPublisher(r.po)
	if err != nil {
		return nil, err
	}
	defer publisher.Close()

	var resolveBuf bytes.Buffer
	w := &nopWriteCloser{Writer: bufio.NewWriter(&resolveBuf)}

	if err := commands.ResolveFilesToWriter(ctx, cacheBuilder, publisher, r.fo, r.so, w); err != nil {
		return nil, err
	}

	if err := w.Flush(); err != nil {
		return nil, err
	}

	// Split the dump of multi-doc yaml back into their individual nodes
	// NOTE: Don't use a strings.Split to ensure we filter out any null docs
	manifests := []string{}
	decoder := yaml.NewDecoder(&resolveBuf)
	for {
		// Use an interface instead of yaml.Node to easily strip whitespaces and nil docs
		var d interface{}
		if err := decoder.Decode(&d); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if d == nil {
			continue
		}

		var buf bytes.Buffer
		if err := yaml.NewEncoder(&buf).Encode(&d); err != nil {
			return nil, err
		}
		manifests = append(manifests, buf.String())
	}

	sha256hash := sha256.Sum256(resolveBuf.Bytes())

	return &Resolved{
		ID:        fmt.Sprintf("%x", sha256hash),
		Manifests: manifests,
	}, nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////

type nopWriteCloser struct {
	*bufio.Writer
}

func (w *nopWriteCloser) Close() error {
	return nil
}
