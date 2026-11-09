package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
)

// ArtifactRecoveryStore is the persistence slice needed to resume staged publication.
type ArtifactRecoveryStore interface {
	ArtifactWriter
	ListRecoverableAcquisitionArtifactsAfter(context.Context, acquisition.ArtifactCursor, int) ([]acquisition.Artifact, error)
}

type RecoveryResult struct {
	Published int
	Repair    int
	Pending   int
}

// RecoverAcquisitionArtifacts resumes the bounded publication protocol after restart. Repair rows
// are retained for explicit intervention; consumed rows are not returned by the store.
func RecoverAcquisitionArtifacts(
	ctx context.Context,
	watchDir string,
	clipDir string,
	store ArtifactRecoveryStore,
	now func() time.Time,
) (RecoveryResult, error) {
	var result RecoveryResult
	if store == nil || watchDir == "" {
		return result, nil
	}
	if now == nil {
		now = time.Now
	}
	var cursor acquisition.ArtifactCursor
	processed := make(map[string]struct{})
	for {
		artifacts, err := store.ListRecoverableAcquisitionArtifactsAfter(ctx, cursor, 500)
		if err != nil {
			return result, err
		}
		if len(artifacts) == 0 {
			return result, nil
		}
		for _, artifact := range artifacts {
			cursor = acquisition.ArtifactCursor{UpdatedAt: artifact.UpdatedAt, ID: artifact.ID}
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			default:
			}
			if _, seen := processed[artifact.ID]; seen {
				continue
			}
			processed[artifact.ID] = struct{}{}
			if artifact.Provider == string(YouTube) && artifact.State == acquisition.ArtifactStaged && artifact.ProviderArchiveEntry == "" {
				artifact = repairArtifact(artifact, "staged yt-dlp artifact has no durable provider archive entry", now())
				if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
					return result, err
				}
				result.Repair++
				continue
			}
			if artifact.ProviderArchiveEntry != "" && !artifact.ProviderArchiveCommitted {
				if err := commitProviderArchive(watchDir, []string{artifact.ProviderArchiveEntry}); err != nil {
					return result, fmt.Errorf("replay provider archive for acquisition artifact %s: %w", artifact.ID, err)
				}
				artifact.ProviderArchiveCommitted = true
				artifact.UpdatedAt = now().UTC()
				if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
					return result, err
				}
			}
			if artifact.State == acquisition.ArtifactRepair && isSidecarExistsRepair(artifact.RepairReason) {
				// Publication used to refuse its own earlier sidecar. Re-drive the row through the
				// staged flow; a real conflict comes back as a settled repair that no longer has
				// this shape, so it is not retried again.
				artifact.State = acquisition.ArtifactStaged
				artifact.RepairReason = ""
			}
			if artifact.State == acquisition.ArtifactRepair {
				if artifact.RepairReason == acquisition.ArtifactMediaMissing && !manifestedMediaExists(watchDir, clipDir, artifact) {
					artifact = repairArtifact(artifact, acquisition.ArtifactMediaGone, now())
					if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
						return result, err
					}
				}
				result.Repair++
				continue
			}
			if artifact.State == acquisition.ArtifactPublished {
				mediaErr := verifyManifestFile(filepath.Join(watchDir, artifact.MediaPath), artifact)
				sidecarErr := verifyPortableProvenance(filepath.Join(watchDir, artifact.SidecarPath), artifact)
				if err := errors.Join(mediaErr, sidecarErr); err != nil {
					if consumed, consumedErr := recoverConsumedArtifact(clipDir, artifact, now()); consumedErr == nil {
						if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{consumed}); err != nil {
							return result, err
						}
						continue
					}
					artifact = repairArtifact(artifact, "published artifact validation: "+err.Error(), now())
					if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
						return result, err
					}
					result.Repair++
				} else {
					result.Pending++ // intake owns the published -> consumed transition
				}
				continue
			}

			stageMedia := filepath.Join(watchDir, artifact.StagingPath)
			if err := verifyManifestFile(stageMedia, artifact); err != nil {
				// A crash may have happened after media publication but before its acknowledgement.
				targetMedia := filepath.Join(watchDir, artifact.MediaPath)
				targetSidecar := filepath.Join(watchDir, artifact.SidecarPath)
				if targetErr := errors.Join(verifyManifestFile(targetMedia, artifact), verifyPortableProvenance(targetSidecar, artifact)); targetErr == nil {
					artifact.State = acquisition.ArtifactPublished
					artifact.UpdatedAt = now().UTC()
					if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
						return result, err
					}
					result.Pending++
					continue
				}
				if consumed, consumedErr := recoverConsumedArtifact(clipDir, artifact, now()); consumedErr == nil {
					if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{consumed}); err != nil {
						return result, err
					}
					continue
				}
				artifact = repairArtifact(artifact, "staged artifact validation: "+err.Error(), now())
				if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
					return result, err
				}
				result.Repair++
				continue
			}

			stageSidecar := strings.TrimSuffix(stageMedia, filepath.Ext(stageMedia)) + ".info.json"
			targetSidecar := filepath.Join(watchDir, artifact.SidecarPath)
			if err := recoverSidecar(stageSidecar, targetSidecar, artifact); err != nil {
				artifact = repairArtifact(artifact, "staged sidecar validation: "+err.Error(), now())
				if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
					return result, err
				}
				result.Repair++
				continue
			}
			if err := publishFile(stageMedia, filepath.Join(watchDir, artifact.MediaPath)); err != nil {
				artifact = repairArtifact(artifact, "resume media publication: "+err.Error(), now())
				if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
					return result, err
				}
				result.Repair++
				continue
			}
			artifact.State = acquisition.ArtifactPublished
			artifact.UpdatedAt = now().UTC()
			if err := store.UpsertAcquisitionArtifacts(ctx, []acquisition.Artifact{artifact}); err != nil {
				return result, err
			}
			result.Published++
		}
	}
}

