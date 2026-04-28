module github.com/vaastav/agentic_blueprint/examples/marketing-agency/workflow

go 1.22.1

require github.com/vaastav/agentic_blueprint/ai_runtime v0.0.0

require (
	github.com/openai/openai-go v1.11.1
	go.opentelemetry.io/otel v1.26.0
	go.opentelemetry.io/otel/trace v1.26.0
)

require (
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	go.opentelemetry.io/otel/metric v1.26.0 // indirect
)

replace github.com/vaastav/agentic_blueprint/ai_runtime => ../../../ai_runtime
