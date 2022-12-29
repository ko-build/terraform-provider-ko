package provider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands"
	"github.com/google/ko/pkg/commands/options"
	"github.com/google/ko/pkg/publish"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	defaultBaseImage = "distroless.dev/static"
	version          = "devel"
)

var validTypes = map[string]struct{}{
	"spdx":         {},
	"cyclonedx":    {},
	"go.version-m": {},
	"none":         {},
}

func resourceImage() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Sample resource in the Terraform provider scaffolding.",

		CreateContext: resourceKoBuildCreate,
		ReadContext:   resourceKoBuildRead,
		DeleteContext: resourceKoBuildDelete,

		SchemaVersion: 1,
		StateUpgraders: []schema.StateUpgrader{
			{
				Version: 0,
				Type:    resourceImageV0().CoreConfigSchema().ImpliedType(),
				Upgrade: func(_ context.Context, rawState map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
					if rawState == nil {
						return rawState, nil
					}

					// Set defaults for new sbom attribute
					// NB: this is important to make read after update from
					// schema 0 to 1 work correctly. We build on read and we
					// need sbom format to be defined to build
					rawState[SBOMKey] = "spdx"

					// Previously "platforms" was just a string for one
					// platform so we take that single platform and put in a
					// slice to migrate
					platforms, ok := rawState[PlatformsKey].(string)
					if !ok {
						return rawState, nil
					}
					rawState[PlatformsKey] = []string{platforms}
					return rawState, nil
				},
			},
		},

		Schema: map[string]*schema.Schema{
			ImportPathKey: {
				Description: "import path to build",
				Type:        schema.TypeString,
				Required:    true,
				ValidateDiagFunc: func(data interface{}, path cty.Path) diag.Diagnostics {
					// TODO: validate stuff here.
					return nil
				},
				ForceNew: true, // Any time this changes, don't try to update in-place, just create it.
			},
			WorkingDirKey: {
				Description: "working directory for the build",
				Optional:    true,
				Default:     ".",
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			PlatformsKey: {
				Description: "Which platform to use when pulling a multi-platform base. Format: all | <os>[/<arch>[/<variant>]][,platform]*",
				Optional:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			BaseImageKey: {
				Description: "base image to use",
				Default:     defaultBaseImage,
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			SBOMKey: {
				Description: "The SBOM media type to use (none will disable SBOM synthesis and upload, also supports: spdx, cyclonedx, go.version-m).",
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
			RepoKey: {
				Description: "Container repository to publish images to. If set, this overrides the provider's docker_repo, and the image name will be exactly the specified `repo`, without the importpath appended.",
				Default:     "",
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			ImageRefKey: {
				Description: "built image reference by digest",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceImageV0() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"importpath": {
				Description: "import path to build",
				Type:        schema.TypeString,
				Required:    true,
				ValidateDiagFunc: func(data interface{}, path cty.Path) diag.Diagnostics {
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
				Description: "platforms to build",
				Default:     "linux/amd64",
				Optional:    true,
				Type:        schema.TypeString, // TODO: type list of strings?
				ForceNew:    true,              // Any time this changes, don't try to update in-place, just create it.
			},
			"base_image": {
				Description: "base image to use",
				Default:     defaultBaseImage,
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"image_ref": {
				Description: "built image reference by digest",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

type buildOpts struct {
	*options.BuildOptions
}

func (o *buildOpts) makeBuilder(ctx context.Context) (*build.Caching, error) {
	builder, err := commands.NewBuilder(ctx, o.BuildOptions)
	if err != nil {
		return nil, err
	}

	return build.NewCaching(builder)
}

type publishOpts struct {
	*options.PublishOptions
}

func (o *publishOpts) makePublisher() (publish.Interface, error) {
	return commands.NewPublisher(o.PublishOptions)
}

type buildOptions struct {
	ip           string
	workingDir   string
	koDockerRepo string // KO_DOCKER_REPO env var, or the provider's configured repo if set.
	imageRepo    string // The image's repo, if set.
	platforms    []string
	baseImage    string
	sbom         string
}

func (o *buildOptions) makeBuilder(ctx context.Context) (*build.Caching, error) {
	bo := []build.Option{
		build.WithPlatforms(o.platforms...),
		build.WithBaseImages(func(ctx context.Context, s string) (name.Reference, build.Result, error) {
			ref, err := name.ParseReference(o.baseImage)
			if err != nil {
				return nil, nil, err
			}

			if cached, found := baseImages.Load(o.baseImage); found {
				return ref, cached.(build.Result), nil
			}

			desc, err := remote.Get(ref,
				remote.WithAuthFromKeychain(authn.DefaultKeychain))
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
	case "cyclonedx":
		bo = append(bo, build.WithCycloneDX())
	case "go.version-m":
		bo = append(bo, build.WithGoVersionSBOM())
	case "none":
		// don't do anything.
	default:
		return nil, fmt.Errorf("unknown sbom type: %q", o.sbom)
	}

	b, err := build.NewGo(ctx, o.workingDir, bo...)
	if err != nil {
		return nil, fmt.Errorf("NewGo: %v", err)
	}
	return build.NewCaching(b)
}

var baseImages sync.Map // Cache of base image lookups.

func doBuild(ctx context.Context, opts buildOptions) (string, error) {
	if opts.koDockerRepo == "" && opts.imageRepo == "" {
		return "", errors.New("one of KO_DOCKER_REPO env var, or provider `docker_repo` or `repo`, or image resource `repo` must be set")
	}
	po := []publish.Option{publish.WithAuthFromKeychain(authn.DefaultKeychain)}
	var repo string
	if opts.imageRepo != "" {
		// image resource's `repo` takes precedence if set, and selects the
		// `--bare` namer so the image is named exactly `repo`.
		repo = opts.imageRepo
		po = append(po, publish.WithNamer(options.MakeNamer(&options.PublishOptions{
			DockerRepo: opts.imageRepo,
			Bare:       true,
		})))
	} else {
		repo = opts.koDockerRepo
	}

	b, err := opts.makeBuilder(ctx)
	if err != nil {
		return "", fmt.Errorf("NewGo: %v", err)
	}
	r, err := b.Build(ctx, opts.ip)
	if err != nil {
		return "", fmt.Errorf("build: %v", err)
	}

	p, err := publish.NewDefault(repo, po...)
	if err != nil {
		return "", fmt.Errorf("NewDefault: %v", err)
	}
	ref, err := p.Publish(ctx, r, opts.ip)
	if err != nil {
		return "", fmt.Errorf("publish: %v", err)
	}
	return ref.String(), nil
}

func fromData(d *schema.ResourceData, providerRepo string) buildOptions {
	return buildOptions{
		ip:           d.Get("importpath").(string),
		workingDir:   d.Get("working_dir").(string),
		koDockerRepo: providerRepo,
		imageRepo:    d.Get("repo").(string),
		platforms:    toStringSlice(d.Get("platforms").([]interface{})),
		baseImage:    d.Get("base_image").(string),
		sbom:         d.Get("sbom").(string),
	}
}

func toStringSlice(in []interface{}) []string {
	if len(in) == 0 {
		return []string{"linux/amd64"}
	}

	out := make([]string, len(in))
	for i, ii := range in {
		if s, ok := ii.(string); ok {
			out[i] = s
		} else {
			panic(fmt.Errorf("Expected string, got %T", ii))
		}
	}
	return out
}

func resourceKoBuildCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	providerOpts, err := NewProviderOpts(meta)
	if err != nil {
		return diag.Errorf("configuring provider: %v", err)
	}

	ref, err := doBuild(ctx, fromData(d, providerOpts.po.DockerRepo))
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}

	d.Set("image_ref", ref)
	d.SetId(ref)
	return nil
}

func resourceKoBuildRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	providerOpts, err := NewProviderOpts(meta)
	if err != nil {
		return diag.Errorf("configuring provider: %v", err)
	}

	ref, err := doBuild(ctx, fromData(d, providerOpts.po.DockerRepo))
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}

	d.Set("image_ref", ref)
	if ref != d.Id() {
		d.SetId("")
	} else {
		log.Println("image not changed")
	}
	return nil
}

func resourceKoBuildDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// TODO: If we ever want to delete the image from the registry, we can do it here.
	return nil
}
