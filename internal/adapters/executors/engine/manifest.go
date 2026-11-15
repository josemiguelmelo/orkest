package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RunRecordedStart owns the manifest state transitions shared by every
// provider. The callback contains only provider resource operations.
func RunRecordedStart(ctx context.Context, store *ManifestStore, plan WorkloadPlan, provider string, start func(context.Context, WorkloadPlan) (ResourceGroup, error)) (result ResourceGroup, resultErr error) {
	if err := store.Record(ctx, plan, "starting", nil); err != nil {
		return ResourceGroup{}, err
	}
	defer func() {
		providerIDs := make(map[string]string, len(result.Components))
		for _, component := range result.Components {
			providerIDs[component.Ref.Key.ResourceID+"/"+component.Ref.Key.ComponentID] = component.Ref.ConcreteID
		}
		state := "uncertain"
		if resultErr == nil {
			state = "active"
		}
		if err := store.Record(context.WithoutCancel(ctx), plan, state, providerIDs); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("persist %s operation result: %w", provider, err))
		}
	}()
	return start(ctx, plan)
}

// RunRecordedLifecycle applies a provider operation and persists its resulting
// state using the same cancellation-safe policy for every provider.
func RunRecordedLifecycle(ctx context.Context, store *ManifestStore, identity GroupIdentity, state, provider string, operation func() error) (resultErr error) {
	defer func() {
		if resultErr == nil {
			if err := store.Update(context.WithoutCancel(ctx), identity, state, nil); err != nil {
				resultErr = fmt.Errorf("persist %s %s state: %w", provider, state, err)
			}
		}
	}()
	return operation()
}

// OperationManifest is a secret-free durable record used to reconcile an
// infrastructure operation after the coordinator or provider client restarts.
// The fingerprint hashes the complete plan, but no plan values are persisted.
type OperationManifest struct {
	Version     int               `json:"version"`
	Identity    GroupIdentity     `json:"identity"`
	Fingerprint string            `json:"fingerprint"`
	State       string            `json:"state"`
	Components  []string          `json:"components"`
	Networks    []string          `json:"networks,omitempty"`
	Storage     []string          `json:"storage,omitempty"`
	ProviderIDs map[string]string `json:"provider_ids,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ManifestStore atomically records operation identity before provider create
// calls. A store instance serializes writers; Orkest's single-coordinator lease
// fences independent processes.
type ManifestStore struct {
	root string
	mu   sync.Mutex
}

func NewManifestStore(root string) (*ManifestStore, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("operation manifest root must be absolute")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create operation manifest directory: %w", err)
	}
	return &ManifestStore{root: root}, nil
}

func (s *ManifestStore) Record(ctx context.Context, plan WorkloadPlan, state string, providerIDs map[string]string) error {
	if s == nil {
		return fmt.Errorf("operation manifest store is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateWorkloadPlan(plan); err != nil {
		return err
	}
	serialized, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("fingerprint infrastructure plan: %w", err)
	}
	fingerprint := sha256.Sum256(serialized)
	manifest := OperationManifest{Version: 1, Identity: plan.Group, Fingerprint: hex.EncodeToString(fingerprint[:]), State: state, UpdatedAt: time.Now().UTC()}
	manifest.Components = make([]string, 0, len(plan.Workloads))
	for _, workload := range plan.Workloads {
		manifest.Components = append(manifest.Components, workload.ID)
	}
	manifest.Networks = make([]string, 0, len(plan.Networks))
	for _, network := range plan.Networks {
		manifest.Networks = append(manifest.Networks, network.ID)
	}
	manifest.Storage = make([]string, 0, len(plan.Storage))
	for _, attachment := range plan.Storage {
		manifest.Storage = append(manifest.Storage, attachment.WorkloadID+"/"+attachment.Binding+"/"+attachment.StorageID+"/"+attachment.Subpath)
	}
	if len(providerIDs) > 0 {
		manifest.ProviderIDs = make(map[string]string, len(providerIDs))
		for key, value := range providerIDs {
			manifest.ProviderIDs[key] = value
		}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode operation manifest: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.write(plan.Group, data)
}

// Read returns the last durable operation record for a group identity.
func (s *ManifestStore) Read(ctx context.Context, identity GroupIdentity) (OperationManifest, error) {
	if s == nil {
		return OperationManifest{}, fmt.Errorf("operation manifest store is required")
	}
	if err := ctx.Err(); err != nil {
		return OperationManifest{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(identity))
	if err != nil {
		return OperationManifest{}, err
	}
	var manifest OperationManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return OperationManifest{}, fmt.Errorf("decode operation manifest: %w", err)
	}
	if manifest.Version != 1 || manifest.Identity != identity || manifest.Fingerprint == "" {
		return OperationManifest{}, fmt.Errorf("invalid operation manifest identity or version")
	}
	return manifest, nil
}

// Update changes lifecycle state and provider handles without needing the
// original plan, which is deliberately not persisted.
func (s *ManifestStore) Update(ctx context.Context, identity GroupIdentity, state string, providerIDs map[string]string) error {
	if s == nil {
		return fmt.Errorf("operation manifest store is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(identity))
	if err != nil {
		return err
	}
	var manifest OperationManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode operation manifest: %w", err)
	}
	if manifest.Version != 1 || manifest.Identity != identity || manifest.Fingerprint == "" {
		return fmt.Errorf("invalid operation manifest identity or version")
	}
	manifest.State = state
	manifest.UpdatedAt = time.Now().UTC()
	if providerIDs != nil {
		manifest.ProviderIDs = providerIDs
	}
	data, err = json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode operation manifest: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return s.write(identity, data)
}

func (s *ManifestStore) path(identity GroupIdentity) string {
	data, _ := json.Marshal(identity)
	key := sha256.Sum256(data)
	return filepath.Join(s.root, hex.EncodeToString(key[:])+".json")
}

func (s *ManifestStore) write(identity GroupIdentity, data []byte) error {
	path := s.path(identity)
	temporary, err := os.CreateTemp(s.root, ".operation-*.tmp")
	if err != nil {
		return fmt.Errorf("create operation manifest temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write operation manifest: %w", err)
	}
	if err = os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("publish operation manifest: %w", err)
	}
	directory, err := os.Open(s.root)
	if err == nil {
		err = directory.Sync()
		_ = directory.Close()
	}
	if err != nil {
		return fmt.Errorf("sync operation manifest directory: %w", err)
	}
	return nil
}
