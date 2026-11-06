// Package conformance defines observable runtime behavior shared by all adapters.
package conformance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
)

type Factory func(*testing.T, string, string) agentruntime.RuntimeHost

// Probe checks the observable effect of one runtime configuration setting.
type Probe struct{ Prompt, Expected string }

// Profile contains protocol-independent prompts. A live profile must provoke a
// real permission and a long-running response; completing early fails the suite.
type Profile struct {
	Config                            configmodel.RuntimeConfig
	Remember                          func(string) string
	Recall, Permission, Wait, Suspend string
	Configuration                     map[string]Probe
}

func Run(t *testing.T, factory Factory, profile Profile) {
	t.Helper()
	if factory == nil || profile.Remember == nil || profile.Recall == "" || profile.Permission == "" || profile.Wait == "" || profile.Suspend == "" {
		t.Fatal("incomplete conformance profile")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	work, state := t.TempDir(), t.TempDir()
	host := factory(t, work, state)
	config := profile.Config
	if host.Runtime() != config.Runtime || host.Version() == "" {
		t.Fatal("runtime identity is incomplete")
	}
	first, err := host.Launch(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	if first.SessionID() == "" {
		t.Fatal("empty session identity")
	}
	if _, err = host.Launch(ctx, config); err == nil {
		t.Fatal("duplicate execution was allowed")
	}
	for _, name := range []string{"instructions", "skills", "http-mcp"} {
		probe, ok := profile.Configuration[name]
		if !ok || probe.Prompt == "" || probe.Expected == "" {
			t.Fatalf("missing configuration probe: %s", name)
		}
	}
	var output strings.Builder
	observer := agentruntime.PromptObserver{Update: func(update interaction.Update) error { output.WriteString(update.Text); return nil }, Permission: func(context.Context, interaction.Permission) (string, error) {
		return "", errors.New("unexpected permission")
	}}
	token, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	token = "orkest-context-" + token
	verifyStreaming(t, ctx, first, profile.Remember(token), observer, &output)
	verifyConfiguration(t, ctx, first, profile.Configuration, observer, &output)
	verifyPermission(t, ctx, first, profile.Permission, observer)
	verifyCancellation(t, ctx, first, profile.Wait, observer)
	id := first.SessionID()
	path := filepath.Join(work, "untracked.txt")
	suspended := interruptSession(t, ctx, first, profile.Suspend, path, observer)
	config.RunID = "revision-2"
	restored, err := host.Restore(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if restored.SessionID() != id {
		t.Fatal("restore created a new native session")
	}
	decision, err := restored.Reconcile(ctx, suspended)
	if err != nil || decision.Action != agentruntime.RecoveryPause {
		t.Fatalf("unqualified history evidence was not paused: %+v %v", decision, err)
	}
	wrong := suspended
	wrong.ID = "different-command"
	decision, err = restored.Reconcile(ctx, wrong)
	if err != nil || decision.Action != agentruntime.RecoveryPause {
		t.Fatal("identical text with a different command identity was continued")
	}
	output.Reset()
	if _, err = restored.Prompt(ctx, interaction.Command{ID: "recall", Text: profile.Recall}, observer); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), token) {
		t.Fatal("restored session lost conversation context")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "preserved" {
		t.Fatal("working files were lost")
	}
	decision, err = restored.Reconcile(ctx, interaction.Command{ID: "unknown", Text: "unknown"})
	if err != nil || decision.Action != agentruntime.RecoveryPause {
		t.Fatalf("uncertain command was not paused: %+v %v", decision, err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err = restored.Prompt(cancelled, interaction.Command{ID: "cancelled-context", Text: "unused"}, observer); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context ignored: %v", err)
	}
	if err = restored.Close(); err != nil {
		t.Fatal(err)
	}
	config.Agent.Instructions += " changed"
	if _, err = host.Restore(ctx, config); err == nil {
		t.Fatal("incompatible snapshot was restored")
	}
	config.TaskSessionID = "missing-session"
	if _, err = host.Restore(ctx, config); err == nil {
		t.Fatal("missing snapshot fell back to launch")
	}
}

func verifyStreaming(t *testing.T, ctx context.Context, session agentruntime.RuntimeSession, prompt string, observer agentruntime.PromptObserver, output *strings.Builder) {
	t.Helper()
	result, err := session.Prompt(ctx, interaction.Command{ID: "remember", Text: prompt}, observer)
	if err != nil || result == agentruntime.StopReasonCancelled || output.Len() == 0 {
		t.Fatalf("streaming prompt failed: %+v %v", result, err)
	}
}

func verifyConfiguration(t *testing.T, ctx context.Context, session agentruntime.RuntimeSession, probes map[string]Probe, observer agentruntime.PromptObserver, output *strings.Builder) {
	t.Helper()
	for name, probe := range probes {
		output.Reset()
		if _, err := session.Prompt(ctx, interaction.Command{ID: "configuration-" + name, Text: probe.Prompt}, observer); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), probe.Expected) {
			t.Fatalf("configuration probe failed: %s", name)
		}
	}
}

func verifyPermission(t *testing.T, ctx context.Context, session agentruntime.RuntimeSession, prompt string, observer agentruntime.PromptObserver) {
	t.Helper()
	approvals := 0
	permissionObserver := observer
	permissionObserver.Permission = func(_ context.Context, p interaction.Permission) (string, error) {
		approvals++
		if len(p.Options) == 0 {
			return "", errors.New("permission has no choices")
		}
		for _, option := range p.Options {
			if option.Kind == "reject_once" {
				return option.ID, nil
			}
		}
		return "", errors.New("permission must offer rejection")
	}
	if _, err := session.Prompt(ctx, interaction.Command{ID: "approval", Text: prompt}, permissionObserver); err != nil {
		t.Fatal(err)
	}
	if approvals == 0 {
		t.Fatal("approval was not forwarded")
	}
}

func verifyCancellation(t *testing.T, ctx context.Context, session agentruntime.RuntimeSession, prompt string, observer agentruntime.PromptObserver) {
	t.Helper()
	started := make(chan struct{}, 1)
	waiting := observer
	waiting.Update = func(interaction.Update) error {
		select {
		case started <- struct{}{}:
		default:
		}
		return nil
	}
	finished := make(chan error, 1)
	go func() {
		result, err := session.Prompt(ctx, interaction.Command{ID: "cancel", Text: prompt}, waiting)
		if err == nil && result != agentruntime.StopReasonCancelled {
			err = errors.New("runtime did not report cancellation")
		}
		finished <- err
	}()
	select {
	case <-started:
	case err := <-finished:
		t.Fatalf("prompt ended before cancellation: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := session.Cancel(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func interruptSession(t *testing.T, ctx context.Context, session agentruntime.RuntimeSession, prompt, path string, observer agentruntime.PromptObserver) interaction.Command {
	t.Helper()
	started := make(chan struct{}, 1)
	finished := make(chan error, 1)
	waiting := observer
	waiting.Update = func(interaction.Update) error {
		select {
		case started <- struct{}{}:
		default:
		}
		return nil
	}
	// Interrupt a separate tool-free turn without issuing user cancellation.
	suspended := interaction.Command{ID: "interrupted", Text: prompt}
	go func() { _, err := session.Prompt(ctx, suspended, waiting); finished <- err }()
	select {
	case <-started:
	case err := <-finished:
		t.Fatalf("recovery prompt ended before interruption: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := os.WriteFile(path, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if !agentruntime.IsRuntimeInterrupted(err) {
			t.Fatalf("interruption was not typed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-session.Done():
	default:
		t.Fatal("close did not confirm termination")
	}
	return suspended
}
