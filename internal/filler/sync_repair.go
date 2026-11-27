package filler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// repairOpaqueDisplayName removes implementation identifiers from the user-facing title even when
// the media is already at its canonical path. Both the former 40-character digest and the current
// 64-character content id are recognised; arbitrary hex-like operator names are left alone.
func (s *Syncer) repairOpaqueDisplayName(rc cliplibrary.RawClip, existing clipstore.Clip, found bool) (cliplibrary.RawClip, bool, error) {
	if !cliplibrary.IsHashDisplayName(rc.Name) {
		return rc, false, nil
	}
	grounded := clips.Clip{Role: rc.Role}
	if found {
		grounded = existing.Clip
	}
	rc.Name, rc.NameSource = groundedRepairName(grounded), clips.NameFromGrounded
	if rc.SidecarInvalid {
		return rc, true, nil
	}
	full := filepath.Join(s.dir, filepath.FromSlash(rc.Path))
	tags, _ := asset.ReadSidecarTags(full)
	if tags.OriginalName == "" || cliplibrary.IsHashDisplayName(tags.OriginalName) {
		tags.OriginalName = rc.Name
		cliplibrary.CarryNameProvenance(&tags, rc.Name, rc.NameSource)
		if found {
			tags.Kind = string(existing.Role)
			tags.Era = existing.Era
			tags.Audience = string(existing.Audience)
			tags.Brand = existing.Brand
			tags.Transcript = existing.Transcript
		}
		if err := asset.WriteSidecarTags(full, tags, false); err != nil {
			return rc, false, err
		}
	}
	return rc, true, nil
}

// repairLegacyContentPath recognises one old Loomarr state: a full hash-shaped filename whose
// stem does not match the full hash-shaped identity computed from the bytes. Arbitrary operator
// filenames are deliberately untouched. The canonical media and sidecar are published without
// replacement before the stale links are removed, so interruption can only leave a duplicate.
func (s *Syncer) repairLegacyContentPath(rc cliplibrary.RawClip, existing clipstore.Clip, found bool) (cliplibrary.RawClip, bool, error) {
	stem := strings.TrimSuffix(filepath.Base(filepath.FromSlash(rc.Path)), filepath.Ext(rc.Path))
	if !split.IsContentHash(rc.ID) || !cliplibrary.IsOpaqueHash(stem) || stem == rc.ID {
		return rc, false, nil
	}
	ext := filepath.Ext(rc.Path)
	canonical := filepath.ToSlash(cliplibrary.ClipRelPath(rc.ID, ext))
	if rc.Path == canonical {
		return rc, false, nil
	}
	if rc.SidecarInvalid {
		return rc, false, nil
	}
	oldFull := filepath.Join(s.dir, filepath.FromSlash(rc.Path))
	newFull := filepath.Join(s.dir, filepath.FromSlash(canonical))

	// If the durable row still has a useful title, put it beside the bytes before moving. This is
	// the last automatic chance to prevent the next scan from replacing it with the stale hash.
	repairName, repairSource := "", clips.NameFromUnknown
	if found {
		repairName, repairSource = existing.Name, existing.NameSource
		if cliplibrary.IsHashDisplayName(repairName) {
			repairName, repairSource = groundedRepairName(existing.Clip), clips.NameFromGrounded
		}
	}
	if repairName != "" {
		tags, _ := asset.ReadSidecarTags(oldFull)
		if tags.OriginalName == "" || cliplibrary.IsHashDisplayName(tags.OriginalName) {
			tags.OriginalName = repairName
			cliplibrary.CarryNameProvenance(&tags, repairName, repairSource)
			tags.Kind = string(existing.Role)
			tags.Era = existing.Era
			tags.Audience = string(existing.Audience)
			tags.Brand = existing.Brand
			tags.Transcript = existing.Transcript
			if err := asset.WriteSidecarTags(oldFull, tags, false); err != nil {
				return rc, false, err
			}
			rc.Name, rc.NameSource = repairName, repairSource
		}
	}
	if err := os.MkdirAll(filepath.Dir(newFull), 0o755); err != nil {
		return rc, false, err
	}
	targetExists := false
	if _, err := os.Stat(newFull); err == nil {
		id, hashErr := asset.ClipID(newFull)
		if hashErr != nil || id != rc.ID {
			return rc, false, fmt.Errorf("canonical target exists with different content")
		}
		targetExists = true
	} else if os.IsNotExist(err) {
	} else {
		return rc, false, err
	}

	oldSidecar, newSidecar := tools.SidecarPathFor(oldFull), tools.SidecarPathFor(newFull)
	if _, err := os.Stat(oldSidecar); err == nil {
		if _, targetErr := os.Stat(newSidecar); os.IsNotExist(targetErr) {
			if err := os.Link(oldSidecar, newSidecar); err != nil {
				return rc, false, err
			}
		} else if targetErr == nil {
			// A prior interrupted repair may already have the canonical media/sidecar. Merge our
			// namespaced facts rather than choosing one file and silently losing the other.
			if oldTags, ok := asset.ReadSidecarTags(oldFull); ok {
				if err := asset.WriteSidecarTags(newFull, oldTags, false); err != nil {
					return rc, false, err
				}
			}
		} else if targetErr != nil {
			return rc, false, targetErr
		}
	} else if !os.IsNotExist(err) {
		return rc, false, err
	}
	// Sidecar first: without media the canonical sidecar is invisible to the scan. Publishing the
	// media second prevents even a concurrent scan from observing a hash title between the two.
	if !targetExists {
		if err := os.Link(oldFull, newFull); err != nil {
			return rc, false, err
		}
	}

	if err := os.Remove(oldFull); err != nil && !os.IsNotExist(err) {
		return rc, false, err
	}
	_ = os.Remove(oldSidecar)
	rc.Path = canonical
	return rc, true, nil
}

