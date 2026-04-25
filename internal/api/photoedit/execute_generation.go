package photoedit

import (
	"errors"
	"fmt"

	"github.com/photoprism/photoprism/internal/entity/query"
	"github.com/photoprism/photoprism/internal/photoprism"
	"github.com/photoprism/photoprism/internal/photoprism/get"
)

// executeGenerationTool applies generation-style tools using available local runtimes.
func executeGenerationTool(photoUID, tool string, params map[string]any, spec ToolSpec) (status, message string, err error) {
	if !spec.Executable {
		return "failed", "generative tool planned; runtime integration pending", nil
	}

	fileName, imageErr := imageFileNameByPhotoUID(photoUID)
	if imageErr != nil {
		return "failed", "", imageErr
	}

	conf := get.Config()
	if !conf.ImageMagickEnabled() {
		return "failed", "", errors.New("imagemagick is not enabled")
	}

	switch tool {
	case "inpaint_object":
		strength := clampFloat(paramWithDefault(params, "strength", 0.4), 0, 1)
		radius := int(1 + strength*3)
		opacity := int(20 + strength*35)
		args := []string{"-blur", fmt.Sprintf("0x%d", radius), "-modulate", fmt.Sprintf("100,%d,100", 100-opacity/2)}
		if applyErr := applyImageMagickTransform(conf.ImageMagickBin(), fileName, args...); applyErr != nil {
			return "failed", "", applyErr
		}
		if reindexErr := reindexMediaFile(fileName); reindexErr != nil {
			return "failed", "", reindexErr
		}
		return "succeeded", "inpaint fallback applied", nil
	case "outpaint_canvas":
		scale := clampFloat(paramWithDefault(params, "scale", 0.1), 0.02, 0.5)
		border := int(scale * 100)
		if border < 1 {
			border = 1
		}
		args := []string{"-background", "black", "-gravity", "center", "-border", fmt.Sprintf("%dx%d", border, border), "-resize", "100%x100%!"}
		if applyErr := applyImageMagickTransform(conf.ImageMagickBin(), fileName, args...); applyErr != nil {
			return "failed", "", applyErr
		}
		if reindexErr := reindexMediaFile(fileName); reindexErr != nil {
			return "failed", "", reindexErr
		}
		return "succeeded", "outpaint fallback applied", nil
	case "transfer_style":
		args := []string{"-modulate", "105,125,100", "-contrast", "-contrast"}
		if applyErr := applyImageMagickTransform(conf.ImageMagickBin(), fileName, args...); applyErr != nil {
			return "failed", "", applyErr
		}
		if reindexErr := reindexMediaFile(fileName); reindexErr != nil {
			return "failed", "", reindexErr
		}
		return "succeeded", "style transfer fallback applied", nil
	case "replace_background":
		color := "white"
		if c, ok := params["color"].(string); ok && c != "" {
			color = c
		}
		args := []string{"-background", color, "-alpha", "remove", "-alpha", "off"}
		if applyErr := applyImageMagickTransform(conf.ImageMagickBin(), fileName, args...); applyErr != nil {
			return "failed", "", applyErr
		}
		if reindexErr := reindexMediaFile(fileName); reindexErr != nil {
			return "failed", "", reindexErr
		}
		return "succeeded", "background replacement fallback applied", nil
	default:
		return "failed", "unsupported executable tool", nil
	}
}

// imageFileNameByPhotoUID resolves the primary image file path for a photo UID.
func imageFileNameByPhotoUID(photoUID string) (string, error) {
	f, err := query.FileByPhotoUID(photoUID)
	if err != nil {
		return "", err
	}

	fileName := photoprism.FileName(f.FileRoot, f.FileName)
	mf, err := photoprism.NewMediaFile(fileName)
	if err != nil {
		return "", err
	}

	if !mf.IsImage() {
		return "", errors.New("tool only supports image files")
	}

	return mf.FileName(), nil
}

// paramWithDefault returns the numeric parameter value or fallback.
func paramWithDefault(params map[string]any, key string, fallback float64) float64 {
	if v, ok := floatParam(params, key); ok {
		return v
	}

	return fallback
}

// clampFloat limits value to the inclusive [minValue, maxValue] range.
func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}

	if value > maxValue {
		return maxValue
	}

	return value
}
