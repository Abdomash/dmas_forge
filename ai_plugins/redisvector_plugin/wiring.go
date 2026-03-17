package redisvector_plugin

import (
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/blueprint"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/coreplugins/address"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/coreplugins/pointer"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/ir"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/wiring"
)

func Container(spec wiring.WiringSpec, name string, dimensions int) string {
	ctrName := name + ".ctr"
	clientName := name + ".client"
	addrName := name + ".addr"

	spec.Define(ctrName, &RedisStackContainer{}, func(ns wiring.Namespace) (ir.IRNode, error) {
		ctr, err := newRedisStackContainer(ctrName)
		if err != nil {
			return nil, err
		}

		err = address.Bind[*RedisStackContainer](ns, addrName, ctr, &ctr.BindAddr)
		return ctr, err
	})

	ptr := pointer.CreatePointer[*RedisStackClient](spec, name, ctrName)

	address.Define[*RedisStackContainer](spec, addrName, ctrName)

	ptr.AddAddrModifier(spec, addrName)

	clientNext := ptr.AddSrcModifier(spec, clientName)
	spec.Define(clientName, &RedisStackClient{}, func(ns wiring.Namespace) (ir.IRNode, error) {
		addr, err := address.Dial[*RedisStackContainer](ns, clientNext)
		if err != nil {
			return nil, blueprint.Errorf("%s expected %s to be an address but encountered %s", clientName, clientNext, err)
		}

		return newRedisStackClient(clientName, addr.Dial, dimensions)
	})

	return name
}
