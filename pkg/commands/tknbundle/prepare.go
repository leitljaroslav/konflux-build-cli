package tknbundle

import (
	"fmt"
	"path/filepath"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

type preparedBundle struct {
	Files   []string // Final source files or rendered Kustomize output to publish.
	TaskDir string   // Directory of the first original selected file, before rendering.
	Version string
}

// prepareBundle requires a nonempty selection from resolveFiles and an existing workDir.
func (c *TektonBundlePush) prepareBundle(sourceFiles []string, workDir string) (preparedBundle, error) {
	override, err := newStepImageOverride(c.Params.StepsImage, c.Params.StepsImageStepNames)
	if err != nil {
		return preparedBundle{}, err
	}
	if err := applyStepImageOverridesInPlace(sourceFiles, override, workDir); err != nil {
		return preparedBundle{}, fmt.Errorf("replacing task step images: %w", err)
	}

	bundle := preparedBundle{
		Files:   sourceFiles,
		TaskDir: filepath.Dir(sourceFiles[0]),
	}
	versionFile := sourceFiles[0]
	if isTaskKustomized(sourceFiles) {
		bundlePath, err := buildKustomization(sourceFiles, workDir)
		if err != nil {
			return preparedBundle{}, err
		}
		versionFile = bundlePath
		bundle.Files = []string{bundlePath}
		l.Logger.Infof("Task is kustomized, using taskVersionFile %q", versionFile)
	}

	bundle.Version, err = readBundleVersion(versionFile)
	if err != nil {
		return preparedBundle{}, err
	}
	l.Logger.Infof("Got task version %q", bundle.Version)
	return bundle, nil
}
