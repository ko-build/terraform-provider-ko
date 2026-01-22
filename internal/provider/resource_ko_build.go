package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
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
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands/options"
	"github.com/google/ko/pkg/publish"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
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
			"go_binary_path": {
				Description: "Path to the Go binary to use for builds (sets KO_GO_PATH)",
				Optional:    true,
				Type:        schema.TypeString,
				ForceNew:    true, // Any time this changes, don't try to update in-place, just create it.
			},
		},
	}
}

type buildOptions struct {
	ip           string
	workingDir   string
	imageRepo    string // The image's repo, either from the KO_DOCKER_REPO env var, or provider-configured dockerRepo/repo, or image resource's repo.
	platforms    []string
	baseImage    string
	sbom         string
	auth         *authn.Basic
	bare         bool     // If true, use the "bare" namer that doesn't append the importpath.
	ldflags      []string // Extra ldflags to pass to the go build.
	env          []string // Extra environment variables to pass to the go build.
	tags         []string // Which tags to use for the produced image instead of the default 'latest'
	goBinaryPath string   // Path to go binary (sets KO_GO_PATH)
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
	// If goBinaryPath is set, set the KO_GO_PATH environment variable
	if o.goBinaryPath != "" {
		os.Setenv("KO_GO_PATH", o.goBinaryPath)
	}

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
func doBuild(ctx context.Context, opts buildOptions) (build.Result, string, error) {
	tflog.Debug(ctx, "building image", map[string]interface{}{
		"importpath": opts.ip,
		"repo":       opts.imageRepo,
		"platforms":  opts.platforms,
		"base_image": opts.baseImage,
	})

	if opts.imageRepo == "" {
		return nil, "", errors.New("one of KO_DOCKER_REPO env var, or provider `repo`, or image resource `repo` must be set")
	}

	b, err := opts.makeBuilder(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("NewGo: %w", err)
	}
	res, err := b.Build(ctx, opts.ip)
	if err != nil {
		tflog.Error(ctx, "build failed", map[string]interface{}{
			"importpath": opts.ip,
			"error":      err.Error(),
		})
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

	tflog.Debug(ctx, "built image", map[string]interface{}{
		"importpath": opts.ip,
		"digest":     dig.String(),
		"ref":        ref.String(),
	})

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

// isManifestInvalidError reports whether err is a MANIFEST_INVALID registry error.
func isManifestInvalidError(err error) bool {
	var terr *transport.Error
	if errors.As(err, &terr) {
		for _, e := range terr.Errors {
			if e.Code == transport.ManifestInvalidErrorCode {
				return true
			}
		}
	}
	return false
}

func doPublish(ctx context.Context, r build.Result, opts buildOptions) (string, error) {
	tflog.Debug(ctx, "publishing image", map[string]interface{}{
		"importpath": opts.ip,
		"repo":       opts.imageRepo,
		"tags":       opts.tags,
	})

	kc := keychain
	if opts.auth != nil {
		kc = authn.NewMultiKeychain(staticKeychain{opts.imageRepo, opts.auth}, kc)
	}

	po := []publish.Option{
		publish.WithAuthFromKeychain(kc),
		publish.WithNamer(namer(opts)),
		publish.WithUserAgent(userAgent),
		publish.WithTransport(newLoggingTransport(ctx)),
	}

	if len(opts.tags) > 0 {
		po = append(po, publish.WithTags(opts.tags))
	}

	p, err := publish.NewDefault(opts.imageRepo, po...)
	if err != nil {
		return "", fmt.Errorf("NewDefault: %w", err)
	}

	// Retry MANIFEST_INVALID errors, which can occur due to registry eventual
	// consistency. Some registries (notably GCR) return 201 for blob uploads
	// before the blob is visible to all endpoints. If the manifest push arrives
	// before blobs propagate, the registry returns MANIFEST_INVALID.
	//
	// We retry a small number of times with a short delay. This handles the
	// transient propagation case (typically succeeds within 1-2s) without
	// spinning on genuinely invalid manifests that will never succeed.
	const maxAttempts = 3
	const retryDelay = time.Second

	var ref name.Reference
	for attempt := 1; ; attempt++ {
		ref, err = p.Publish(ctx, r, opts.ip)
		if err == nil {
			break
		}

		if !isManifestInvalidError(err) || attempt >= maxAttempts {
			tflog.Error(ctx, "publish failed", map[string]interface{}{
				"importpath": opts.ip,
				"repo":       opts.imageRepo,
				"error":      err.Error(),
			})
			return "", fmt.Errorf("publish: %w", err)
		}

		tflog.Debug(ctx, "retrying after MANIFEST_INVALID (registry propagation delay)",
			map[string]interface{}{
				"attempt":      attempt,
				"max_attempts": maxAttempts,
				"delay":        retryDelay.String(),
				"importpath":   opts.ip,
			})
		time.Sleep(retryDelay)
	}

	tflog.Debug(ctx, "published image", map[string]interface{}{
		"ref": ref.String(),
	})
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
		ip:           d.Get("importpath").(string),
		workingDir:   d.Get("working_dir").(string),
		imageRepo:    repo,
		platforms:    defaultPlatform(toStringSlice(d.Get("platforms").([]interface{}))),
		baseImage:    getString(d, "base_image", po.bo.BaseImage),
		sbom:         d.Get("sbom").(string),
		auth:         po.auth,
		bare:         bare,
		ldflags:      toStringSlice(d.Get("ldflags").([]interface{})),
		env:          toStringSlice(d.Get("env").([]interface{})),
		tags:         toStringSlice(d.Get("tags").([]interface{})),
		goBinaryPath: getString(d, "go_binary_path", ""),
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

	res, _, err := doBuild(ctx, fromData(d, po))
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
	_, ref, err := doBuild(ctx, fromData(d, po))
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

// loggingTransport wraps an http.RoundTripper to log registry requests and responses.
// Logs at TRACE level for all requests, and logs full request/response bodies on errors
// to help debug issues like MANIFEST_INVALID.
type loggingTransport struct {
	inner http.RoundTripper
	ctx   context.Context
}

func newLoggingTransport(ctx context.Context) http.RoundTripper {
	return &loggingTransport{
		inner: http.DefaultTransport,
		ctx:   ctx,
	}
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := t.ctx

	// Log request at trace level
	tflog.Trace(ctx, "registry request",
		map[string]interface{}{
			"method":       req.Method,
			"url":          req.URL.String(),
			"content_type": req.Header.Get("Content-Type"),
		})

	// Capture request body for potential error logging
	var reqBody []byte
	if req.Body != nil && shouldLogBody(req.Header.Get("Content-Type")) {
		var err error
		reqBody, err = io.ReadAll(req.Body)
		if err != nil {
			tflog.Warn(ctx, "failed to read request body for logging", map[string]interface{}{"error": err.Error()})
		} else {
			req.Body = io.NopCloser(bytes.NewReader(reqBody))
		}
	}

	start := time.Now()
	resp, err := t.inner.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		tflog.Error(ctx, "registry request failed",
			map[string]interface{}{
				"method":   req.Method,
				"url":      req.URL.String(),
				"error":    err.Error(),
				"duration": duration.String(),
			})
		return resp, err
	}

	// Log response at trace level
	tflog.Trace(ctx, "registry response",
		map[string]interface{}{
			"method":       req.Method,
			"url":          req.URL.String(),
			"status":       resp.StatusCode,
			"duration":     duration.String(),
			"content_type": resp.Header.Get("Content-Type"),
		})

	// On error responses, log full details to help debug MANIFEST_INVALID and similar errors
	if resp.StatusCode >= 400 {
		t.logErrorDetails(ctx, req, reqBody, resp)
	}

	return resp, nil
}

// logErrorDetails logs detailed request/response information for registry operations.
// Full request/response bodies are only included at TRACE level.
//
// Not all 4xx responses are errors:
//   - 401 on /v2/ is the standard Docker registry auth challenge (client then fetches token)
//   - 404 on HEAD requests are existence checks (blob/manifest not found = needs upload)
//
// These are logged at DEBUG level. Actual errors (400, 403, 5xx) are logged at ERROR.
func (t *loggingTransport) logErrorDetails(ctx context.Context, req *http.Request, reqBody []byte, resp *http.Response) {
	attrs := map[string]interface{}{
		"method": req.Method,
		"url":    req.URL.String(),
		"status": resp.StatusCode,
	}

	traceLevel := logging.LogLevel() == "TRACE"

	// Include request body (typically the manifest) only at TRACE level
	if traceLevel && len(reqBody) > 0 {
		attrs["request_body"] = string(reqBody)
	}

	// Capture response body (contains error details)
	if resp.Body != nil {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			attrs["response_body_error"] = err.Error()
		} else {
			if traceLevel {
				attrs["response_body"] = string(respBody)
			}
			// Restore body for further processing
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
		}
	}

	// Include request headers only at TRACE level
	if traceLevel {
		if reqDump, err := httputil.DumpRequestOut(req, false); err == nil {
			attrs["request_headers"] = string(reqDump)
		}
	}

	// Determine log level based on response type
	if isExpectedProtocolResponse(req, resp) {
		tflog.Debug(ctx, "registry response", attrs)
	} else {
		tflog.Error(ctx, "registry error response", attrs)
	}
}

// isExpectedProtocolResponse returns true for HTTP responses that are part of
// normal registry protocol operation, not actual errors.
func isExpectedProtocolResponse(req *http.Request, resp *http.Response) bool {
	// 401 on /v2/ is the auth challenge - client will fetch token and retry
	if resp.StatusCode == http.StatusUnauthorized && strings.HasSuffix(req.URL.Path, "/v2/") {
		return true
	}

	// 404 on HEAD requests are existence checks for blobs/manifests
	// The client checks if content exists before uploading; 404 means "upload needed"
	if resp.StatusCode == http.StatusNotFound && req.Method == http.MethodHead {
		return true
	}

	return false
}

// shouldLogBody returns true if the content type indicates a body worth logging.
func shouldLogBody(contentType string) bool {
	// Log manifest and JSON bodies which are relevant for debugging MANIFEST_INVALID
	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "application/vnd.oci") ||
		strings.Contains(contentType, "application/vnd.docker")
}
