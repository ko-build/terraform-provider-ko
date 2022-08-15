package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands"
	"github.com/google/ko/pkg/commands/options"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"gopkg.in/yaml.v3"
)

func resolveConfig() *schema.Resource {
	return &schema.Resource{
		Description: "",

		CreateContext: resourceKoResolveCreate,
		ReadContext:   resourceKoResolveRead,
		DeleteContext: resourceKoBuildDelete,

		Schema: map[string]*schema.Schema{
			FilenamesKey: {
				Description: "Filenames, directorys, or URLs to files to use to create the resource",
				Required:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true,
			},
			RecursiveKey: {
				Description: "Process the directory used in -f, --filename recursively. Useful when you want to manage related manifests organized within the same directory.",
				Optional:    true,
				Type:        schema.TypeBool,
				ForceNew:    true,
			},
			PushKey: {
				Description: "Push images to KO_DOCKER_REPO",
				Default:     true,
				Optional:    true,
				Type:        schema.TypeBool,
				ForceNew:    true,
			},
			SelectorKey: {
				Description: "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)",
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true,
			},
			PlatformsKey: {
				Description: "Which platform to use when pulling a multi-platform base. Format: all | <os>[/<arch>[/<variant>]][,platform]*",
				Optional:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true,
			},
			SBOMKey: {
				Description: "The SBOM media type to use (none will disable SBOM synthesis and upload, also supports: spdx, cyclonedx, go.version-m).",
				Default:     "spdx",
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true,
			},
			BaseImageKey: {
				Description: "",
				Default:     defaultBaseImage,
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true,
			},
			TagsKey: {
				Description: "Which tags to use for the produced image instead of the default 'latest' tag ",
				Optional:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				ForceNew:    true,
			},

			// Computed fields
			ManifestsKey: {
				Description: "A list of resolved manifests in a 'yamldecode'able format. Note that whitespaces and nil docs will be stripped from these results.",
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Computed:    true,
			},
		},
	}
}

type Resolver interface {
	Resolve(ctx context.Context) (*Resolved, error)
}

type Resolved struct {
	Manifests []string
}

// ID returns the hash of the parsed manifests, not the raw manifests.  This
// ensures that "noop" changes to the resolved results (such as nil yaml docs),
// do not result in a new ID.
func (r *Resolved) ID() string {
	hash := sha256.Sum256([]byte(strings.Join(r.Manifests, "\n---\n")))
	return fmt.Sprintf("%x", hash)
}

type resolver struct {
	bo *options.BuildOptions
	po *options.PublishOptions
	fo *options.FilenameOptions
	so *options.SelectorOptions
}

func NewResolver(d *schema.ResourceData, meta interface{}) (*resolver, error) {
	opts, err := NewProviderOpts(meta)
	if err != nil {
		return nil, err
	}

	r := &resolver{
		bo: opts.bo,
		po: opts.po,
		fo: &options.FilenameOptions{},
		so: &options.SelectorOptions{},
	}

	if p, ok := d.Get(BaseImageKey).(string); ok {
		r.bo.BaseImage = p
	}

	if p, ok := d.Get(TagsKey).([]interface{}); ok {
		if len(p) == 0 {
			r.po.Tags = []string{"latest"}
		} else {
			r.po.Tags = StringSlice(p)
		}
	}

	if p, ok := d.Get(PushKey).(bool); ok {
		r.po.Push = p
	}

	if p, ok := d.Get(FilenamesKey).([]interface{}); ok {
		r.fo.Filenames = StringSlice(p)
	}

	if p, ok := d.Get(RecursiveKey).(bool); ok {
		r.fo.Recursive = p
	}

	if p, ok := d.Get(SelectorKey).(string); ok {
		r.so.Selector = p
	}

	return r, nil
}

func (r *resolver) Resolve(ctx context.Context) (*Resolved, error) {
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

	return NewResolved(resolveBuf.Bytes())
}

func NewResolved(data []byte) (*Resolved, error) {
	var manifests []string

	// NOTE: Don't use a strings.Split to ensure we filter out any null docs
	decoder := yaml.NewDecoder(bytes.NewBuffer(data))
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

	return &Resolved{
		Manifests: manifests,
	}, nil
}

func resourceKoResolveCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	r, err := NewResolver(d, meta)
	if err != nil {
		return diag.Errorf("building resolver: %v", err)
	}

	resolved, err := r.Resolve(ctx)
	if err != nil {
		return diag.Errorf("resolving: %v", err)
	}

	d.SetId(resolved.ID())
	d.Set("manifests", resolved.Manifests)
	return nil
}

func resourceKoResolveRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	r, err := NewResolver(d, meta)
	if err != nil {
		return diag.Errorf("building resolver: %v", err)
	}

	// NOTE: Fake the publisher to prevent needing to republish the images on reads
	r.po.Tags = []string{}

	resolved, err := r.Resolve(ctx)
	if err != nil {
		return diag.Errorf("resolving: %v", err)
	}

	d.Set("manifests", resolved.Manifests)
	if resolved.ID() != d.Id() {
		d.SetId("")
	}
	return nil
}

func resourceKoResolveDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

type nopWriteCloser struct {
	*bufio.Writer
}

func (w *nopWriteCloser) Close() error {
	return nil
}
