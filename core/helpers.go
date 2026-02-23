package orchestration

import (
	"context"
	"fmt"
)

func (o *Orchestrator) currentResponsePipeline() *responsePipeline {
	if o == nil {
		return nil
	}

	return o.responsePipeline.Load()
}

func (o *Orchestrator) currentActiveContext() context.Context {
	if o == nil {
		return context.Background()
	}

	if activeTurnCtx := o.currentResponsePipeline().Ctx(); activeTurnCtx != nil {
		return activeTurnCtx
	}

	if o.baseContext != nil {
		return o.baseContext
	}

	return context.Background()
}

func withContextCancelHook(ctx context.Context, onContextDone func()) chan struct{} {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			onContextDone()
		case <-done:
		}
	}()
	return done
}

type workerRun func(context.Context) error

func panicSafeNamedWorker(name string, run func(context.Context) error) workerRun {
	return func(ctx context.Context) (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("%s worker panicked: %v", name, recovered)
			}
		}()

		if err = run(ctx); err != nil {
			return fmt.Errorf("%s worker failed: %w", name, err)
		}

		return nil
	}
}
