module github.com/vaastav/agentic_blueprint/examples/agentic-hotel/wiring

go 1.23.0

require (
	github.com/blueprint-uservices/blueprint/blueprint v0.0.0-20250729202253-a8f505263256
	github.com/blueprint-uservices/blueprint/plugins v0.0.0-20250729202253-a8f505263256
	github.com/vaastav/agentic_blueprint/ai_plugins v0.0.0
	github.com/vaastav/agentic_blueprint/ai_runtime v0.0.0
	github.com/vaastav/agentic_blueprint/examples/agentic-hotel/workflow v0.0.0
)

require (
	github.com/blueprint-uservices/blueprint/runtime v0.0.0-20240405152959-f078915d2306 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hailocab/go-geoindex v0.0.0-20160127134810-64631bfe9711 // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/openai/openai-go v1.11.1 // indirect
	github.com/otiai10/copy v1.14.0 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240424034433-3c2c7870ae76 // indirect
	go.mongodb.org/mongo-driver v1.15.0 // indirect
	go.opentelemetry.io/otel v1.26.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.26.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.26.0 // indirect
	go.opentelemetry.io/otel/metric v1.26.0 // indirect
	go.opentelemetry.io/otel/sdk v1.26.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.26.0 // indirect
	go.opentelemetry.io/otel/trace v1.26.0 // indirect
	golang.org/x/crypto v0.32.0 // indirect
	golang.org/x/exp v0.0.0-20240416160154-fe59bbe5cc7f // indirect
	golang.org/x/mod v0.25.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/tools v0.34.0 // indirect
)

replace github.com/vaastav/agentic_blueprint/ai_plugins => ../../../ai_plugins

replace github.com/vaastav/agentic_blueprint/ai_runtime => ../../../ai_runtime

replace github.com/vaastav/agentic_blueprint/examples/agentic-hotel/workflow => ../workflow
