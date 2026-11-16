package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	identitymodel "github.com/josemiguelmelo/orkest/internal/server/identity/model"
	"github.com/josemiguelmelo/orkest/internal/server/organization"
	"golang.org/x/crypto/argon2"
)

const (
	localPasswordMemoryKiB = 64 * 1024
	localPasswordTime      = 3
	localPasswordThreads   = 4
	localPasswordKeyLength = 32
)

var (
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrNoLocalPassword       = errors.New("account has no local password")
	ErrNoLocalAccountToLink  = errors.New("no local account matches verified email")
	ErrAmbiguousLocalAccount = errors.New("multiple local accounts match verified email")
)

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

type LocalCredential struct {
	User         identitymodel.User
	PasswordHash string
}

type LocalAccountRepository interface {
	ProvisionLocalAccount(context.Context, string, string, string, string, string, string, string, string) (identitymodel.User, error)
	BootstrapLocalAdminRequired(context.Context, string) (bool, error)
	BootstrapLocalAdmin(context.Context, string, string, string, string, string, string) (identitymodel.User, bool, error)
	LocalCredentialByUsername(context.Context, string) (LocalCredential, error)
	LocalCredentialByUserID(context.Context, string) (LocalCredential, error)
	UpdateLocalPassword(context.Context, string, string, string) error
	HasLocalPassword(context.Context, string) (bool, error)
	CreateLocalSession(context.Context, []byte, string, time.Time) error
	UserForLocalSession(context.Context, []byte) (identitymodel.User, error)
	RevokeLocalSession(context.Context, []byte) error
	LinkKeycloakIdentityByEmail(context.Context, string, string) (identitymodel.User, error)
	HasLocalAccountEmail(context.Context, string) (bool, error)
}

type LocalAuthenticationService struct {
	repository    LocalAccountRepository
	users         *UserRegistrationService
	orgs          *organization.OrganizationService
	passwordSlots chan struct{}
	now           func() time.Time
}

func NewLocalAuthenticationService(repository LocalAccountRepository, users *UserRegistrationService, orgs *organization.OrganizationService) *LocalAuthenticationService {
	return &LocalAuthenticationService{repository: repository, users: users, orgs: orgs, passwordSlots: make(chan struct{}, 4), now: time.Now}
}

func (s *LocalAuthenticationService) Provision(ctx context.Context, actorID, orgID, username, email, password, role string) (identitymodel.User, error) {
	if actorID == "" || orgID == "" || len(password) < 12 || len(password) > 1024 {
		return identitymodel.User{}, modelerrors.ErrInvalid
	}
	username = strings.TrimSpace(username)
	normalized := strings.ToLower(username)
	email = strings.TrimSpace(email)
	parsedEmail, emailErr := mail.ParseAddress(email)
	if username == "" || len(username) > 64 || strings.ContainsAny(username, " \t\r\n") || emailErr != nil || parsedEmail.Address != email || len(email) > 320 {
		return identitymodel.User{}, modelerrors.ErrInvalid
	}
	if role != "member" && role != "admin" && role != "owner" {
		return identitymodel.User{}, modelerrors.ErrInvalid
	}
	platformAdmin, err := s.users.IsPlatformAdmin(ctx, actorID)
	if err != nil {
		return identitymodel.User{}, err
	}
	if !platformAdmin {
		actorRole, e := s.orgs.RoleForUser(ctx, actorID, orgID)
		if e != nil {
			return identitymodel.User{}, e
		}
		if actorRole != "owner" && actorRole != "admin" || role == "owner" && actorRole != "owner" {
			return identitymodel.User{}, modelerrors.ErrForbidden
		}
	}
	if err := s.acquirePasswordSlot(ctx); err != nil {
		return identitymodel.User{}, err
	}
	hash, err := hashPassword(password)
	<-s.passwordSlots
	if err != nil {
		return identitymodel.User{}, fmt.Errorf("hash local password: %w", err)
	}
	id, err := ids.NewUUID()
	if err != nil {
		return identitymodel.User{}, fmt.Errorf("generate user ID: %w", err)
	}
	return s.repository.ProvisionLocalAccount(ctx, id, orgID, username, normalized, email, strings.ToLower(email), hash, role)
}

func (s *LocalAuthenticationService) BootstrapPlatformAdmin(ctx context.Context, username, email, password string) (bool, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	parsedEmail, emailErr := mail.ParseAddress(email)
	if username == "" || len(username) > 64 || strings.ContainsAny(username, " \t\r\n") || emailErr != nil || parsedEmail.Address != email || len(email) > 320 || password == "" || len(password) > 1024 {
		return false, modelerrors.ErrInvalid
	}
	normalizedUsername := strings.ToLower(username)
	required, err := s.repository.BootstrapLocalAdminRequired(ctx, normalizedUsername)
	if err != nil || !required {
		return false, err
	}
	if err := s.acquirePasswordSlot(ctx); err != nil {
		return false, err
	}
	hash, err := hashPassword(password)
	<-s.passwordSlots
	if err != nil {
		return false, fmt.Errorf("hash bootstrap password: %w", err)
	}
	id, err := ids.NewUUID()
	if err != nil {
		return false, fmt.Errorf("generate bootstrap user ID: %w", err)
	}
	_, created, err := s.repository.BootstrapLocalAdmin(ctx, id, username, normalizedUsername, email, strings.ToLower(email), hash)
	return created, err
}

