package orchestration

func (o *Orchestrator) CancelTurn() {
	// TODO: This could potentially be done directly on the turn instead of
	// as an exposed method
	if o.conversation.cancelActiveTurn() {
		if o.orchestrateOptions.onCancellation != nil {
			o.orchestrateOptions.onCancellation()
		}
		o.UnpauseTurn()
	}
}

func (o *Orchestrator) PauseTurn() {
	if o.audioOutput != nil {
		o.audioOutput.ClearBuffer()
	}
	o.conversation.pauseActiveTurn()
}

func (o *Orchestrator) UnpauseTurn() {
	o.conversation.unpauseActiveTurn()
}
