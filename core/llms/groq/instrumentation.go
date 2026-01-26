package groq

import (
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
)

const scopeName = "github.com/koscakluka/ema-core/core/llms/groq"

var (
	tracer = otel.Tracer(scopeName)
	meter  = otel.Meter(scopeName)
	logger = otelslog.NewLogger(scopeName)
)
