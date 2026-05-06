package specs

import (
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/wiring"
	"github.com/blueprint-uservices/blueprint/plugins/cmdbuilder"
	"github.com/blueprint-uservices/blueprint/plugins/goproc"
	"github.com/blueprint-uservices/blueprint/plugins/http"
	"github.com/blueprint-uservices/blueprint/plugins/linuxcontainer"
)

var HTTP = cmdbuilder.SpecOption{
	Name:        "http",
	Description: "Deploy each agentic hotel service as its own HTTP service",
	Build:       makeHTTPSpec,
}

func makeHTTPSpec(spec wiring.WiringSpec) ([]string, error) {
	s, err := defineHotelServices(spec)
	if err != nil {
		return nil, err
	}

	containers := []string{}
	for _, svc := range s.all() {
		http.Deploy(spec, svc)
		goproc.Deploy(spec, svc)
		containers = append(containers, linuxcontainer.Deploy(spec, svc))
	}

	return append(containers, s.dbs...), nil
}
