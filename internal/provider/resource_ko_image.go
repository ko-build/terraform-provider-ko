package provider

import (
	"context"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceImage() *schema.Resource {
	return &schema.Resource{
		Description:        "Deprecated: use ko_build",
		DeprecationMessage: "Deprecated: use ko_build",

		CreateContext: resourceKoImageCreate,
		ReadContext:   resourceKoImageRead,
		DeleteContext: resourceKoImageDelete,

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

func resourceKoImageCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.Set("image_ref", "")
	d.SetId("id")
	return nil
}

func resourceKoImageRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func resourceKoImageDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}
