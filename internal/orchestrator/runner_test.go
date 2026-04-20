package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeExecutor struct {
	calls int32
	fn    func(ctx context.Context, host string, task Task) Result
}

func (f *fakeExecutor) Execute(ctx context.Context, host string, task Task) Result {
	atomic.AddInt32(&f.calls, 1)
	return f.fn(ctx, host, task)
}

func TestRunner_RunsInParallelAndAggregates(t *testing.T) {
	fe := &fakeExecutor{fn: func(ctx context.Context, host string, _ Task) Result {
		time.Sleep(50 * time.Millisecond)
		return Result{Host: host, OK: true}
	}}
	r := NewRunner(fe, 4)

	start := time.Now()
	results := r.Run(context.Background(), []string{"a", "b", "c", "d"}, Task{Name: "x"})
	elapsed := time.Since(start)

	require.Len(t, results, 4)
	for _, r := range results {
		require.True(t, r.OK)
	}
	require.Less(t, elapsed, 180*time.Millisecond, "should run in parallel")
	require.Equal(t, int32(4), atomic.LoadInt32(&fe.calls))
}

func TestRunner_OneFailureDoesNotAbortOthers(t *testing.T) {
	fe := &fakeExecutor{fn: func(_ context.Context, host string, _ Task) Result {
		if host == "b" {
			return Result{Host: host, OK: false, Err: errors.New("boom")}
		}
		return Result{Host: host, OK: true}
	}}
	r := NewRunner(fe, 2)
	results := r.Run(context.Background(), []string{"a", "b", "c"}, Task{Name: "x"})

	byHost := map[string]Result{}
	for _, r := range results {
		byHost[r.Host] = r
	}
	require.True(t, byHost["a"].OK)
	require.False(t, byHost["b"].OK)
	require.True(t, byHost["c"].OK)
}

func TestRunner_CancelPropagates(t *testing.T) {
	block := make(chan struct{})
	fe := &fakeExecutor{fn: func(ctx context.Context, host string, _ Task) Result {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return Result{Host: host, OK: false, Err: ctx.Err()}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	r := NewRunner(fe, 4)

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	results := r.Run(ctx, []string{"a", "b"}, Task{Name: "x"})
	close(block)
	require.Len(t, results, 2)
}