func (s *LocalAuthenticationService) Authenticate(ctx context.Context, username, password string) (identitymodel.User, string, error) {
	if len(username) > 64 || len(password) > 1024 {
		return identitymodel.User{}, "", ErrInvalidCredentials
	}
	credential, err := s.repository.LocalCredentialByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err := s.acquirePasswordSlot(ctx); err != nil {
		return identitymodel.User{}, "", err
	}
	defer func() { <-s.passwordSlots }()
	if err != nil {
		if !errors.Is(err, ErrInvalidCredentials) {
			return identitymodel.User{}, "", err
		}
		dummyHashOnce.Do(func() {
			salt := []byte("orkest-auth-salt")
			derived := argon2.IDKey([]byte("not-a-real-password"), salt, localPasswordTime, localPasswordMemoryKiB, localPasswordThreads, localPasswordKeyLength)
			dummyHash = fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", localPasswordMemoryKiB, localPasswordTime, localPasswordThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived))
		})
		_ = verifyPassword(dummyHash, password)
		return identitymodel.User{}, "", ErrInvalidCredentials
	}
	if !verifyPassword(credential.PasswordHash, password) {
		return identitymodel.User{}, "", ErrInvalidCredentials
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return identitymodel.User{}, "", fmt.Errorf("generate local session: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	expires := s.now().Add(12 * time.Hour)
	if err := s.repository.CreateLocalSession(ctx, hashSession(token), credential.User.ID, expires); err != nil {
		return identitymodel.User{}, "", fmt.Errorf("persist local session: %w", err)
	}
	return credential.User, "local." + token, nil
}

func (s *LocalAuthenticationService) acquirePasswordSlot(ctx context.Context) error {
	select {
	case s.passwordSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *LocalAuthenticationService) UserForSession(ctx context.Context, token string) (identitymodel.User, error) {
	if !strings.HasPrefix(token, "local.") {
		return identitymodel.User{}, ErrInvalidCredentials
	}
	return s.repository.UserForLocalSession(ctx, hashSession(strings.TrimPrefix(token, "local.")))
}

func (s *LocalAuthenticationService) RevokeSession(ctx context.Context, token string) error {
	if !strings.HasPrefix(token, "local.") {
		return nil
	}
	return s.repository.RevokeLocalSession(ctx, hashSession(strings.TrimPrefix(token, "local.")))
}

func (s *LocalAuthenticationService) LinkKeycloakIdentity(ctx context.Context, subject, email string, emailVerified bool) (identitymodel.User, error) {
	if !emailVerified || strings.TrimSpace(subject) == "" || strings.TrimSpace(email) == "" {
		return identitymodel.User{}, ErrInvalidCredentials
	}
	return s.repository.LinkKeycloakIdentityByEmail(ctx, subject, email)
}

func (s *LocalAuthenticationService) HasAccountEmail(ctx context.Context, email string) (bool, error) {
	return s.repository.HasLocalAccountEmail(ctx, email)
}

func (s *LocalAuthenticationService) HasLocalPassword(ctx context.Context, userID string) (bool, error) {
	return s.repository.HasLocalPassword(ctx, userID)
}

func (s *LocalAuthenticationService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if userID == "" || len(currentPassword) == 0 || len(currentPassword) > 1024 ||
		len(newPassword) < 12 || len(newPassword) > 1024 || currentPassword == newPassword {
		return modelerrors.ErrInvalid
	}
	credential, err := s.repository.LocalCredentialByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.acquirePasswordSlot(ctx); err != nil {
		return err
	}
	defer func() { <-s.passwordSlots }()
	if !verifyPassword(credential.PasswordHash, currentPassword) {
		return ErrInvalidCredentials
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash replacement password: %w", err)
	}
	return s.repository.UpdateLocalPassword(ctx, userID, credential.PasswordHash, hash)
}

func hashSession(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived := argon2.IDKey([]byte(password), salt, localPasswordTime, localPasswordMemoryKiB, localPasswordThreads, localPasswordKeyLength)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", localPasswordMemoryKiB, localPasswordTime, localPasswordThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" || parts[2] != fmt.Sprintf("m=%d,t=%d,p=%d", localPasswordMemoryKiB, localPasswordTime, localPasswordThreads) {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) != 16 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(want) != localPasswordKeyLength {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, localPasswordTime, localPasswordMemoryKiB, localPasswordThreads, uint32(localPasswordKeyLength))
	return subtle.ConstantTimeCompare(got, want) == 1
}
