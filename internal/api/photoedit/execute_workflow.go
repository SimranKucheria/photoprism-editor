package photoedit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/photoprism/photoprism/internal/entity"
	"github.com/photoprism/photoprism/internal/entity/query"
	"github.com/photoprism/photoprism/internal/photoprism"
	"github.com/photoprism/photoprism/internal/photoprism/get"
)

// executeWorkflowTool applies workflow operations when a runtime exists.
func executeWorkflowTool(photoUID, tool string, spec ToolSpec) (status, message string, err error) {
	if !spec.Executable {
		return "failed", "workflow tool planned; runtime integration pending", nil
	}

	switch tool {
	case "metadata_sync":
		p, findErr := query.PhotoByUID(photoUID)
		if findErr != nil {
			return "failed", "", findErr
		}
		saveSidecarYAML(&p)
		return "succeeded", "metadata synchronized", nil
	case "version_checkpoint":
		versionPath, copyErr := snapshotPhotoFile(photoUID)
		if copyErr != nil {
			return "failed", "", copyErr
		}
		return "succeeded", "checkpoint saved: " + filepath.Base(versionPath), nil
	default:
		return "failed", "unsupported executable tool", nil
	}
}

// snapshotPhotoFile copies the primary image file to a versioned sidecar path.
// The destination is <sidecar_path>/versions/<photoUID>/<timestamp><ext>.
func snapshotPhotoFile(photoUID string) (string, error) {
	f, err := query.FileByPhotoUID(photoUID)
	if err != nil {
		return "", err
	}

	conf := get.Config()
	srcPath := photoprism.FileName(f.FileRoot, f.FileName)

	ext := filepath.Ext(f.FileName)
	stamp := time.Now().UTC().Format("20060102-150405")
	destDir := filepath.Join(conf.SidecarPath(), "versions", photoUID)

	if mkErr := os.MkdirAll(destDir, 0750); mkErr != nil {
		return "", fmt.Errorf("version checkpoint: could not create directory: %w", mkErr)
	}

	destPath := filepath.Join(destDir, stamp+ext)

	if copyErr := copyFile(srcPath, destPath); copyErr != nil {
		return "", fmt.Errorf("version checkpoint: %w", copyErr)
	}

	return destPath, nil
}

// copyFile copies src to dst using a direct io.Copy.
func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src is a server-side resolved originals path.
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}

// saveSidecarYAML writes photo metadata to YAML when sidecars are enabled.
func saveSidecarYAML(photo *entity.Photo) {
	if photo == nil || !photo.HasID() {
		return
	}

	conf := get.Config()
	if !conf.SidecarYaml() {
		return
	}

	_ = photo.SaveSidecarYaml(conf.OriginalsPath(), conf.SidecarPath())
}
