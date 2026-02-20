package orchestration

import "context"

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
