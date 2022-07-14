package provider

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/publish"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	defaultBaseImage = "gcr.io/distroless/static:nonroot"
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
				Optional:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"base_image": {
				Description: "base image to use",
				Default:     defaultBaseImage,
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
			"sbom": {
				Description: "sbom type to generate",
				Default:     "spdx",
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
				ValidateDiagFunc: func(data interface{}, path cty.Path) diag.Diagnostics {
					v := data.(string)
					if _, found := validTypes[v]; !found {
						return diag.Errorf("Invalid sbom type: %q", v)
					}
					return nil
				},
			},
			"image_ref": {
				Description: "built image reference by digest",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

type buildOptions struct {
	ip         string
	workingDir string
	dockerRepo string
	platforms  []string
	baseImage  string
	sbom       string
}

var baseImages sync.Map // Cache of base image lookups.

func doBuild(ctx context.Context, opts buildOptions) (string, error) {
	bo := []build.Option{
		build.WithPlatforms(opts.platforms...),
		build.WithBaseImages(func(ctx context.Context, _ string) (name.Reference, build.Result, error) {
			ref, err := name.ParseReference(opts.baseImage)
			if err != nil {
				return nil, nil, err
			}

			if cached, found := baseImages.Load(opts.baseImage); found {
				return ref, cached.(build.Result), nil
			}

			desc, err := remote.Get(ref,
				remote.WithAuthFromKeychain(authn.DefaultKeychain))
			if err != nil {
				return nil, nil, err
			}
			if desc.MediaType.IsImage() {
				img, err := desc.Image()
				baseImages.Store(opts.baseImage, img)
				return ref, img, err
			}
			if desc.MediaType.IsIndex() {
				idx, err := desc.ImageIndex()
				baseImages.Store(opts.baseImage, idx)
				return ref, idx, err
			}
			return nil, nil, fmt.Errorf("Unexpected base image media type: %s", desc.MediaType)
		}),
	}
	switch opts.sbom {
	case "spdx":
		bo = append(bo, build.WithSPDX(version))
	case "cyclonedx":
		bo = append(bo, build.WithCycloneDX())
	case "go.version-m":
		bo = append(bo, build.WithGoVersionSBOM())
	case "none":
		// don't do anything.
	default:
		return "", fmt.Errorf("Unknown sbom type: %q", opts.sbom)
	}

	b, err := build.NewGo(ctx, opts.workingDir, bo...)
	if err != nil {
		return "", fmt.Errorf("NewGo: %v", err)
	}
	r, err := b.Build(ctx, opts.ip)
	if err != nil {
		return "", fmt.Errorf("Build: %v", err)
	}

	p, err := publish.NewDefault(opts.dockerRepo,
		publish.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return "", fmt.Errorf("NewDefault: %v", err)
	}
	ref, err := p.Publish(ctx, r, opts.ip)
	if err != nil {
		return "", fmt.Errorf("Publish: %v", err)
	}
	return ref.String(), nil
}

func fromData(d *schema.ResourceData, repo string) buildOptions {
	return buildOptions{
		ip:         d.Get("importpath").(string),
		workingDir: d.Get("working_dir").(string),
		dockerRepo: repo,
		platforms:  toStringSlice(d.Get("platforms").([]interface{})),
		baseImage:  d.Get("base_image").(string),
		sbom:       d.Get("sbom").(string),
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
	ref, err := doBuild(ctx, fromData(d, meta.(string)))
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}

	d.Set("image_ref", ref)
	d.SetId(ref)
	return nil
}

func resourceKoBuildRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	ref, err := doBuild(ctx, fromData(d, meta.(string)))
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
