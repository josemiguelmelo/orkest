package taskeditor

import "context"

// UnavailableBackend keeps unsupported executor configurations explicit without
// changing their existing agent execution behavior.
type UnavailableBackend struct{}

func (UnavailableBackend) StartEditor(context.Context, Spec) (Instance, error) {
	return Instance{}, ErrUnavailable
}

func (UnavailableBackend) InspectEditor(context.Context, Scope) (Instance, bool, error) {
	return Instance{}, false, ErrUnavailable
}
func (UnavailableBackend) StopEditor(context.Context, Instance) error      { return ErrUnavailable }
func (UnavailableBackend) ListEditors(context.Context) ([]Instance, error) { return nil, nil }
func (UnavailableBackend) Inspect(context.Context, string, string, string, []Checkout) (Inspection, error) {
	return Inspection{}, ErrUnavailable
}