// groundedRepairName replaces an opaque title from facts a person or grounded classifier supplied.
// When there are none, it uses a neutral kind label rather than exposing an implementation id as
// if that were a title.
func groundedRepairName(c clips.Clip) string {
	name, _ := cliplibrary.GroundedName(c)
	return name
}

// repairPlaceholderDisplayName names a clip that has no source title from the evidence gathered
// since it was catalogued (#1452). Sync first names such a clip before transcription or vision has
// run, so "Untitled commercial" would otherwise stick after its transcript has identified it.
//
// ⚠ Only a name Loomarr derived is replaced: one this step recorded (nameSource), the neutral
// "Untitled <kind>", or the pre-#1452 brand/category/era derivation. A source title or a name a
// person gave is left alone. The derived name goes to `displayName`, never `originalName`, which
// is the text era grounding reads.
func (s *Syncer) repairPlaceholderDisplayName(rc cliplibrary.RawClip, existing clipstore.Clip, found bool) (cliplibrary.RawClip, bool, error) {
	if !found || rc.SidecarInvalid {
		return rc, false, nil
	}
	full := filepath.Join(s.dir, filepath.FromSlash(rc.Path))
	if asset.SidecarTitle(full) != "" {
		return rc, false, nil
	}
	tags, state := asset.ReadSidecarTagsState(full)
	if state == asset.SidecarInvalid {
		return rc, false, nil
	}
	// A segment's chapter title or operator edit is not Loomarr's to improve, and neither is the
	// filename fallback it records explicitly (#1988).
	if recorded := clips.NameSource(tags.NameProvenance); recorded.Known() && recorded != clips.NameFromGrounded {
		return rc, false, nil
	}
	legacy := existing.Clip
	legacy.Transcript, legacy.VisibleText = "", ""
	legacyName, _ := cliplibrary.GroundedName(legacy)
	derived := tags.NameSource != "" || strings.HasPrefix(rc.Name, "Untitled ") || rc.Name == legacyName
	if !derived {
		return rc, false, nil
	}
	name, source := cliplibrary.GroundedName(existing.Clip)
	if name == rc.Name && tags.NameSource == string(source) {
		return rc, false, nil
	}
	tags.DisplayName, tags.NameSource = name, string(source)
	if err := asset.WriteSidecarTags(full, tags, false); err != nil {
		return rc, false, err
	}
	rc.Name, rc.NameSource = name, clips.NameFromGrounded
	return rc, true, nil
}