func recoverConsumedArtifact(clipDir string, artifact acquisition.Artifact, at time.Time) (acquisition.Artifact, error) {
	if clipDir == "" || artifact.ClipHash == "" {
		return artifact, errors.New("consumed location unavailable")
	}
	media, err := cliplibrary.ClipPath(clipDir, artifact.ClipHash, filepath.Ext(artifact.MediaPath))
	if err != nil {
		return artifact, err
	}
	if err := verifyManifestFile(media, artifact); err != nil {
		return artifact, err
	}
	sidecar := strings.TrimSuffix(media, filepath.Ext(media)) + ".info.json"
	if err := verifyPortableProvenance(sidecar, artifact); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return artifact, err
		}
		// The durable manifest has just revalidated the exact catalog bytes. Only a
		// missing sidecar is repairable: malformed, symlinked, or substituted files
		// remain held rather than being overwritten.
		if err := asset.WriteSidecarTags(media, asset.SidecarTags{
			SourceID: artifact.SourceID, AcquisitionID: artifact.AcquisitionID,
		}, true); err != nil {
			return artifact, err
		}
		if err := verifyPortableProvenance(sidecar, artifact); err != nil {
			return artifact, err
		}
	}
	relative, err := filepath.Rel(clipDir, media)
	if err != nil || !withinDir(relative) {
		return artifact, errors.New("consumed artifact escaped clip root")
	}
	artifact.State = acquisition.ArtifactConsumed
	artifact.MediaPath = filepath.ToSlash(relative)
	artifact.SidecarPath = strings.TrimSuffix(artifact.MediaPath, filepath.Ext(artifact.MediaPath)) + ".info.json"
	artifact.RepairReason = ""
	artifact.UpdatedAt = at.UTC()
	return artifact, nil
}

func verifyManifestFile(path string, artifact acquisition.Artifact) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("media is symlinked or not a regular file")
	}
	digest, size, err := asset.FileSHA256(path)
	if err != nil {
		return err
	}
	if digest != artifact.MediaSHA256 || size != artifact.MediaBytes {
		return errors.New("media digest or size changed")
	}
	return nil
}

