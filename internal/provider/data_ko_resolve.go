package provider

import (
	"bufio"
	"bytes"
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands"
	"github.com/google/ko/pkg/commands/options"
	"github.com/google/ko/pkg/publish"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataDryResolveConfig() *schema.Resource {
	return &schema.Resource{
		Description: "",

		ReadContext: dataDryResolveConfigRead,

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
			SelectorKey: {
				Description: "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)",
				Optional:    true,
				Type:        schema.TypeString,
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

// dryResolver is an implementation of Resolver that doesn't actually build anything
type dryResolver struct {
	fo *options.FilenameOptions
	so *options.SelectorOptions
}

func NewDryResolver(d *schema.ResourceData, meta interface{}) (*dryResolver, error) {
	r := &dryResolver{
		fo: &options.FilenameOptions{},
		so: &options.SelectorOptions{},
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

func (r *dryResolver) Resolve(ctx context.Context) (*Resolved, error) {
	var resolveBuf bytes.Buffer
	w := &nopWriteCloser{Writer: bufio.NewWriter(&resolveBuf)}

	emptyBuilder, err := build.NewCaching(&emptyBuilder{})
	if err != nil {
		return nil, err
	}

	if err := commands.ResolveFilesToWriter(ctx, emptyBuilder, &emptyPublisher{}, r.fo, r.so, w); err != nil {
		return nil, err
	}

	if err := w.Flush(); err != nil {
		return nil, err
	}

	return NewResolved(resolveBuf.Bytes())
}

func dataDryResolveConfigRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	r, err := NewDryResolver(d, meta)
	if err != nil {
		return diag.Errorf("building dry resolver: %v", err)
	}

	resolved, err := r.Resolve(ctx)
	if err != nil {
		return diag.Errorf("resolving: %v", err)
	}

	d.SetId(resolved.ID())
	d.Set("manifests", resolved.Manifests)
	return nil
}

var _ = build.Interface(&emptyBuilder{})

type emptyBuilder struct{}

// Build implements build.Interface
func (*emptyBuilder) Build(ctx context.Context, ip string) (build.Result, error) {
	return empty.Image, nil
}

// IsSupportedReference implements build.Interface
func (*emptyBuilder) IsSupportedReference(string) error {
	return nil
}

// QualifyImport implements build.Interface
func (*emptyBuilder) QualifyImport(ip string) (string, error) {
	return ip, nil
}

var _ = publish.Interface(&emptyPublisher{})

type emptyPublisher struct{}

// Close implements publish.Interface
func (*emptyPublisher) Close() error {
	return nil
}

// Publish implements publish.Interface
func (*emptyPublisher) Publish(ctx context.Context, result build.Result, ip string) (name.Reference, error) {
	return name.ParseReference(ip)
}
