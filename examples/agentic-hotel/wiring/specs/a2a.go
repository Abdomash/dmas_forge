package specs

import (
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/wiring"
	"github.com/blueprint-uservices/blueprint/plugins/cmdbuilder"
	"github.com/blueprint-uservices/blueprint/plugins/goproc"
	"github.com/blueprint-uservices/blueprint/plugins/http"
	"github.com/blueprint-uservices/blueprint/plugins/linuxcontainer"
	"github.com/vaastav/agentic_blueprint/ai_plugins/a2a"
)

var A2A = cmdbuilder.SpecOption{
	Name:        "a2a",
	Description: "Deploy internal agentic hotel services over A2A with frontend exposed over HTTP",
	Build:       makeA2ASpec,
}

func makeA2ASpec(spec wiring.WiringSpec) ([]string, error) {
	s, err := defineHotelServices(spec)
	if err != nil {
		return nil, err
	}

	containers := []string{}
	for _, svc := range s.internal() {
		a2a.Deploy(spec, svc)
		goproc.Deploy(spec, svc)
		containers = append(containers, linuxcontainer.Deploy(spec, svc))
	}

	http.Deploy(spec, s.frontend)
	goproc.Deploy(spec, s.frontend)
	containers = append(containers, linuxcontainer.Deploy(spec, s.frontend))

	return append(containers, s.dbs...), nil
}