func recoverSidecar(stagePath, targetPath string, artifact acquisition.Artifact) error {
	if targetPath == "" {
		return errors.New("manifest has no sidecar path")
	}
	if _, err := os.Lstat(targetPath); err == nil {
		return acceptExistingSidecar(stagePath, targetPath, artifact)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := verifyPortableProvenance(stagePath, artifact); err != nil {
		return err
	}
	return publishFile(stagePath, targetPath)
}

// acceptExistingSidecar decides whether a sidecar already sitting at the publication target may
// stand in for this artifact's own. It may when the bytes are identical to the staged sidecar, or
// when it carries this artifact's exact acquisition provenance — both are a previous attempt of the
// same publication. Anything else belongs to another item: it is reported with both identities and
// is never replaced.
func acceptExistingSidecar(stagePath, targetPath string, artifact acquisition.Artifact) error {
	if staged, err := os.ReadFile(stagePath); err == nil {
		if existing, err := os.ReadFile(targetPath); err == nil && bytes.Equal(staged, existing) {
			return nil
		}
	}
	provenanceErr := verifyPortableProvenance(targetPath, artifact)
	if provenanceErr == nil {
		return nil
	}
	tags, state := asset.ReadSidecarTagsState(strings.TrimSuffix(targetPath, ".info.json") + filepath.Ext(artifact.MediaPath))
	existing := "no valid Loomarr provenance"
	if state == asset.SidecarValid {
		existing = fmt.Sprintf("source %q, acquisition %q", tags.SourceID, tags.AcquisitionID)
	}
	return fmt.Errorf("sidecar %s is occupied by another item (%s), not this artifact (source %q, acquisition %q): %w",
		targetPath, existing, artifact.SourceID, artifact.AcquisitionID, provenanceErr)
}

// isSidecarExistsRepair recognises the historical repair shape written when publication refused a
// pre-existing sidecar. Such rows are re-driven through the staged flow, which now decides properly.
func isSidecarExistsRepair(reason string) bool {
	return strings.HasPrefix(reason, "publish sidecar: ") && strings.HasSuffix(reason, "already exists")
}

// manifestedMediaExists reports whether any location the manifest can name still holds a regular
// file: the watch-relative media or staging path, the clip-folder path, or the content-addressed
// catalog path. A transient "missing" repair is only settled as gone when none does.
func manifestedMediaExists(watchDir, clipDir string, artifact acquisition.Artifact) bool {
	candidates := []string{
		filepath.Join(watchDir, artifact.MediaPath),
		filepath.Join(watchDir, artifact.StagingPath),
	}
	if clipDir != "" {
		candidates = append(candidates, filepath.Join(clipDir, artifact.MediaPath))
		if artifact.ClipHash != "" {
			if catalog, err := cliplibrary.ClipPath(clipDir, artifact.ClipHash, filepath.Ext(artifact.MediaPath)); err == nil {
				candidates = append(candidates, catalog)
			}
		}
	}
	for _, path := range candidates {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func verifyPortableProvenance(path string, artifact acquisition.Artifact) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("sidecar is symlinked or not a regular file")
	}
	tags, state := asset.ReadSidecarTagsState(strings.TrimSuffix(path, ".info.json") + filepath.Ext(artifact.MediaPath))
	if state != asset.SidecarValid || tags.AcquisitionID != artifact.AcquisitionID || tags.SourceID != artifact.SourceID {
		return fmt.Errorf("sidecar does not carry exact acquisition provenance")
	}
	mediaPath := strings.TrimSuffix(path, ".info.json") + filepath.Ext(artifact.MediaPath)
	if !asset.SidecarFetchedByUs(mediaPath) {
		return errors.New("sidecar does not mark Loomarr ownership")
	}
	return nil
}

func repairArtifact(artifact acquisition.Artifact, reason string, at time.Time) acquisition.Artifact {
	artifact.State = acquisition.ArtifactRepair
	artifact.RepairReason = reason
	artifact.UpdatedAt = at.UTC()
	return artifact
}
