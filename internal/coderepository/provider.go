package coderepository

import "context"

type CodeRepositoryProvider interface {
	ProviderName() string
	Clone(context.Context, string, string) error
	SyncMain(context.Context, string, string) error
	PrepareTaskCheckout(context.Context, string, string, string) error
}
