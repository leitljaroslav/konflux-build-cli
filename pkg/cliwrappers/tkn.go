package cliwrappers

import (
	"encoding/csv"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const tknExecutableName string = "tkn"

type TknPushArgs struct {
	DestinationImage string
	Filenames        []string
	Annotations      map[string]string
	DockerConfigDir  string // Directory containing config.json.
}

var tknLog = l.Logger.WithField("logger", "TknCli")

type TknCliInterface interface {
	Push(args *TknPushArgs) (stdout, stderr string, err error)
}

type TknCli struct {
	Executor CliExecutorInterface
}

var _ TknCliInterface = &TknCli{}

func NewTknCli(executor CliExecutorInterface) (*TknCli, error) {
	cliAvailable, err := CheckCliToolAvailable(tknExecutableName)
	if err != nil {
		return nil, err
	}
	if !cliAvailable {
		return nil, fmt.Errorf("required executable %q not found in PATH", tknExecutableName)
	}

	return &TknCli{
		Executor: executor,
	}, nil
}

func (b *TknCli) Push(args *TknPushArgs) (stdout, stderr string, err error) {
	if args == nil {
		return "", "", fmt.Errorf("push args must not be nil")
	}
	if args.DestinationImage == "" {
		return "", "", fmt.Errorf("destination image must not be empty")
	}
	if len(args.Filenames) == 0 {
		return "", "", fmt.Errorf("at least one bundle file is required")
	}

	tknArgs := []string{"bundle", "push", args.DestinationImage}
	for _, filename := range args.Filenames {
		if filename == "" {
			return "", "", fmt.Errorf("bundle filename must not be empty")
		}
		encoded, err := encodeTknCSVValue(filename)
		if err != nil {
			return "", "", fmt.Errorf("encoding bundle filename: %w", err)
		}
		tknArgs = append(tknArgs, "--filenames", encoded)
	}

	for _, key := range slices.Sorted(maps.Keys(args.Annotations)) {
		value := key + "=" + args.Annotations[key]
		encoded, err := encodeTknCSVValue(value)
		if err != nil {
			return "", "", fmt.Errorf("encoding annotation %q: %w", key, err)
		}
		tknArgs = append(tknArgs, "--annotate", encoded)
	}

	tknLog.Debugf("Running command:\n%s", shellJoin("tkn", tknArgs...))
	cmd := Cmd{
		Name:      "tkn",
		Args:      tknArgs,
		LogOutput: true,
	}

	if args.DockerConfigDir != "" {
		cmd.Env = append(os.Environ(), "DOCKER_CONFIG="+args.DockerConfigDir)
	}

	stdout, stderr, exitCode, err := b.Executor.Execute(cmd)
	if err != nil {
		return stdout, stderr, fmt.Errorf(
			"tkn bundle push failed with exit code %d: %w", exitCode, err,
		)
	}

	return stdout, stderr, nil
}

func encodeTknCSVValue(value string) (string, error) {
	var output strings.Builder
	writer := csv.NewWriter(&output)

	if err := writer.Write([]string{value}); err != nil {
		return "", err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return strings.TrimSuffix(output.String(), "\n"), nil
}
