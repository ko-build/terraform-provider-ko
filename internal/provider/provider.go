package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/ko/pkg/commands/options"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func init() {
	// Set descriptions to support markdown syntax, this will be used in document generation
	// and the language server.
	schema.DescriptionKind = schema.StringMarkdown

	// Customize the content of descriptions when output. For example you can add defaults on
	// to the exported descriptions if present.
	// schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
	// 	desc := s.Description
	// 	if s.Default != nil {
	// 		desc += fmt.Sprintf(" Defaults to `%v`.", s.Default)
	// 	}
	// 	return strings.TrimSpace(desc)
	// }
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"docker_repo": {
					Description: "[DEPRECATED: use `repo`] Container repository to publish images to. Defaults to `KO_DOCKER_REPO` env var",
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("KO_DOCKER_REPO", ""),
					Type:        schema.TypeString,
				},
				"repo": {
					Description: "Container repository to publish images to. Defaults to `KO_DOCKER_REPO` env var",
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("KO_DOCKER_REPO", ""),
					Type:        schema.TypeString,
				},
				"basic_auth": {
					Description: "Basic auth to use to authorize requests",
					Optional:    true,
					Default:     "",
					Type:        schema.TypeString,
				},
			},
			ResourcesMap: map[string]*schema.Resource{
				"ko_image":   resourceImage(),
				"ko_build":   resourceBuild(),
				"ko_resolve": resolveConfig(),
			},
		}

		p.ConfigureContextFunc = configure(version, p)

		return p
	}
}

// configure initializes the global provider with sensible defaults (that mimic what ko does with cli/cobra defaults)
// TODO: review input parameters
func configure(version string, p *schema.Provider) func(context.Context, *schema.ResourceData) (interface{}, diag.Diagnostics) { //nolint: revive
	return func(_ context.Context, s *schema.ResourceData) (interface{}, diag.Diagnostics) {
		koDockerRepo, ok := s.Get("repo").(string)
		if !ok {
			return nil, diag.Errorf("expected repo to be string")
		}
		if koDockerRepo == "" {
			koDockerRepo, ok = s.Get("docker_repo").(string)
			if !ok {
				return nil, diag.Errorf("expected docker_repo to be string")
			}
		}

		var auth *authn.Basic
		if a, ok := s.Get("basic_auth").(string); !ok {
			return nil, diag.Errorf("expected basic_auth to be string")
		} else if a != "" {
			user, pass, ok := strings.Cut(a, ":")
			if !ok {
				return nil, diag.Errorf(`basic_auth did not contain ":"`)
			}
			auth = &authn.Basic{
				Username: user,
				Password: pass,
			}
		}

		return &Opts{
			bo: &options.BuildOptions{},
			po: &options.PublishOptions{
				DockerRepo: koDockerRepo,
			},
			auth: auth,
		}, nil
	}
}

type Opts struct {
	bo   *options.BuildOptions
	po   *options.PublishOptions
	auth *authn.Basic
}

func NewProviderOpts(meta interface{}) (*Opts, error) {
	opts, ok := meta.(*Opts)
	if !ok {
		return nil, fmt.Errorf("parsing provider args: %v", meta)
	}

	// This won't parse the cmd flags, but it will parse any environment vars and set some helpful defaults for us
	if err := opts.bo.LoadConfig(); err != nil {
		return nil, err
	}

	return opts, nil
}
