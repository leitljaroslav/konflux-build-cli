package tknbundle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/distribution/distribution/v3/registry/api/errcode"
	registryv2 "github.com/distribution/distribution/v3/registry/api/v2"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/types"
)

type migrationImage struct {
	Reference string
	FilePath  string
	Tag       string
	Digest    string
}

type registryTag struct {
	Name      string
	Digest    string
	timestamp int64
}

type migrationTagSearch func(context.Context, types.ImageReference, string) ([]registryTag, error)

// resolveMigrationImage finds an existing image for the migration script. If the image is
// not present, it returns a tag to use when publishing it; Digest remains empty.
// A missing migration script is not an error and returns nil.
func resolveMigrationImage(bundle preparedBundle, image string) (*migrationImage, error) {
	return resolveMigrationImageWithSearch(bundle, image, searchAvailableTags)
}

func resolveMigrationImageWithSearch(bundle preparedBundle, image string, search migrationTagSearch) (*migrationImage, error) {
	if bundle.Version == "" {
		return nil, nil
	}

	migrationFilePath := filepath.Join(bundle.TaskDir, "migrations", bundle.Version+".sh")
	migrationFilePath = filepath.Clean(migrationFilePath)
	migrationFile, err := os.Open(migrationFilePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening migration file %q: %w", migrationFilePath, err)
	}
	defer func() {
		err := migrationFile.Close()
		if err != nil {
			l.Logger.WithError(err).Error("error closing a file")
		}
	}()

	migrationFileInfo, err := migrationFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("checking migration file %q: %w", migrationFilePath, err)
	}
	if !migrationFileInfo.Mode().IsRegular() {
		return nil, errors.New("migration file is not a regular file")
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, migrationFile); err != nil {
		return nil, fmt.Errorf("hashing migration file %q: %w", migrationFilePath, err)
	}

	checksum := fmt.Sprintf("%x", hash.Sum(nil))
	tagPrefix := fmt.Sprintf("migration-%s-%s-", bundle.Version, checksum)
	imageRef, err := docker.ParseReference("//" + image)
	if err != nil {
		return nil, fmt.Errorf("parsing image reference %q: %w", image, err)
	}

	availableTags, err := search(context.Background(), imageRef, tagPrefix)
	if err != nil {
		return nil, err
	}
	if len(availableTags) > 0 {
		return &migrationImage{
			Reference: reference.TrimNamed(imageRef.DockerReference()).Name() + ":" + availableTags[0].Name,
			FilePath:  migrationFilePath,
			Tag:       availableTags[0].Name,
			Digest:    availableTags[0].Digest,
		}, nil
	}

	tag := fmt.Sprintf("%s%d", tagPrefix, time.Now().UTC().Unix())
	return &migrationImage{
		Reference: reference.TrimNamed(imageRef.DockerReference()).Name() + ":" + tag,
		FilePath:  migrationFilePath,
		Tag:       tag,
	}, nil
}

// searchAvailableTags uses the registry's standard Docker Registry v2 tag and
// manifest endpoints through go.podman.io/image. It therefore follows the
// registry configuration and authentication used by the rest of the CLI.
func searchAvailableTags(ctx context.Context, imageRef types.ImageReference, tagPrefix string) ([]registryTag, error) {
	availableTags, err := docker.GetRepositoryTags(ctx, nil, imageRef)
	if err != nil {
		if isUnknownRepositoryError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing tags for image repository %q: %w", imageRef.DockerReference().Name(), err)
	}

	repository := reference.TrimNamed(imageRef.DockerReference()).Name()
	tags := matchingMigrationTags(availableTags, tagPrefix)

	for i := range tags {
		tagRef, err := docker.ParseReference("//" + repository + ":" + tags[i].Name)
		if err != nil {
			return nil, fmt.Errorf("parsing migration image tag %q: %w", tags[i].Name, err)
		}

		source, err := tagRef.NewImageSource(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("opening migration image %q: %w", tagRef.StringWithinTransport(), err)
		}
		manifest, _, manifestErr := source.GetManifest(ctx, nil)
		closeErr := source.Close()
		if manifestErr != nil {
			return nil, fmt.Errorf("reading migration image manifest %q: %w", tagRef.StringWithinTransport(), manifestErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("closing migration image %q: %w", tagRef.StringWithinTransport(), closeErr)
		}

		tags[i].Digest = digest.FromBytes(manifest).String()
	}

	return tags, nil
}

// isUnknownRepositoryError recognizes the OCI Distribution API response used
// when a repository has not been created yet. A missing repository has no
// migration tags, so the caller should publish the first migration image.
func isUnknownRepositoryError(err error) bool {
	if registryErrors, ok := errors.AsType[errcode.Errors](err); ok {
		for _, registryErr := range registryErrors {
			if errorCoder, ok := registryErr.(errcode.ErrorCoder); ok && errorCoder.ErrorCode() == registryv2.ErrorCodeNameUnknown {
				return true
			}
		}
	}

	var errorCoder errcode.ErrorCoder
	if errors.As(err, &errorCoder) && errorCoder.ErrorCode() == registryv2.ErrorCodeNameUnknown {
		return true
	}

	// go.podman.io/image decodes registry errors with an older Distribution
	// package. Decode their shared JSON representation with distribution/v3.
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		data, marshalErr := json.Marshal(cause)
		if marshalErr != nil {
			continue
		}
		var registryErr errcode.Error
		if json.Unmarshal(data, &registryErr) == nil && registryErr.Code == registryv2.ErrorCodeNameUnknown {
			return true
		}
	}
	return false
}

func matchingMigrationTags(availableTags []string, tagPrefix string) []registryTag {
	tags := make([]registryTag, 0)
	for _, tag := range availableTags {
		if !strings.HasPrefix(tag, tagPrefix) {
			continue
		}

		timestampText := strings.TrimPrefix(tag, tagPrefix)
		timestamp, err := strconv.ParseInt(timestampText, 10, 64)
		if err != nil {
			continue
		}
		tags = append(tags, registryTag{Name: tag, timestamp: timestamp})
	}

	// The timestamp is Unix seconds, so descending numeric order selects the
	// newest matching migration tag regardless of the registry's list order.
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].timestamp > tags[j].timestamp
	})
	return tags
}
