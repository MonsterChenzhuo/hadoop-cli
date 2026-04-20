package orchestrator

import (
	"context"
	"sync"
	"time"
)

type Executor interface {
	Execute(ctx context.Context, host string, task Task) Result
}

type Runner struct {
	exec        Executor
	parallelism int
	defaultTO   time.Duration
}

func NewRunner(exec Executor, parallelism int) *Runner {
	if parallelism <= 0 {
		parallelism = 4
	}
	return &Runner{exec: exec, parallelism: parallelism, defaultTO: 5 * time.Minute}
}

func (r *Runner) Run(ctx context.Context, hosts []string, task Task) []Result {
	if task.Timeout == 0 {
		task.Timeout = r.defaultTO
	}
	out := make([]Result, len(hosts))
	sem := make(chan struct{}, r.parallelism)
	var wg sync.WaitGroup

	for i, h := range hosts {
		i, h := i, h
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			subCtx, cancel := context.WithTimeout(ctx, task.Timeout)
			defer cancel()
			start := time.Now()
			res := r.exec.Execute(subCtx, h, task)
			res.Elapsed = time.Since(start)
			if res.Host == "" {
				res.Host = h
			}
			out[i] = res
		}()
	}
	wg.Wait()
	return out
}
