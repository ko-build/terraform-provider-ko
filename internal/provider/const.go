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
	// TagOnlyKey used for common "tag_only" resource attribute
	TagOnlyKey = "tag_only"
	// PushKey used for common "push" resource attribute
	PushKey = "push"
	// FilenamesKey used for common "filenames" resource attribute
	FilenamesKey = "filenames"
	// RecursiveKey used for common "recursive" resource attribute
	RecursiveKey = "recursive"
	// SelectorKey used for common "selector" resource attribute
	SelectorKey = "selector"
	// ImageRefKey used for common "image_ref" resource attribute
	ImageRefKey = "image_ref"
	// ManifestsKey used for common "manifests" resource attribute
	ManifestsKey = "manifests"
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
