package specs

import (
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/wiring"
	"github.com/blueprint-uservices/blueprint/plugins/cmdbuilder"
	"github.com/blueprint-uservices/blueprint/plugins/goproc"
	"github.com/blueprint-uservices/blueprint/plugins/http"
	"github.com/blueprint-uservices/blueprint/plugins/linuxcontainer"
)

var Single = cmdbuilder.SpecOption{
	Name:        "single",
	Description: "Deploy agentic hotel behind one frontend HTTP endpoint",
	Build:       makeSingleSpec,
}

func makeSingleSpec(spec wiring.WiringSpec) ([]string, error) {
	s, err := defineHotelServices(spec)
	if err != nil {
		return nil, err
	}

	http.Deploy(spec, s.frontend)
	goproc.Deploy(spec, s.frontend)
	ctr := linuxcontainer.Deploy(spec, s.frontend)

	return append([]string{ctr}, s.dbs...), nil
}
