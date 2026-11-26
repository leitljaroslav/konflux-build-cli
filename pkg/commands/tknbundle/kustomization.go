package tknbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	kustomizationAPIVersion string      = "kustomize.config.k8s.io/v1beta1"
	kustomizationKind       string      = "Kustomization"
	fileMode                os.FileMode = 0o600
)

// buildKustomization renders the selected files into bundle.yaml in workDir.
// The source paths must be absolute, and workDir must already exist.
func buildKustomization(files []string, workDir string) (string, error) {
	buildDir, err := sourceKustomizationDir(files)
	if err != nil {
		return "", err
	}
	if buildDir == "" {
		kustomization, err := json.Marshal(types.Kustomization{
			TypeMeta: types.TypeMeta{
				APIVersion: kustomizationAPIVersion,
				Kind:       kustomizationKind,
			},
			Resources: files,
		})
		if err != nil {
			return "", fmt.Errorf("encoding bundle kustomization: %w", err)
		}
		if err := os.WriteFile(filepath.Join(workDir, "kustomization.yaml"), kustomization, fileMode); err != nil {
			return "", fmt.Errorf("writing bundle kustomization: %w", err)
		}
		buildDir = workDir
	}
	options := krusty.MakeDefaultOptions()
	// Source resources can live outside the temporary build directory.
	options.LoadRestrictions = types.LoadRestrictionsNone
	resources, err := krusty.MakeKustomizer(options).Run(filesys.MakeFsOnDisk(), buildDir)
	if err != nil {
		return "", fmt.Errorf("building bundle kustomization: %w", err)
	}
	bundle, err := resources.AsYaml()
	if err != nil {
		return "", fmt.Errorf("encoding bundle resources: %w", err)
	}
	bundlePath := filepath.Join(workDir, "bundle.yaml")
	if err := os.WriteFile(bundlePath, bundle, fileMode); err != nil {
		return "", fmt.Errorf("writing bundle resources: %w", err)
	}
	return bundlePath, nil
}

// sourceKustomizationDir checks for a Kustomization beside the first selected
// file, regardless of whether that Kustomization was selected or excluded.
// Later selected directories do not affect the task's Kustomization.
func sourceKustomizationDir(files []string) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	buildDir := filepath.Dir(files[0])
	for _, name := range []string{"kustomization.yaml", "kustomization.yml"} {
		path := filepath.Join(buildDir, name)
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("checking source kustomization %q: %w", path, err)
		}
		if info.Mode().IsRegular() {
			return buildDir, nil
		}
	}
	return "", nil
}

// isTaskKustomized discovers a Kustomization beside the first selected file.
func isTaskKustomized(files []string) bool {
	buildDir, err := sourceKustomizationDir(files)
	return err == nil && buildDir != ""
}
