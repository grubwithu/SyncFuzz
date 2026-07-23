package profiling

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type EvidenceLinkRelation string

const (
	EvidenceLinkExactDeviceInode   EvidenceLinkRelation = "exact-device-inode"
	EvidenceLinkExactSocketID      EvidenceLinkRelation = "exact-socket-id"
	EvidenceLinkExactCanonicalPath EvidenceLinkRelation = "exact-canonical-path"
	EvidenceLinkExactPath          EvidenceLinkRelation = "exact-path"
)

// EvidenceLink binds one kernel-observed effect to one independently observed
// persistent resource in the same checkpoint interval. The first linker is
// intentionally conservative: it only accepts an exact (device,inode),
// socket ID, canonical-path, or path identity; it never uses a basename or
// process-name heuristic. Device and inode must both be present because an
// inode alone is not globally unique.
type EvidenceLink struct {
	LinkID     string               `json:"link_id"`
	EffectID   string               `json:"effect_id"`
	ResourceID string               `json:"resource_id"`
	Relation   EvidenceLinkRelation `json:"relation"`
}

// CanonicalizeWorkspaceEventPaths adds a container-visible canonical path to
// relative resource paths. It is valid for commands whose working directory is
// the supplied workspace root; other paths remain canonicalized lexically but
// will not match workspace probe resources outside that root.
func CanonicalizeWorkspaceEventPaths(events []RawEvent, workspaceRoot string) []RawEvent {
	result := append([]RawEvent{}, events...)
	root := strings.TrimSpace(workspaceRoot)
	if root == "" {
		return result
	}
	root = filepath.Clean(root)
	for index := range result {
		path := strings.TrimSpace(result[index].Resource.Path)
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) {
			result[index].Resource.CanonicalPath = filepath.Clean(path)
			continue
		}
		result[index].Resource.CanonicalPath = filepath.Join(root, path)
	}
	return result
}

func CanonicalWorkspaceResourcePath(workspaceRoot string, relativePath string) string {
	if strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(relativePath) == "" {
		return ""
	}
	if filepath.IsAbs(relativePath) {
		return filepath.Clean(relativePath)
	}
	return filepath.Join(filepath.Clean(workspaceRoot), relativePath)
}

func linkEffectsToDelta(effects []NormalizedEffect, delta StateDelta) []EvidenceLink {
	resources := append([]PersistentResource{}, delta.Added...)
	resources = append(resources, delta.Removed...)
	links := make([]EvidenceLink, 0)
	seen := make(map[string]struct{})
	for _, effect := range effects {
		for _, resource := range resources {
			relation, matched := matchEffectResource(effect, resource.Resource)
			if !matched {
				continue
			}
			link := EvidenceLink{
				LinkID:     fmt.Sprintf("%s=>%s", effect.EffectID, resource.Resource.ResourceID),
				EffectID:   effect.EffectID,
				ResourceID: resource.Resource.ResourceID,
				Relation:   relation,
			}
			if _, ok := seen[link.LinkID]; ok {
				continue
			}
			seen[link.LinkID] = struct{}{}
			links = append(links, link)
		}
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].EffectID != links[j].EffectID {
			return links[i].EffectID < links[j].EffectID
		}
		return links[i].ResourceID < links[j].ResourceID
	})
	return links
}

func matchEffectResource(effect NormalizedEffect, resource ResourceRef) (EvidenceLinkRelation, bool) {
	if effect.Resource.SocketID != "" && effect.Resource.SocketID == resource.SocketID &&
		effect.Family == StateFamilyIPC && resource.Family == StateFamilyIPC {
		return EvidenceLinkExactSocketID, true
	}
	if effect.Family == resource.Family && effect.Resource.Device != 0 && effect.Resource.Inode != 0 &&
		resource.Device != 0 && resource.Inode != 0 &&
		effect.Resource.Device == resource.Device && effect.Resource.Inode == resource.Inode {
		return EvidenceLinkExactDeviceInode, true
	}
	effectCanonical := strings.TrimSpace(effect.Resource.CanonicalPath)
	resourceCanonical := strings.TrimSpace(resource.CanonicalPath)
	if effectCanonical != "" && resourceCanonical != "" && filepath.Clean(effectCanonical) == filepath.Clean(resourceCanonical) {
		return EvidenceLinkExactCanonicalPath, true
	}
	effectPath := strings.TrimSpace(effect.Resource.Path)
	resourcePath := strings.TrimSpace(resource.Path)
	if effectPath != "" && resourcePath != "" && filepath.Clean(effectPath) == filepath.Clean(resourcePath) {
		return EvidenceLinkExactPath, true
	}
	return "", false
}
