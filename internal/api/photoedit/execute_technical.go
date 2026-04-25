package photoedit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/photoprism/photoprism/internal/entity/query"
	"github.com/photoprism/photoprism/internal/photoprism"
	"github.com/photoprism/photoprism/internal/photoprism/get"
)

// executeTechnicalTool applies technical operations when a runtime exists.
func executeTechnicalTool(photoUID, tool string, params map[string]any, spec ToolSpec) (status, message string, err error) {
	if !spec.Executable {
		return "failed", "technical tool planned; runtime integration pending", nil
	}

	switch tool {
	case "rotate":
		if execErr := executeRotateTool(photoUID, params); execErr != nil {
			return "failed", "", execErr
		}
		return "succeeded", "", nil
	case "crop":
		if execErr := executeCropTool(photoUID, params); execErr != nil {
			return "failed", "", execErr
		}
		return "succeeded", "", nil
	case "exposure":
		if execErr := executeExposureTool(photoUID, params); execErr != nil {
			return "failed", "", execErr
		}
		return "succeeded", "", nil
	case "contrast":
		if execErr := executeContrastTool(photoUID, params); execErr != nil {
			return "failed", "", execErr
		}
		return "succeeded", "", nil
	case "saturation":
		if execErr := executeSaturationTool(photoUID, params); execErr != nil {
			return "failed", "", execErr
		}
		return "succeeded", "", nil
	case "temperature":
		if execErr := executeTemperatureTool(photoUID, params); execErr != nil {
			return "failed", "", execErr
		}
		return "succeeded", "", nil
	case "auto_crop_straighten":
		adjusted := map[string]any{}
		for k, v := range params {
			adjusted[k] = v
		}
		if _, ok := floatParam(adjusted, "degrees"); !ok {
			adjusted["degrees"] = 0.0
		}

		if execErr := executeRotateTool(photoUID, adjusted); execErr != nil {
			return "failed", "", execErr
		}
		return "succeeded", "straighten applied; persistent crop not yet supported", nil
	default:
		return "failed", "unsupported executable tool", nil
	}
}

// executeRotateTool rotates the primary file by updating Exif orientation and reindexing.
func executeRotateTool(photoUID string, params map[string]any) error {
	f, err := query.FileByPhotoUID(photoUID)
	if err != nil {
		return err
	}

	degrees := 90.0
	if v, ok := floatParam(params, "degrees"); ok {
		degrees = v
	}

	nextOrientation, err := rotateOrientation(f.Orientation(), degrees)
	if err != nil {
		return err
	}

	fileName := photoprism.FileName(f.FileRoot, f.FileName)
	mf, err := photoprism.NewMediaFile(fileName)
	if err != nil {
		return err
	}

	if err = mf.ChangeOrientation(nextOrientation); err != nil {
		return err
	}

	return reindexMediaFile(mf.FileName())
}

// executeExposureTool applies an exposure adjustment to the primary photo file.
func executeExposureTool(photoUID string, params map[string]any) error {
	f, err := query.FileByPhotoUID(photoUID)
	if err != nil {
		return err
	}

	fileName := photoprism.FileName(f.FileRoot, f.FileName)
	mf, err := photoprism.NewMediaFile(fileName)
	if err != nil {
		return err
	}

	if !mf.IsImage() {
		return errors.New("exposure only supports image files")
	}

	value := 0.0
	if v, ok := floatParam(params, "value"); ok {
		value = v
	}

	if value < -2.0 {
		value = -2.0
	} else if value > 2.0 {
		value = 2.0
	}

	if value == 0 {
		return nil
	}

	conf := get.Config()
	if !conf.ImageMagickEnabled() {
		return errors.New("imagemagick is not enabled")
	}

	brightness := int(value * 50)
	if brightness < -100 {
		brightness = -100
	} else if brightness > 100 {
		brightness = 100
	}

	adj := fmt.Sprintf("%dx0", brightness)
	if err = applyImageMagickTransform(conf.ImageMagickBin(), mf.FileName(), "-brightness-contrast", adj); err != nil {
		return err
	}

	return reindexMediaFile(mf.FileName())
}

// executeCropTool crops an image based on width and height percentages.
func executeCropTool(photoUID string, params map[string]any) error {
	fileName, err := imageFileNameByPhotoUID(photoUID)
	if err != nil {
		return err
	}

	conf := get.Config()
	if !conf.ImageMagickEnabled() {
		return errors.New("imagemagick is not enabled")
	}

	width := clampFloat(paramWithDefault(params, "width", 0.9), 0.1, 1.0)
	height := clampFloat(paramWithDefault(params, "height", 0.9), 0.1, 1.0)

	crop := fmt.Sprintf("%dx%d%%+0+0", int(width*100), int(height*100))
	if err = applyImageMagickTransform(conf.ImageMagickBin(), fileName, "-gravity", "center", "-crop", crop, "+repage"); err != nil {
		return err
	}

	return reindexMediaFile(fileName)
}

