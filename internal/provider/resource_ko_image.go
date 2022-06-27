package provider

import (
	"context"
	"fmt"
	"log"

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
	baseImage  = "gcr.io/distroless/static:nonroot"
	targetRepo = "gcr.io/jason-chainguard"
)

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
			"platforms": {
				Description: "platforms to build",
				Default:     "linux/amd64",
				Optional:    true,
				Type:        schema.TypeString, // TODO: type list of strings?
				ForceNew:    true,              // Any time this changes, don't try to update in-place, just create it.
			},
			"image_ref": {
				Description: "built image reference by digest",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func doBuild(ctx context.Context, ip, platforms, repo string) (string, error) {
	b, err := build.NewGo(ctx, ".",
		build.WithPlatforms(platforms),
		build.WithBaseImages(func(ctx context.Context, _ string) (name.Reference, build.Result, error) {
			ref := name.MustParseReference(baseImage)
			base, err := remote.Index(ref, remote.WithContext(ctx))
			return ref, base, err
		}))
	if err != nil {
		return "", fmt.Errorf("NewGo: %v", err)
	}
	r, err := b.Build(ctx, ip)
	if err != nil {
		return "", fmt.Errorf("Build: %v", err)
	}

	p, err := publish.NewDefault(repo,
		publish.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return "", fmt.Errorf("NewDefault: %v", err)
	}
	ref, err := p.Publish(ctx, r, ip)
	if err != nil {
		return "", fmt.Errorf("Publish: %v", err)
	}
	return ref.String(), nil
}

func resourceKoBuildCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	repo, ok := meta.(string)
	if !ok {
		return diag.Errorf("meta to be a string")
	}

	ref, err := doBuild(ctx,
		d.Get("importpath").(string),
		d.Get("platforms").(string),
		repo)
	if err != nil {
		return diag.Errorf("doBuild: %v", err)
	}

	d.Set("image_ref", ref)
	d.SetId(ref)
	return nil
}

func resourceKoBuildRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// Build the image again, and only unset ID if it changed.
	repo, ok := meta.(string)
	if !ok {
		return diag.Errorf("meta to be a string")
	}

	ref, err := doBuild(ctx,
		d.Get("importpath").(string),
		d.Get("platforms").(string),
		repo)
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
