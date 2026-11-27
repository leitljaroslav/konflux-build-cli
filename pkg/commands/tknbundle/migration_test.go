package tknbundle

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/types"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	registryv2 "github.com/distribution/distribution/v3/registry/api/v2"
	. "github.com/onsi/gomega"
)

func TestResolveMigrationImageReusesMatchingImage(t *testing.T) {
	g := NewWithT(t)
	taskDir := t.TempDir()
	const taskVersion = "1.2.3"
	const image = "registry.example.com/team/bundle:latest"
	content := []byte("#!/bin/sh\necho migrate\n")
	writeMigrationScript(g, taskDir, taskVersion, content)

	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	tagPrefix := fmt.Sprintf("migration-%s-%s-", taskVersion, checksum)
	newestTag := tagPrefix + "1700000000"
	manifestDigest := "sha256:" + strings.Repeat("a", 64)

	got, err := resolveMigrationImageWithSearch(preparedBundle{TaskDir: taskDir, Version: taskVersion}, image, func(_ context.Context, ref types.ImageReference, prefix string) ([]registryTag, error) {
		g.Expect(prefix).To(Equal(tagPrefix))
		g.Expect(reference.TrimNamed(ref.DockerReference()).Name()).To(Equal("registry.example.com/team/bundle"))
		return []registryTag{{Name: newestTag, Digest: manifestDigest}}, nil
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).NotTo(BeNil())
	g.Expect(got.Reference).To(Equal("registry.example.com/team/bundle:" + newestTag))
	g.Expect(got.FilePath).To(Equal(filepath.Join(taskDir, "migrations", taskVersion+".sh")))
	g.Expect(got.Tag).To(Equal(newestTag))
	g.Expect(got.Digest).To(Equal(manifestDigest))
}

func TestResolveMigrationImageCreatesTagWhenNoMatchExists(t *testing.T) {
	g := NewWithT(t)
	taskDir := t.TempDir()
	const taskVersion = "2.0"
	const image = "registry.example.com/team/bundle:latest"
	content := []byte("migration script")
	writeMigrationScript(g, taskDir, taskVersion, content)

	checksum := fmt.Sprintf("%x", sha256.Sum256(content))
	tagPrefix := fmt.Sprintf("migration-%s-%s-", taskVersion, checksum)
	got, err := resolveMigrationImageWithSearch(preparedBundle{TaskDir: taskDir, Version: taskVersion}, image, func(_ context.Context, _ types.ImageReference, prefix string) ([]registryTag, error) {
		g.Expect(prefix).To(Equal(tagPrefix))
		return nil, nil
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).NotTo(BeNil())
	g.Expect(got.Digest).To(BeEmpty())
	g.Expect(got.Tag).To(HavePrefix(tagPrefix))

	_, err = strconv.ParseInt(strings.TrimPrefix(got.Tag, tagPrefix), 10, 64)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Reference).To(Equal("registry.example.com/team/bundle:" + got.Tag))
}

func TestResolveMigrationImageSkipsMissingMigration(t *testing.T) {
	taskDir := t.TempDir()
	search := func(context.Context, types.ImageReference, string) ([]registryTag, error) {
		panic("unexpected migration tag search for missing migration")
	}

	t.Run("empty version", func(t *testing.T) {
		g := NewWithT(t)
		got, err := resolveMigrationImageWithSearch(preparedBundle{TaskDir: taskDir}, "registry.example.com/team/bundle:latest", search)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("script does not exist", func(t *testing.T) {
		g := NewWithT(t)
		got, err := resolveMigrationImageWithSearch(preparedBundle{TaskDir: taskDir, Version: "1.0"}, "registry.example.com/team/bundle:latest", search)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(BeNil())
	})
}

func TestResolveMigrationImagePropagatesSearchError(t *testing.T) {
	g := NewWithT(t)
	taskDir := t.TempDir()
	writeMigrationScript(g, taskDir, "1.0", []byte("migration"))
	wantErr := errors.New("registry unavailable")

	_, err := resolveMigrationImageWithSearch(preparedBundle{TaskDir: taskDir, Version: "1.0"}, "registry.example.com/team/bundle:latest", func(context.Context, types.ImageReference, string) ([]registryTag, error) {
		return nil, wantErr
	})
	g.Expect(err).To(MatchError(wantErr))
}

func TestIsUnknownRepositoryError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "registry reports missing repository",
			err:  fmt.Errorf("fetching tags list: %w", errcode.Errors{registryv2.ErrorCodeNameUnknown.WithDetail("repo")}),
			want: true,
		},
		{
			name: "registry reports a different error",
			err:  fmt.Errorf("fetching tags list: %w", errcode.Errors{registryv2.ErrorCodeManifestUnknown.WithDetail("tag")}),
		},
		{name: "non-registry error", err: errors.New("connection refused")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isUnknownRepositoryError(tt.err)).To(Equal(tt.want))
		})
	}
}

func TestMatchingMigrationTagsSortsNewestFirst(t *testing.T) {
	g := NewWithT(t)
	const prefix = "migration-1.0-sha256-"
	tags := matchingMigrationTags([]string{
		prefix + "20",
		"migration-1x0-sha256-99",
		prefix + "not-a-timestamp",
		prefix + "100",
		prefix + "3",
	}, prefix)

	g.Expect(tags).To(HaveLen(3))
	g.Expect([]string{tags[0].Name, tags[1].Name, tags[2].Name}).To(Equal([]string{
		prefix + "100",
		prefix + "20",
		prefix + "3",
	}))
}

func writeMigrationScript(g *WithT, taskDir, version string, content []byte) {
	migrationsDir := filepath.Join(taskDir, "migrations")
	g.Expect(os.MkdirAll(migrationsDir, 0o755)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(migrationsDir, version+".sh"), content, 0o600)).To(Succeed())
}
