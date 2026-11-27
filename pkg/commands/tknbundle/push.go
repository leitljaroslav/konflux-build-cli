package tknbundle

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/spf13/cobra"
	"go.podman.io/image/v5/docker/reference"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

type TektonBundlePushCliWrappers struct {
	TknCli  cliwrappers.TknCliInterface
	OrasCli cliwrappers.OrasCliInterface
}

type TektonBundlePush struct {
	Params        *TektonBundlePushParams
	CliWrappers   TektonBundlePushCliWrappers
	ResultsWriter common.ResultsWriterInterface
}

func (c *TektonBundlePush) Run() error {
	common.LogParameters(TektonBundlePushParamsConfig, c.Params)

	if err := c.validateParams(); err != nil {
		return err
	}

	sourceFiles, err := resolveFiles(c.Params.Source, c.Params.Context)
	if err != nil {
		return fmt.Errorf("resolving bundle files: %w", err)
	}
	l.Logger.Debugf("Resolved bundle files: %v", sourceFiles)

	workDir, err := os.MkdirTemp("", "tekton-bundle-push-*")
	if err != nil {
		return fmt.Errorf("creating bundle working directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(workDir); err != nil {
			l.Logger.Warnf("removing bundle working directory %q: %s", workDir, err)
		}
	}()

	bundle, err := c.prepareBundle(sourceFiles, workDir)
	if err != nil {
		return err
	}

	migration, err := c.ensureMigration(bundle)
	if err != nil {
		return err
	}

	source := sourceInfo{
		URL:      c.Params.URL,
		Revision: c.Params.Revision,
		Context:  c.Params.Context,
	}
	annots, err := assembleAnnotations(source, bundle, migration)
	if err != nil {
		return err
	}
	l.Logger.Info("Added annotations:")
	for _, key := range annots.keys {
		l.Logger.Infof("- %s=%s", key, annots.values[key])
	}

	result, err := c.pushBundle(bundle.Files, annots.values)
	if err != nil {
		return err
	}

	return c.writeResults(result)
}

func extractBundleDigest(pushOutput string) (string, error) {
	const outputPrefix = "Pushed Tekton Bundle to "
	for line := range strings.SplitSeq(pushOutput, "\n") {
		_, pushedReference, found := strings.Cut(line, outputPrefix)
		if !found {
			continue
		}

		fields := strings.Fields(strings.TrimSpace(pushedReference))
		if len(fields) == 0 {
			continue
		}
		digest := common.GetImageDigest(fields[0])
		if common.IsImageDigestValid(digest) {
			return digest, nil
		}
	}

	return "", fmt.Errorf("push output does not contain a valid digest after %q", outputPrefix)
}

func NewTektonBundlePush(cmd *cobra.Command) (*TektonBundlePush, error) {
	pushCmd := &TektonBundlePush{}
	params := &TektonBundlePushParams{}
	if err := common.ParseParameters(cmd, TektonBundlePushParamsConfig, params); err != nil {
		return nil, err
	}
	pushCmd.Params = params
	if err := pushCmd.initCliWrappers(); err != nil {
		return nil, err
	}
	pushCmd.ResultsWriter = common.NewResultsWriter()
	return pushCmd, nil
}

func (c *TektonBundlePush) initCliWrappers() error {
	executor := cliwrappers.NewCliExecutor()
	tknCli, err := cliwrappers.NewTknCli(executor)
	if err != nil {
		return err
	}
	c.CliWrappers.TknCli = tknCli
	return nil
}

func (c *TektonBundlePush) pushBundle(files []string, annotationMap map[string]string) (TektonBundlePushResult, error) {
	l.Logger.Infof("Pushing bundle %s to registry", c.Params.Image)
	if c.CliWrappers.TknCli == nil {
		l.Logger.Errorf("Failed to push bundle %s to registry", c.Params.Image)
		return TektonBundlePushResult{}, fmt.Errorf("pushing bundle %q to registry: tkn CLI wrapper is not configured", c.Params.Image)
	}

	args := &cliwrappers.TknPushArgs{
		DestinationImage: c.Params.Image,
		Filenames:        files,
		Annotations:      annotationMap,
	}
	retryer := cliwrappers.NewRetryer(func() (string, string, int, error) {
		stdout, stderr, err := c.CliWrappers.TknCli.Push(args)
		if err != nil {
			return stdout, stderr, 1, err
		}
		return stdout, stderr, 0, nil
	}).WithConstantDelay(time.Second).WithMaxAttempts(3)
	stdout, _, _, err := retryer.Run()
	if err != nil {
		l.Logger.Errorf("Failed to push bundle %s to registry", c.Params.Image)
		return TektonBundlePushResult{}, fmt.Errorf("pushing bundle %q to registry: %w", c.Params.Image, err)
	}
	l.Logger.Debugf("tkn bundle push stdout:\n%s", stdout)
	imageDigest, err := extractBundleDigest(stdout)
	if err != nil {
		return TektonBundlePushResult{}, fmt.Errorf("extracting bundle digest: %w", err)
	}
	return TektonBundlePushResult{
		ImageURL:    c.Params.Image,
		ImageDigest: imageDigest,
		ImageRef:    c.Params.Image + "@" + imageDigest,
	}, nil
}

func (c *TektonBundlePush) validateParams() error {
	// validate the image
	_, err := reference.ParseNormalizedNamed(c.Params.Image)
	if err != nil {
		return fmt.Errorf("invalid image: %w", err)
	}
	// validate the source
	info, err := os.Stat(c.Params.Source)
	if err != nil {
		return fmt.Errorf("checking source directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source path %q is not a directory", info.Name())
	}

	if _, err := newStepImageOverride(c.Params.StepsImage, c.Params.StepsImageStepNames); err != nil {
		return err
	}

	return nil
}
