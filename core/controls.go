package orchestration

func (o *Orchestrator) CancelTurn() {
	// TODO: This could potentially be done directly on the turn instead of
	// as an exposed method
	if activeTurn := o.turns.activeTurn; activeTurn != nil && !activeTurn.Cancelled {
		// TODO: Replace with a method on the activeTurn
		activeTurn.Cancelled = true
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
	if activeTurn := o.turns.activeTurn; activeTurn != nil {
		activeTurn.Pause()
	}
}

func (o *Orchestrator) UnpauseTurn() {
	if activeTurn := o.turns.activeTurn; activeTurn != nil {
		activeTurn.Unpause()
	}
}
