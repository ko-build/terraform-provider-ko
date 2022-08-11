package provider

import (
	"fmt"
)

const (
	// PlatformsKey is used for common "platforms" resource attribute
	PlatformsKey = "platforms"
	// SBOMKey is used for common "sbom" resource attribute
	SBOMKey      = "sbom"
	BaseImageKey = "base_image"
	TagsKey      = "tags"
	TagOnlyKey   = "tag_only"
	PushKey      = "push"
	FilenamesKey = "filenames"
	RecursiveKey = "recursive"

	SelectorKey = "selector"

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
