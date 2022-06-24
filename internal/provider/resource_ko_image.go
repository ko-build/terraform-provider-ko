package provider

import (
	"context"
	"log"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceImage() *schema.Resource {
	return &schema.Resource{
		// This description is used by the documentation generator and the language server.
		Description: "Sample resource in the Terraform provider scaffolding.",

		CreateContext: resourceScaffoldingCreate,
		ReadContext:   resourceScaffoldingRead,
		DeleteContext: resourceScaffoldingDelete,

		Schema: map[string]*schema.Schema{
			"importpath": {
				Description: "import path blah",
				Type:        schema.TypeString,
				Required:    true,
				ValidateDiagFunc: func(data interface{}, path cty.Path) diag.Diagnostics {
					// TODO: validate stuff here.
					return nil
				},
				ForceNew: true, // Any time this changes, don't try to update in-place, just create it.
			},
			"image_ref": {
				Description: "image at digest",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func resourceScaffoldingCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	ip := d.Get("importpath").(string)
	log.Println("got importpath", ip)

	d.Set("image_ref", "my-repo@sha256:abc") // TODO
	d.SetId("my-repo@sha256:abc")            // TODO
	return nil
}

func resourceScaffoldingRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// TODO: Build the image again, and only unset ID if it changed.
	if changed {
		d.SetId("")
	} else {
		log.Println("image not changed")
	}
	return nil
}

func resourceScaffoldingDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// TODO: If we ever want to delete the image from the registry, we can do it here.
	return nil
}
