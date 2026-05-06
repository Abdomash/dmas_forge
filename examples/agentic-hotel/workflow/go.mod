module github.com/vaastav/agentic_blueprint/examples/agentic-hotel/workflow

go 1.23.0

require (
	github.com/blueprint-uservices/blueprint/runtime v0.0.0-20240405152959-f078915d2306
	github.com/hailocab/go-geoindex v0.0.0-20160127134810-64631bfe9711
	github.com/openai/openai-go v1.11.1
	github.com/vaastav/agentic_blueprint/ai_runtime v0.0.0
	go.mongodb.org/mongo-driver v1.15.0
)

require (
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	go.opentelemetry.io/otel v1.26.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.26.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.26.0 // indirect
	go.opentelemetry.io/otel/metric v1.26.0 // indirect
	go.opentelemetry.io/otel/sdk v1.26.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.26.0 // indirect
	go.opentelemetry.io/otel/trace v1.26.0 // indirect
	golang.org/x/exp v0.0.0-20230728194245-b0cb94b80691 // indirect
)

replace github.com/vaastav/agentic_blueprint/ai_runtime => ../../../ai_runtime
