package taskeditor

import "context"

// UnavailableEditorExecutor keeps unsupported engine configurations explicit without
// changing their existing agent execution behavior.
type UnavailableEditorExecutor struct{}

func (UnavailableEditorExecutor) StartEditor(context.Context, Spec) (Instance, error) {
	return Instance{}, ErrUnavailable
}

func (UnavailableEditorExecutor) InspectEditor(context.Context, Scope) (Instance, bool, error) {
	return Instance{}, false, ErrUnavailable
}

func (UnavailableEditorExecutor) StopEditor(context.Context, Instance) error { return ErrUnavailable }

func (UnavailableEditorExecutor) ListEditors(context.Context) ([]Instance, error) { return nil, nil }

func (UnavailableEditorExecutor) Inspect(context.Context, string, string, string, []Checkout) (Inspection, error) {
	return Inspection{}, ErrUnavailable
}
