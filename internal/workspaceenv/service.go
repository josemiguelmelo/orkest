package workspaceenv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
)

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Service struct {
	repository Repository
	workspaces WorkspaceAccess
	box        *secretbox.Box
}

func NewService(repository Repository, workspaces WorkspaceAccess, box *secretbox.Box) *Service {
	return &Service{repository: repository, workspaces: workspaces, box: box}
}

func (s *Service) List(ctx context.Context, userID, orgID, workspaceID string) ([]Value, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	stored, err := s.repository.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	values := make([]Value, 0, len(stored))
	for _, item := range stored {
		value := Value{Name: item.Name, Secret: item.Encrypted}
		if !item.Encrypted {
			value.Value = item.Value
		}
		values = append(values, value)
	}
	return values, nil
}

func (s *Service) Put(ctx context.Context, userID, orgID, workspaceID, name string, input Input) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	if !validName(name) || strings.ContainsRune(input.Value, '\x00') || len(input.Value) > 65536 {
		return modelerrors.ErrInvalid
	}
	value := input.Value
	if input.Secret {
		if s.box == nil {
			return secretbox.ErrKeyUnavailable
		}
		sealed, err := s.box.Seal(value)
		if err != nil {
			return fmt.Errorf("encrypt workspace secret: %w", err)
		}
		value = string(sealed)
	}
	return s.repository.Put(ctx, workspaceID, name, value, input.Secret)
}

func (s *Service) Delete(ctx context.Context, userID, orgID, workspaceID, name string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	if !validName(name) {
		return modelerrors.ErrInvalid
	}
	return s.repository.Delete(ctx, workspaceID, name)
}

func (s *Service) Resolve(ctx context.Context, workspaceID string) (RuntimeValues, error) {
	stored, err := s.repository.Resolve(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	values := make(RuntimeValues, len(stored))
	for _, item := range stored {
		value := item.Value
		if item.Encrypted {
			if s.box == nil {
				return nil, secretbox.ErrKeyUnavailable
			}
			value, err = s.box.Open([]byte(value))
			if err != nil {
				return nil, errors.New("decrypt workspace environment value")
			}
		}
		values[item.Name] = value
	}
	return values, nil
}

func (s *Service) SealSnapshot(values RuntimeValues) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	if s.box == nil {
		return "", secretbox.ErrKeyUnavailable
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	sealed, err := s.box.Seal(string(encoded))
	if err != nil {
		return "", fmt.Errorf("encrypt workspace environment snapshot: %w", err)
	}
	return string(sealed), nil
}

func (s *Service) OpenSnapshot(encrypted string) (RuntimeValues, error) {
	if encrypted == "" {
		return nil, nil
	}
	if s.box == nil {
		return nil, secretbox.ErrKeyUnavailable
	}
	plain, err := s.box.Open([]byte(encrypted))
	if err != nil {
		return nil, errors.New("decrypt workspace environment snapshot")
	}
	var values RuntimeValues
	if err := json.Unmarshal([]byte(plain), &values); err != nil {
		return nil, errors.New("decode workspace environment snapshot")
	}
	return values, nil
}

func (s *Service) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role != "admin" && w.Role != "owner" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func validName(name string) bool {
	return namePattern.MatchString(name) && name != "HOME" && !strings.HasPrefix(name, "ORKEST_") && !strings.HasPrefix(name, "OPENCODE_") && !strings.HasPrefix(name, "XDG_")
}