// executeContrastTool applies contrast changes.
func executeContrastTool(photoUID string, params map[string]any) error {
	fileName, err := imageFileNameByPhotoUID(photoUID)
	if err != nil {
		return err
	}

	conf := get.Config()
	if !conf.ImageMagickEnabled() {
		return errors.New("imagemagick is not enabled")
	}

	value := clampFloat(paramWithDefault(params, "value", 0), -1, 1)
	adj := fmt.Sprintf("0x%d", int(value*60))
	if err = applyImageMagickTransform(conf.ImageMagickBin(), fileName, "-brightness-contrast", adj); err != nil {
		return err
	}

	return reindexMediaFile(fileName)
}

// executeSaturationTool applies saturation changes.
func executeSaturationTool(photoUID string, params map[string]any) error {
	fileName, err := imageFileNameByPhotoUID(photoUID)
	if err != nil {
		return err
	}

	conf := get.Config()
	if !conf.ImageMagickEnabled() {
		return errors.New("imagemagick is not enabled")
	}

	value := clampFloat(paramWithDefault(params, "value", 0), -1, 1)
	sat := int(clampFloat((1+value)*100, 20, 220))
	if err = applyImageMagickTransform(conf.ImageMagickBin(), fileName, "-modulate", fmt.Sprintf("100,%d,100", sat)); err != nil {
		return err
	}

	return reindexMediaFile(fileName)
}

// executeTemperatureTool applies warm/cool color balance changes.
func executeTemperatureTool(photoUID string, params map[string]any) error {
	fileName, err := imageFileNameByPhotoUID(photoUID)
	if err != nil {
		return err
	}

	conf := get.Config()
	if !conf.ImageMagickEnabled() {
		return errors.New("imagemagick is not enabled")
	}

	value := clampFloat(paramWithDefault(params, "value", 0), -1, 1)
	if value == 0 {
		return nil
	}

	redMul := 1.0
	blueMul := 1.0
	if value > 0 {
		redMul = 1.0 + (0.25 * value)
		blueMul = 1.0 - (0.25 * value)
	} else {
		cool := -value
		redMul = 1.0 - (0.25 * cool)
		blueMul = 1.0 + (0.25 * cool)
	}

	if err = applyImageMagickTransform(
		conf.ImageMagickBin(),
		fileName,
		"-colorspace", "RGB",
		"-channel", "R", "-evaluate", "multiply", fmt.Sprintf("%.3f", redMul), "+channel",
		"-channel", "B", "-evaluate", "multiply", fmt.Sprintf("%.3f", blueMul), "+channel",
	); err != nil {
		return err
	}

	return reindexMediaFile(fileName)
}

// applyImageMagickTransform applies transform operations using a temp file and atomic rename.
func applyImageMagickTransform(bin, fileName string, args ...string) error {
	if strings.TrimSpace(bin) == "" {
		return errors.New("imagemagick binary not configured")
	}

	dir := filepath.Dir(fileName)
	tmp, err := os.CreateTemp(dir, "pp-edit-exposure-*.jpg")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpName)
		return closeErr
	}

	defer func() {
		_ = os.Remove(tmpName)
	}()

	cmdArgs := make([]string, 0, len(args)+2)
	cmdArgs = append(cmdArgs, fileName)
	cmdArgs = append(cmdArgs, args...)
	cmdArgs = append(cmdArgs, tmpName)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// #nosec G204 -- command uses validated config binary and server-side file paths.
	cmd := exec.CommandContext(ctx, bin, cmdArgs...)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		if len(out) > 0 {
			return fmt.Errorf("imagemagick transform failed: %s", strings.TrimSpace(string(out)))
		}
		return runErr
	}

	if err = os.Rename(tmpName, fileName); err != nil {
		return err
	}

	return nil
}

// reindexMediaFile updates database metadata after file mutations.
func reindexMediaFile(fileName string) error {
	conf := get.Config()
	if res := get.Index().FileName(fileName, photoprism.IndexOptionsSingle(conf)); res.Failed() {
		return fmt.Errorf("reindex failed: %w", res.Err)
	}

	return nil
}

// rotateOrientation returns the next Exif orientation for 90-degree rotations.
func rotateOrientation(current int, degrees float64) (int, error) {
	steps := int(degrees / 90)
	if degrees != float64(steps*90) {
		return 0, errors.New("rotate only supports 90-degree steps")
	}

	steps = steps % 4
	if steps < 0 {
		steps += 4
	}

	if steps == 0 {
		return current, nil
	}

	seq := []int{1, 6, 3, 8}
	idx := -1
	for i, o := range seq {
		if o == current {
			idx = i
			break
		}
	}

	if idx < 0 {
		idx = 0
	}

	return seq[(idx+steps)%len(seq)], nil
}

// floatParam returns a numeric parameter as float64 when present.
func floatParam(params map[string]any, key string) (float64, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}

	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	default:
		return 0, false
	}
}
