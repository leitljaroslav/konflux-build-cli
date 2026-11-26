package cliwrappers_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

func TestNewTknCli_MissingExecutable(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("PATH", t.TempDir())

	tknCli, err := cliwrappers.NewTknCli(&mockExecutor{})

	g.Expect(tknCli).To(BeNil())
	g.Expect(err).To(MatchError(
		`required executable "tkn" not found in PATH`,
	))
}

func TestTknCli_PushPreservesCSVValues(t *testing.T) {
	g := NewWithT(t)

	const image = "registry.example.com/org/bundle:latest"
	const filename = `tasks/build, "release".yaml`
	const description = `Build, test "release"=ready`
	const expectedStdout = "bundle pushed"
	const expectedStderr = "push progress"

	executed := false
	executor := &mockExecutor{
		executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
			executed = true
			g.Expect(cmd.Name).To(Equal("tkn"))
			g.Expect(len(cmd.Args)).To(BeNumerically(">=", 2))
			g.Expect(cmd.Args[:2]).To(Equal([]string{"bundle", "push"}))

			// Match tkn's StringSlice flags to exercise its actual CSV parsing.
			flags := pflag.NewFlagSet("push", pflag.ContinueOnError)
			flags.StringSlice("filenames", nil, "")
			flags.StringSlice("annotate", nil, "")
			g.Expect(flags.Parse(cmd.Args[2:])).To(Succeed())
			g.Expect(flags.Args()).To(Equal([]string{image}))

			filenames, err := flags.GetStringSlice("filenames")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(filenames).To(Equal([]string{filename, "tasks/test.yaml"}))

			annotations, err := flags.GetStringSlice("annotate")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(annotations).To(ConsistOf(
				"description="+description,
				"title=Release bundle",
			))

			return expectedStdout, expectedStderr, 0, nil
		},
	}
	tknCli := &cliwrappers.TknCli{Executor: executor}

	stdout, stderr, err := tknCli.Push(&cliwrappers.TknPushArgs{
		DestinationImage: image,
		Filenames:        []string{filename, "tasks/test.yaml"},
		Annotations: map[string]string{
			"description": description,
			"title":       "Release bundle",
		},
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(executed).To(BeTrue())
	g.Expect(stdout).To(Equal(expectedStdout))
	g.Expect(stderr).To(Equal(expectedStderr))
}

func TestTknCli_PushUsesDockerConfigDir(t *testing.T) {
	g := NewWithT(t)

	t.Setenv("DOCKER_CONFIG", "/old/config")
	configDir := t.TempDir()
	inheritedPath := os.Getenv("PATH")
	executed := false
	executor := &mockExecutor{
		executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
			executed = true
			// Environ applies os/exec's last-value-wins behavior for duplicate keys.
			child := &exec.Cmd{Env: cmd.Env}
			env := child.Environ()
			g.Expect(env).To(ContainElement("DOCKER_CONFIG=" + configDir))
			g.Expect(env).NotTo(ContainElement("DOCKER_CONFIG=/old/config"))
			g.Expect(env).To(ContainElement("PATH=" + inheritedPath))
			g.Expect(os.Getenv("DOCKER_CONFIG")).To(Equal("/old/config"))

			return "", "", 0, nil
		},
	}

	tknCli := &cliwrappers.TknCli{Executor: executor}

	_, _, err := tknCli.Push(&cliwrappers.TknPushArgs{
		DestinationImage: "registry.example.com/org/bundle:latest",
		Filenames:        []string{"tasks/build.yaml"},
		DockerConfigDir:  configDir,
	})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(executed).To(BeTrue())
	g.Expect(os.Getenv("DOCKER_CONFIG")).To(Equal("/old/config"))
}

func TestTknCli_PushReturnsExecutionError(t *testing.T) {
	g := NewWithT(t)
	executionErr := errors.New("process failed")
	executor := &mockExecutor{
		executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return "partial output", "auth failed", 1, executionErr
		},
	}
	tknCli := &cliwrappers.TknCli{Executor: executor}

	stdout, stderr, err := tknCli.Push(&cliwrappers.TknPushArgs{
		DestinationImage: "registry.example.com/org/bundle:latest",
		Filenames:        []string{"tasks/build.yaml"},
	})

	g.Expect(stdout).To(Equal("partial output"))
	g.Expect(stderr).To(Equal("auth failed"))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("exit code 1"))
	g.Expect(errors.Is(err, executionErr)).To(BeTrue())
}
