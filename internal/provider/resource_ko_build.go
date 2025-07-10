package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"github.com/chrismellard/docker-credential-acr-env/pkg/credhelper"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/github"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands/options"
	"github.com/google/ko/pkg/publish"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	version   = "devel"
	userAgent = "terraform-provider-ko"
)

var validTypes = map[string]struct{}{
	"spdx": {},
	"none": {},
}

func resourceBuild() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Sample resource in the Terraform provider scaffolding.",

		CreateContext: resourceKoBuildCreate,
		ReadContext:   resourceKoBuildRead,
		DeleteContext: resourceKoBuildDelete,

		SchemaVersion: 1,

		Schema: map[string]*schema.Schema{
			"importpath": {
				Description: "import path to build",
				Type:        schema.TypeString,
				Required:    true,
				ValidateDiagFunc: func(_ interface{}, _ cty.Path) diag.Diagnostics {
					// TODO: validate stuff here.
					return nil
				},
				ForceNew: true, // Any time this changes, don't try to update in-place, just create it.
			},
			"working_dir": {
				Description: "working directory for the build",
				Optional:    true,
				Default:     ".",
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"platforms": {
				Description: "Which platform to use when pulling a multi-platform base. Format: all | <os>[/<arch>[/<variant>]][,platform]*",
				Optional:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"base_image": {
				Description: "base image to use",
				Default:     "",
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"sbom": {
				Description: "The SBOM media type to use (none will disable SBOM synthesis and upload).",
				Default:     "spdx",
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
				ValidateDiagFunc: func(data interface{}, _ cty.Path) diag.Diagnostics {
					v := data.(string)
					if _, found := validTypes[v]; !found {
						return diag.Errorf("Invalid sbom type: %q", v)
					}
					return nil
				},
			},
			"repo": {
				Description: "Container repository to publish images to. If set, this overrides the provider's `repo`, and the image name will be exactly the specified `repo`, without the importpath appended.",
				Default:     "",
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"image_ref": {
				Description: "built image reference by digest",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"ldflags": {
				Description: "Extra ldflags to pass to the go build",
				Optional:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"env": {
				Description: "Extra environment variables to pass to the go build",
				Optional:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"tags": {
				Description: "Which tags to use for the produced image instead of the default 'latest' tag",
				Optional:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
		},
	}
}

type buildOptions struct {
	ip         string
	workingDir string
	imageRepo  string // The image's repo, either from the KO_DOCKER_REPO env var, or provider-configured dockerRepo/repo, or image resource's repo.
	platforms  []string
	baseImage  string
	sbom       string
	auth       *authn.Basic
	bare       bool     // If true, use the "bare" namer that doesn't append the importpath.
	ldflags    []string // Extra ldflags to pass to the go build.
	env        []string // Extra environment variables to pass to the go build.
	tags       []string // Which tags to use for the produced image instead of the default 'latest'
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

func (o *buildOptions) makeBuilder(ctx context.Context) (*build.Caching, error) {
	bo := []build.Option{
		build.WithTrimpath(true),
		build.WithPlatforms(o.platforms...),
		build.WithConfig(map[string]build.Config{
			o.ip: {
				Ldflags: o.ldflags,
				Env:     o.env,
			}}),
		build.WithBaseImages(func(_ context.Context, _ string) (name.Reference, build.Result, error) {
			ref, err := name.ParseReference(o.baseImage)
			if err != nil {
				return nil, nil, err
			}

			if cached, found := baseImages.Load(o.baseImage); found {
				return ref, cached.(build.Result), nil
			}

			kc := keychain
			if o.auth != nil {
				kc = authn.NewMultiKeychain(staticKeychain{o.imageRepo, o.auth}, kc)
			}
			desc, err := remote.Get(ref,
				remote.WithAuthFromKeychain(kc),
				remote.WithUserAgent(userAgent),
			)
			if err != nil {
				return nil, nil, err
			}
			if desc.MediaType.IsImage() {
				img, err := desc.Image()
				baseImages.Store(o.baseImage, img)
				return ref, img, err
			}
			if desc.MediaType.IsIndex() {
				idx, err := desc.ImageIndex()
				baseImages.Store(o.baseImage, idx)
				return ref, idx, err
			}
			return nil, nil, fmt.Errorf("unexpected base image media type: %s", desc.MediaType)
		}),
	}

	switch o.sbom {
	case "spdx":
		bo = append(bo, build.WithSPDX(version))
	case "none":
		bo = append(bo, build.WithDisabledSBOM())
	default:
		return nil, fmt.Errorf("unknown sbom type: %q", o.sbom)
	}

	// We read the environment variable directly here instead of plumbing it through as a provider option to keep the behavior consistent with resolve.
	// While CreationTime is a build.Option, it is not a field in options.BuildOptions and is inferred from the environment variable when a new resolver is created.
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		s, err := strconv.ParseInt(epoch, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("the environment variable %s should be the number of seconds since January 1st 1970, 00:00 UTC, got: %w", epoch, err)
		}
		bo = append(bo, build.WithCreationTime(v1.Time{Time: time.Unix(s, 0)}))
	}

	b, err := build.NewGo(ctx, o.workingDir, bo...)
	if err != nil {
		return nil, fmt.Errorf("NewGo: %w", err)
	}
	return build.NewCaching(b)
}

var baseImages sync.Map // Cache of base image lookups.

// doBuild builds the image and returns the built image, and the full name.Reference by digest that the image would be pushed to.
//
// doBuild doesn't publish images, use doPublish to publish the build.Result that doBuild returns.
func doBuild(ctx context.Context, opts buildOptions, includeTag bool) (build.Result, string, error) {
	if opts.imageRepo == "" {
		return nil, "", errors.New("one of KO_DOCKER_REPO env var, or provider `repo`, or image resource `repo` must be set")
	}

	b, err := opts.makeBuilder(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("NewGo: %w", err)
	}
	res, err := b.Build(ctx, opts.ip)
	if err != nil {
		return nil, "", fmt.Errorf("build: %w", err)
	}
	dig, err := res.Digest()
	if err != nil {
		return nil, "", fmt.Errorf("digest: %w", err)
	}
	ref, err := name.ParseReference(namer(opts)(opts.imageRepo, opts.ip))
	if err != nil {
		return nil, "", fmt.Errorf("ParseReference: %w", err)
	}

	if includeTag && len(opts.tags) == 1 {
		// Return the tagged reference with digest appended (same format as doPublish)
		taggedRef := ref.Context().Tag(opts.tags[0])
		return res, taggedRef.String() + "@" + dig.String(), nil
	}

	return res, ref.Context().Digest(dig.String()).String(), nil
}

func namer(opts buildOptions) publish.Namer {
	return options.MakeNamer(&options.PublishOptions{
		DockerRepo:          opts.imageRepo,
		Bare:                opts.bare,
		PreserveImportPaths: !opts.bare,
		Tags:                opts.tags,
	})
}

func doPublish(ctx context.Context, r build.Result, opts buildOptions) (string, error) {
	kc := keychain
	if opts.auth != nil {
		kc = authn.NewMultiKeychain(staticKeychain{opts.imageRepo, opts.auth}, kc)
	}

	po := []publish.Option{
		publish.WithAuthFromKeychain(kc),
		publish.WithNamer(namer(opts)),
		publish.WithUserAgent(userAgent),
	}

	if len(opts.tags) > 0 {
		po = append(po, publish.WithTags(opts.tags))
	}

	p, err := publish.NewDefault(opts.imageRepo, po...)
	if err != nil {
		return "", fmt.Errorf("NewDefault: %w", err)
	}
	ref, err := p.Publish(ctx, r, opts.ip)
	if err != nil {
		return "", fmt.Errorf("publish: %w", err)
	}
	return ref.String(), nil
}

func fromData(d *schema.ResourceData, po *Opts) buildOptions {
	// Use the repo configured in the ko_build resource, if set.
	// Otherwise, fallback to the provider-configured repo.
	// If the ko_build resource configured the repo, use bare image naming.
	repo := po.po.DockerRepo
	bare := false
	if r := d.Get("repo").(string); r != "" {
		repo = r
		bare = true
	}

	return buildOptions{
		ip:         d.Get("importpath").(string),
		workingDir: d.Get("working_dir").(string),
		imageRepo:  repo,
		platforms:  defaultPlatform(toStringSlice(d.Get("platforms").([]interface{}))),
		baseImage:  getString(d, "base_image", po.bo.BaseImage),
		sbom:       d.Get("sbom").(string),
		auth:       po.auth,
		bare:       bare,
		ldflags:    toStringSlice(d.Get("ldflags").([]interface{})),
		env:        toStringSlice(d.Get("env").([]interface{})),
		tags:       toStringSlice(d.Get("tags").([]interface{})),
	}
}

func getString(d *schema.ResourceData, key string, defaultVal string) string {
	if v, ok := d.Get(key).(string); ok && v != "" {
		return v
	}
	return defaultVal
}

func defaultPlatform(in []string) []string {
	if len(in) == 0 {
		return []string{"linux/amd64"}
	}
	return in
}

func toStringSlice(in []interface{}) []string {
	out := make([]string, len(in))
	for i, ii := range in {
		if s, ok := ii.(string); ok {
			out[i] = s
		} else {
			panic(fmt.Errorf("expected string, got %T", ii))
		}
	}
	return out
}

func resourceKoBuildCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	po, err := NewProviderOpts(meta)
	if err != nil {
		return diag.Errorf("configuring provider: %v", err)
	}

	res, _, err := doBuild(ctx, fromData(d, po), false)
	if err != nil {
		return diag.Errorf("[id=%s] create doBuild: %v", d.Id(), err)
	}
	ref, err := doPublish(ctx, res, fromData(d, po))
	if err != nil {
		return diag.Errorf("[id=%s] create doPublish: %v", d.Id(), err)
	}

	_ = d.Set("image_ref", ref)
	d.SetId(ref)
	return nil
}

const zeroRef = "example.com/zero@sha256:0000000000000000000000000000000000000000000000000000000000000000"

func resourceKoBuildRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	po, err := NewProviderOpts(meta)
	if err != nil {
		return diag.Errorf("configuring provider: %v", err)
	}

	var diags diag.Diagnostics
	_, ref, err := doBuild(ctx, fromData(d, po), true)
	if err != nil {
		ref = zeroRef
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Summary:  "Image build failed to read -- create may fail.",
			Detail:   fmt.Sprintf("failed to read image: %v", err),
		})
	}

	_ = d.Set("image_ref", ref)
	if ref != d.Id() || ref == zeroRef {
		d.SetId("") // triggers create on next apply.
	} else {
		d.SetId(ref)
	}
	return diags
}

func resourceKoBuildDelete(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	// TODO: If we ever want to delete the image from the registry, we can do it here.
	return nil
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
