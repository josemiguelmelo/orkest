package executionprofile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/distribution/reference"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
)

var ErrNotConfigured = errors.New("execution image registry is not configured")

type ExecutionProfileService struct {
	profileRepository ExecutionProfileRepository
	access            WorkspaceAccess
	imageRepository   string
	images            map[coreruntime.Runtime]string
}

func NewExecutionProfileService(profileRepository ExecutionProfileRepository, access WorkspaceAccess, imageRepository string, images map[coreruntime.Runtime]string) *ExecutionProfileService {
	return &ExecutionProfileService{profileRepository: profileRepository, access: access, imageRepository: imageRepository, images: images}
}

func (s *ExecutionProfileService) List(ctx context.Context, user, org, ws string) ([]Profile, error) {
	if _, err := s.access.GetWorkspace(ctx, user, org, ws); err != nil {
		return nil, err
	}
	return s.profileRepository.List(ctx, ws)
}

func (s *ExecutionProfileService) Builds(ctx context.Context, user, org, ws, id string) ([]Build, error) {
	if _, err := s.access.GetWorkspace(ctx, user, org, ws); err != nil {
		return nil, err
	}
	if _, err := s.profileRepository.Get(ctx, ws, id); err != nil {
		return nil, err
	}
	return s.profileRepository.Builds(ctx, ws, id)
}

// Save snapshots build inputs and enqueues atomically; generation is optimistic concurrency control.
func (s *ExecutionProfileService) Save(ctx context.Context, user, org, ws, id string, generation int64, input Input) (Profile, error) {
	w, err := s.access.GetWorkspace(ctx, user, org, ws)
	if err != nil {
		return Profile{}, err
	}
	if w.Role != "admin" && w.Role != "owner" {
		return Profile{}, modelerrors.ErrForbidden
	}
	input.Name = strings.TrimSpace(input.Name)
	input.ToolchainImage = strings.TrimSpace(input.ToolchainImage)
	if input.Name == "" || len(input.Name) > 200 || len(input.Description) > 2000 || !input.Runtime.Available() {
		return Profile{}, modelerrors.ErrInvalid
	}
	if _, err = reference.ParseNormalizedNamed(input.ToolchainImage); err != nil {
		return Profile{}, modelerrors.ErrInvalid
	}
	if s.imageRepository == "" {
		return Profile{}, ErrNotConfigured
	}
	target, parseErr := reference.ParseNormalizedNamed(s.imageRepository)
	if parseErr != nil || !reference.IsNameOnly(target) {
		return Profile{}, ErrNotConfigured
	}

	create := id == ""
	if create {
		id, err = ids.NewUUID()
		if err != nil {
			return Profile{}, err
		}
	} else {
		previous, getErr := s.profileRepository.Get(ctx, ws, id)
		if getErr != nil {
			return Profile{}, getErr
		}
		if previous.Runtime != input.Runtime {
			return Profile{}, fmt.Errorf("%w: create another profile to change runtime", modelerrors.ErrInvalid)
		}
	}
	base := s.images[input.Runtime]
	if base == "" {
		provider, _ := coreruntime.ProviderFor(input.Runtime)
		base = provider.DefaultImage()
	}
	buildID, err := ids.NewUUID()
	if err != nil {
		return Profile{}, err
	}
	p := Profile{ID: id, WorkspaceID: ws, Input: input, Generation: generation}
	provider, _ := coreruntime.ProviderFor(input.Runtime)
	b := Build{RuntimeVersion: provider.Version(), ID: buildID, ProfileID: id, WorkspaceID: ws, Runtime: input.Runtime, ToolchainImage: input.ToolchainImage, BaseImage: base, Destination: s.imageRepository + ":build-" + buildID}
	return s.profileRepository.Save(ctx, p, b, create)
}

// Resolve is internal: callers have already authorized the workspace operation.
func (s *ExecutionProfileService) Resolve(ctx context.Context, ws, id string, runtime coreruntime.Runtime) (string, error) {
	p, err := s.profileRepository.Get(ctx, ws, id)
	if err != nil {
		return "", err
	}
	if p.Runtime != runtime || p.ExecutionImage == "" {
		return "", fmt.Errorf("%w: profile image is not ready or runtime is incompatible", modelerrors.ErrConflict)
	}
	return p.ExecutionImage, nil
}

func (s *ExecutionProfileService) ValidateProfile(ctx context.Context, ws, id string, runtime coreruntime.Runtime) error {
	p, err := s.profileRepository.Get(ctx, ws, id)
	if err != nil {
		return err
	}
	if p.Runtime != runtime {
		return modelerrors.ErrInvalid
	}
	return nil
}
