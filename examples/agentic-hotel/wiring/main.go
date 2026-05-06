package main

import (
	"github.com/blueprint-uservices/blueprint/plugins/cmdbuilder"
	"github.com/vaastav/agentic_blueprint/examples/agentic-hotel/wiring/specs"
)

func main() {
	cmdbuilder.MakeAndExecute(
		"agentic-hotel",
		specs.Single,
		specs.HTTP,
		specs.A2A,
		specs.MCP,
	)
}
