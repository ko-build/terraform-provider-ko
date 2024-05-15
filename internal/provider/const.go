package provider

import (
	"fmt"
)

const (
	// ImportPathKey is used for common "importpath" resource attribute
	ImportPathKey = "importpath"
	// WorkingDirKey is used for common "working_dir" resource attribute
	WorkingDirKey = "working_dir"
	// PlatformsKey is used for common "platforms" resource attribute
	PlatformsKey = "platforms"
	// SBOMKey is used for common "sbom" resource attribute
	SBOMKey = "sbom"
	// BaseImageKey is used for common "base_image" resource attribute
	BaseImageKey = "base_image"
	// TagsKey is used for common "tags" resource attribute
	TagsKey = "tags"
	// TagOnlyKey is used for common "tag_only" resource attribute
	TagOnlyKey = "tag_only"
	// PushKey is used for common "push" resource attribute
	PushKey = "push"
	// FilenamesKey is used for common "filenames" resource attribute
	FilenamesKey = "filenames"
	// RecursiveKey is used for common "recursive" resource attribute
	RecursiveKey = "recursive"
	// SelectorKey is used for common "selector" resource attribute
	SelectorKey = "selector"
	// ImageRefKey is used for common "image_ref" resource attribute
	ImageRefKey = "image_ref"
	// ManifestsKey is used for common "manifests" resource attribute
	ManifestsKey = "manifests"
	// RepoKey is used for common "repo" resource attribute
	RepoKey = "repo"
	// Ldflags is used for common "ldflags" resource attribute
	LdflagsKey = "ldflags"
)

func StringSlice(in []interface{}) []string {
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
