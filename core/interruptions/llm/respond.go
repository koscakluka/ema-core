package llm

import (
	"context"
	"fmt"

	"github.com/koscakluka/ema-core/core/interruptions"
	"github.com/koscakluka/ema-core/core/llms"
)

func respond(ctx context.Context, interruption llms.InterruptionV0, o interruptions.OrchestratorV0) (*llms.InterruptionV0, error) {
	switch interruptionType(interruption.Type) {
	case InterruptionTypeContinuation:
		o.CancelTurn()
		found := -1
		count := 0
		for turn := range o.Turns().RValues {
			if turn.Role == llms.TurnRoleUser {
				found = count
				break
			}
			count++
		}

		if found == -1 {
			// TODO: Queue prompt since there is nothing to add it to
			interruption.Resolved = true
			return &interruption, nil
		}

		for range found {
			o.Turns().Pop()
		}

		lastUserTurn := o.Turns().Pop()
		if lastUserTurn != nil {
			o.QueuePrompt(lastUserTurn.Content + " " + interruption.Source)
		} else {
			o.QueuePrompt(interruption.Source)
		}
		interruption.Resolved = true
		return &interruption, nil

	case InterruptionTypeClarification:
		o.CancelTurn()
		o.QueuePrompt(interruption.Source)
		interruption.Resolved = true
		return &interruption, nil

	case InterruptionTypeCancellation:
		o.CancelTurn()
		interruption.Resolved = true
		return &interruption, nil

	case InterruptionTypeIgnorable,
		InterruptionTypeRepetition,
		InterruptionTypeNoise:
		interruption.Resolved = true
		return &interruption, nil

	case InterruptionTypeAction:
		interruption.Resolved = true
		if err := o.CallTool(ctx, interruption.Source); err != nil {
			return nil, err
		}
		return &interruption, nil

	case InterruptionTypeNewPrompt:
		o.QueuePrompt(interruption.Source)
		interruption.Resolved = true
		return &interruption, nil

	default:
		return nil, fmt.Errorf("unknown interruption type: %s", interruption.Type)
	}
}
