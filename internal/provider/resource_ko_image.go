package provider

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"sync"

	"github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"github.com/chrismellard/docker-credential-acr-env/pkg/credhelper"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/github"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/google"
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
	defaultBaseImage = "cgr.dev/chainguard/static"
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
				Description: "platforms to build",
				Default:     "linux/amd64",
				Optional:    true,
				Type:        schema.TypeString, // TODO: type list of strings?
				ForceNew:    true,              // Any time this changes, don't try to update in-place, just create it.
			},
			BaseImageKey: {
				Description: "base image to use",
				Default:     defaultBaseImage,
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
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
	ip         string
	workingDir string
	imageRepo  string // The image's repo, either from the KO_DOCKER_REPO env var, or provider-configured dockerRepo/repo, or image resource's repo.
	platforms  []string
	baseImage  string
	sbom       string
	auth       *authn.Basic
	bare       bool // If true, use the "bare" namer that doesn't append the importpath.
}

var (
	amazonKeychain authn.Keychain = authn.NewKeychainFromHelper(ecr.NewECRHelper(ecr.WithLogger(ioutil.Discard)))
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
		build.WithPlatforms(o.platforms...),
		build.WithBaseImages(func(ctx context.Context, s string) (name.Reference, build.Result, error) {
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
			desc, err := remote.Get(ref, remote.WithAuthFromKeychain(kc))
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
		bo = append(bo, build.WithDisabledSBOM())
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
	if opts.imageRepo == "" {
		return "", errors.New("one of KO_DOCKER_REPO env var, or provider `docker_repo` or `repo`, or image resource `repo` must be set")
	}

	b, err := opts.makeBuilder(ctx)
	if err != nil {
		return "", fmt.Errorf("NewGo: %v", err)
	}
	r, err := b.Build(ctx, opts.ip)
	if err != nil {
		return "", fmt.Errorf("build: %v", err)
	}

	kc := keychain
	if opts.auth != nil {
		kc = authn.NewMultiKeychain(staticKeychain{opts.imageRepo, opts.auth}, kc)
	}
	po := []publish.Option{publish.WithAuthFromKeychain(kc)}
	if opts.bare {
		po = append(po, publish.WithNamer(options.MakeNamer(&options.PublishOptions{
			DockerRepo: opts.imageRepo,
			Bare:       true,
		})))
	}

	p, err := publish.NewDefault(opts.imageRepo, po...)
	if err != nil {
		return "", fmt.Errorf("NewDefault: %v", err)
	}
	ref, err := p.Publish(ctx, r, opts.ip)
	if err != nil {
		return "", fmt.Errorf("publish: %v", err)
	}
	return ref.String(), nil
}

func fromData(d *schema.ResourceData, po *providerOpts) buildOptions {
	// Use the repo configured in the ko_image resource, if set.
	// Otherwise, fallback to the provider-configured repo.
	// If the ko_image resource configured the repo, use bare image naming.
	repo := po.po.DockerRepo
	bare := false
	if r := d.Get(RepoKey).(string); r != "" {
		repo = r
		bare = true
	}

	return buildOptions{
		ip:         d.Get("importpath").(string),
		workingDir: d.Get("working_dir").(string),
		imageRepo:  repo,
		platforms:  toStringSlice(d.Get("platforms").([]interface{})),
		baseImage:  d.Get("base_image").(string),
		sbom:       d.Get("sbom").(string),
		auth:       po.auth,
		bare:       bare,
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
	po, err := NewProviderOpts(meta)
	if err != nil {
		return diag.Errorf("configuring provider: %v", err)
	}

	ref, err := doBuild(ctx, fromData(d, po))
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}

	d.Set("image_ref", ref)
	d.SetId(ref)
	return nil
}

func resourceKoBuildRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	po, err := NewProviderOpts(meta)
	if err != nil {
		return diag.Errorf("configuring provider: %v", err)
	}

	ref, err := doBuild(ctx, fromData(d, po))
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
