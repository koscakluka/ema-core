package llm

import (
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
)

const scopeName = "github.com/koscakluka/ema-core/core/interruptions/llm"

var (
	tracer = otel.Tracer(scopeName)
	meter  = otel.Meter(scopeName)
	logger = otelslog.NewLogger(scopeName)
)
