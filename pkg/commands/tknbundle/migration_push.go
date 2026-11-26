package tknbundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/opencontainers/go-digest"
)

// ensureMigration reuses or publishes the migration image
func (c *TektonBundlePush) ensureMigration(bundle preparedBundle) (*migrationImage, error) {
	migration, err := resolveMigrationImage(bundle, c.Params.Image)
	if err != nil {
		return nil, fmt.Errorf("processing migration: %w", err)
	}
	if migration == nil {
		return nil, nil
	}
	if migration.Digest != "" {
		l.Logger.Infof("Found migration image %q, digest %q", migration.Reference, migration.Digest)
		return migration, nil
	}
	if err := c.pushMigrationImage(migration); err != nil {
		return nil, fmt.Errorf("pushing migration image: %w", err)
	}
	return migration, nil
}

func (c *TektonBundlePush) pushMigrationImage(migrationImg *migrationImage) error {
	if c.CliWrappers.OrasCli == nil {
		orasCli, err := cliwrappers.NewOrasCli(cliwrappers.NewCliExecutor())
		if err != nil {
			return err
		}
		c.CliWrappers.OrasCli = orasCli
	}

	registryAuth, err := common.SelectRegistryAuthFromDefaultAuthFile(c.Params.Image)
	if err != nil {
		return fmt.Errorf("selecting registry authentication: %w", err)
	}

	registryConfig, err := os.CreateTemp("", "migration-registry-config-*.json")
	if err != nil {
		return fmt.Errorf("creating registry config file: %w", err)
	}
	registryConfigPath := registryConfig.Name()
	defer func() {
		if err := os.Remove(registryConfigPath); err != nil {
			l.Logger.Warnf("removing registry config %q: %s", registryConfigPath, err)
		}
	}()

	config, err := json.Marshal(common.RegistryAuths{
		Auths: map[string]common.AuthEntry{
			registryAuth.Registry: {Auth: registryAuth.Token},
		},
	})
	if err != nil {
		_ = registryConfig.Close()
		return fmt.Errorf("encoding registry config: %w", err)
	}
	if _, err := registryConfig.Write(config); err != nil {
		_ = registryConfig.Close()
		return fmt.Errorf("writing registry config: %w", err)
	}
	if err := registryConfig.Close(); err != nil {
		return fmt.Errorf("closing registry config: %w", err)
	}

	pushArgs := &cliwrappers.OrasPushArgs{
		DestinationImage: migrationImg.Reference,
		FileName:         filepath.Base(migrationImg.FilePath),
		Workdir:          filepath.Dir(migrationImg.FilePath),
		Annotations:      []string{"dev.konflux-ci.task.is-migration=true"},
		RegistryConfig:   registryConfigPath,
		Format:           "go-template",
		Template:         "{{.digest}}",
	}
	retryer := cliwrappers.NewRetryer(func() (string, string, int, error) {
		stdout, stderr, err := c.CliWrappers.OrasCli.Push(pushArgs)
		if err != nil {
			return stdout, stderr, 1, err
		}
		return stdout, stderr, 0, nil
	}).WithConstantDelay(time.Second).WithMaxAttempts(3)
	stdout, _, _, err := retryer.Run()
	if err != nil {
		return fmt.Errorf("ORAS push failed: %w", err)
	}

	pushedDigest, err := digest.Parse(strings.TrimSpace(stdout))
	if err != nil {
		return fmt.Errorf("parsing ORAS push digest %q: %w", stdout, err)
	}
	migrationImg.Digest = pushedDigest.String()
	l.Logger.Infof("Pushed migration image %s, digest %s", migrationImg.Reference, migrationImg.Digest)
	return nil
}
