package orchestration

import (
	"context"
	"sync"
)

// backgroundWork gates admission before cancellation and joining. It owns only
// platform work; stopping it never stops an independently owned agentctl process.
type backgroundWork struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
	work   sync.WaitGroup
}

func newBackgroundWork(ctx context.Context) *backgroundWork {
	ctx, cancel := context.WithCancel(ctx)
	return &backgroundWork{ctx: ctx, cancel: cancel}
}

func (b *backgroundWork) Go(fn func(context.Context)) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.ctx.Err() != nil {
		return false
	}
	b.work.Add(1)
	go func() { defer b.work.Done(); fn(b.ctx) }()
	return true
}

func (b *backgroundWork) Close() {
	b.mu.Lock()
	b.closed = true
	b.cancel()
	b.mu.Unlock()
	b.work.Wait()
}

// reconcileBatch uses fixed workers for bounded attempts, never persistent streams.
func reconcileBatch(ctx context.Context, launches []Launch, attempt func(context.Context, Launch)) {
	jobs := make(chan Launch)
	var work sync.WaitGroup
	for range 4 {
		work.Go(func() {
			for launch := range jobs {
				if ctx.Err() != nil {
					return
				}
				attempt(ctx, launch)
			}
		})
	}
	defer work.Wait()
	defer close(jobs)
	for _, launch := range launches {
		select {
		case <-ctx.Done():
			return
		case jobs <- launch:
		}
	}
}
