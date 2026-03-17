package redisvector_plugin

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/blueprint-uservices/blueprint/blueprint/pkg/coreplugins/address"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/coreplugins/service"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/ir"
	"github.com/blueprint-uservices/blueprint/plugins/docker"
	"github.com/blueprint-uservices/blueprint/plugins/golang"
	"github.com/blueprint-uservices/blueprint/plugins/golang/goparser"
	"github.com/blueprint-uservices/blueprint/plugins/workflow/workflowspec"
	"github.com/vaastav/agentic_blueprint/ai_runtime/plugins/vectorstore"
)

type RedisStackContainer struct {
	docker.Container
	docker.ProvidesContainerInstance

	InstanceName string
	BindAddr     *address.BindConfig
	Iface        *goparser.ParsedInterface
}

type RedisStackInterface struct {
	service.ServiceInterface
	Wrapped service.ServiceInterface
}

func (r *RedisStackInterface) GetName() string {
	return "redisvector(" + r.Wrapped.GetName() + ")"
}

func (r *RedisStackInterface) GetMethods() []service.Method {
	return r.Wrapped.GetMethods()
}

func newRedisStackContainer(name string) (*RedisStackContainer, error) {
	spec, err := workflowspec.GetService[vectorstore.RedisStackVectorStore]()
	if err != nil {
		return nil, err
	}
	return &RedisStackContainer{
		InstanceName: name,
		Iface:        spec.Iface,
	}, nil
}

func (r *RedisStackContainer) String() string {
	return r.InstanceName + " = RedisStackProcess(" + r.BindAddr.Name() + ")"
}

func (r *RedisStackContainer) Name() string {
	return r.InstanceName
}

func (r *RedisStackContainer) GetInterface(ctx ir.BuildContext) (service.ServiceInterface, error) {
	iface := r.Iface.ServiceInterface(ctx)
	return &RedisStackInterface{Wrapped: iface}, nil
}

func (r *RedisStackContainer) AddContainerInstance(target docker.ContainerWorkspace) error {
	r.BindAddr.Port = 6379
	return target.DeclarePrebuiltInstance(r.InstanceName, "redis/redis-stack-server", r.BindAddr)
}

type RedisStackClient struct {
	golang.Service
	ir.IRNode
	service.ServiceNode

	InstanceName string
	Addr         *address.DialConfig
	Spec         *workflowspec.Service
	Dimensions   int
}

func newRedisStackClient(name string, addr *address.DialConfig, dimensions int) (*RedisStackClient, error) {
	spec, err := workflowspec.GetService[vectorstore.RedisStackVectorStore]()
	if err != nil {
		return nil, err
	}
	return &RedisStackClient{
		InstanceName: name,
		Addr:         addr,
		Spec:         spec,
		Dimensions:   dimensions,
	}, nil
}

func (n *RedisStackClient) String() string {
	return n.InstanceName + " = RedisStackClient(" + n.Addr.Name() + ")"
}

func (n *RedisStackClient) Name() string {
	return n.InstanceName
}

func (n *RedisStackClient) AddInstantiation(builder golang.NamespaceBuilder) error {
	if builder.Visited(n.InstanceName) {
		return nil
	}

	slog.Info(fmt.Sprintf("Instantiating RedisStackClient %v in %v/%v", n.InstanceName, builder.Info().Package.PackageName, builder.Info().FileName))

	constructor := n.Spec.Constructor.AsConstructor()
	return builder.DeclareConstructor(n.InstanceName, constructor, []ir.IRNode{n.Addr, &ir.IRValue{Value: strconv.Itoa(n.Dimensions)}})
}

func (n *RedisStackClient) AddToWorkspace(builder golang.WorkspaceBuilder) error {
	return n.Spec.AddToWorkspace(builder)
}

func (n *RedisStackClient) AddInterfaces(builder golang.ModuleBuilder) error {
	return n.Spec.AddToModule(builder)
}

func (n *RedisStackClient) GetInterface(ctx ir.BuildContext) (service.ServiceInterface, error) {
	return n.Spec.Iface.ServiceInterface(ctx), nil
}

func (n *RedisStackClient) ImplementsGolangNode()    {}
func (n *RedisStackClient) ImplementsGolangService() {}
