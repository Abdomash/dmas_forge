package rag_plugin

import (
	"fmt"
	"log/slog"

	"github.com/blueprint-uservices/blueprint/blueprint/pkg/coreplugins/service"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/ir"
	"github.com/blueprint-uservices/blueprint/plugins/golang"
	"github.com/blueprint-uservices/blueprint/plugins/workflow/workflowspec"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
	"github.com/vaastav/agentic_blueprint/ai_runtime/plugins/rag"
)

type KnowledgeBaseClient struct {
	golang.Service
	ir.IRNode
	service.ServiceNode

	Spec       *workflowspec.Service
	ClientName string
}

func newKnowledgeBaseClient[Impl core.KnowledgeBase](name string) (*KnowledgeBaseClient, error) {
	spec, err := workflowspec.GetService[Impl]()
	if err != nil {
		return nil, err
	}
	return &KnowledgeBaseClient{Spec: spec, ClientName: name}, nil
}

func (node *KnowledgeBaseClient) Name() string {
	return node.ClientName
}

func (node *KnowledgeBaseClient) String() string {
	return node.Name() + " = " + node.Spec.Constructor.Name + "()"
}

func (node *KnowledgeBaseClient) AddInstantiation(builder golang.NamespaceBuilder) error {
	if builder.Visited(node.ClientName) {
		return nil
	}

	slog.Info(fmt.Sprintf("Instantiating KnowledgeBaseClient %v in %v/%v", node.ClientName, builder.Info().Package.PackageName, builder.Info().FileName))

	constructor := node.Spec.Constructor.AsConstructor()
	return builder.DeclareConstructor(node.ClientName, constructor, []ir.IRNode{})
}

func (node *KnowledgeBaseClient) AddToWorkspace(builder golang.WorkspaceBuilder) error {
	return node.Spec.AddToWorkspace(builder)
}

func (node *KnowledgeBaseClient) AddInterfaces(builder golang.ModuleBuilder) error {
	return node.Spec.AddToModule(builder)
}

func (node *KnowledgeBaseClient) GetInterface(ctx ir.BuildContext) (service.ServiceInterface, error) {
	return node.Spec.Iface.ServiceInterface(ctx), nil
}

func (node *KnowledgeBaseClient) ImplementsGolangNode() {}

type OpenAIKnowledgeBaseClient struct {
	golang.Service
	ir.IRNode
	service.ServiceNode

	Spec           *workflowspec.Service
	ClientName     string
	URL            string
	APIKey         string
	EmbeddingModel string
	VectorStore    ir.IRNode
}

func newOpenAIKnowledgeBaseClient(name string, url string, apiKey string, embeddingModel string, vectorStore ir.IRNode) (*OpenAIKnowledgeBaseClient, error) {
	spec, err := workflowspec.GetService[rag.OpenAIKnowledgeBase]()
	if err != nil {
		return nil, err
	}
	return &OpenAIKnowledgeBaseClient{
		Spec:           spec,
		ClientName:     name,
		URL:            url,
		APIKey:         apiKey,
		EmbeddingModel: embeddingModel,
		VectorStore:    vectorStore,
	}, nil
}

func (node *OpenAIKnowledgeBaseClient) Name() string {
	return node.ClientName
}

func (node *OpenAIKnowledgeBaseClient) String() string {
	return node.Name() + " = OpenAIKnowledgeBase(" + node.VectorStore.Name() + ")"
}

func (node *OpenAIKnowledgeBaseClient) AddInstantiation(builder golang.NamespaceBuilder) error {
	if builder.Visited(node.ClientName) {
		return nil
	}

	slog.Info(fmt.Sprintf("Instantiating OpenAIKnowledgeBaseClient %v in %v/%v", node.ClientName, builder.Info().Package.PackageName, builder.Info().FileName))

	constructor := node.Spec.Constructor.AsConstructor()
	return builder.DeclareConstructor(node.ClientName, constructor, []ir.IRNode{
		&ir.IRValue{Value: node.URL},
		&ir.IRValue{Value: node.APIKey},
		&ir.IRValue{Value: node.EmbeddingModel},
		node.VectorStore,
	})
}

func (node *OpenAIKnowledgeBaseClient) AddToWorkspace(builder golang.WorkspaceBuilder) error {
	return node.Spec.AddToWorkspace(builder)
}

func (node *OpenAIKnowledgeBaseClient) AddInterfaces(builder golang.ModuleBuilder) error {
	return node.Spec.AddToModule(builder)
}

func (node *OpenAIKnowledgeBaseClient) GetInterface(ctx ir.BuildContext) (service.ServiceInterface, error) {
	return node.Spec.Iface.ServiceInterface(ctx), nil
}

func (node *OpenAIKnowledgeBaseClient) ImplementsGolangNode() {}

type RAGAgentClient struct {
	golang.Service
	ir.IRNode
	service.ServiceNode

	Spec       *workflowspec.Service
	ClientName string
	InnerAgent ir.IRNode
	KB         ir.IRNode
	ReadOnly   bool
}

func newRAGAgentClient(name string, innerAgent ir.IRNode, kb ir.IRNode, readOnly bool) (*RAGAgentClient, error) {
	spec, err := workflowspec.GetService[rag.RAGAgent]()
	if err != nil {
		return nil, err
	}
	return &RAGAgentClient{
		Spec:       spec,
		ClientName: name,
		InnerAgent: innerAgent,
		KB:         kb,
		ReadOnly:   readOnly,
	}, nil
}

func (node *RAGAgentClient) Name() string {
	return node.ClientName
}

func (node *RAGAgentClient) String() string {
	return node.Name() + " = RAGAgent(" + node.InnerAgent.Name() + ", " + node.KB.Name() + ")"
}

func (node *RAGAgentClient) AddInstantiation(builder golang.NamespaceBuilder) error {
	if builder.Visited(node.ClientName) {
		return nil
	}

	slog.Info(fmt.Sprintf("Instantiating RAGAgentClient %v in %v/%v", node.ClientName, builder.Info().Package.PackageName, builder.Info().FileName))

	constructor := node.Spec.Constructor.AsConstructor()
	return builder.DeclareConstructor(node.ClientName, constructor, []ir.IRNode{
		node.InnerAgent,
		node.KB,
		&ir.IRValue{Value: fmt.Sprintf("%v", node.ReadOnly)},
	})
}

func (node *RAGAgentClient) AddToWorkspace(builder golang.WorkspaceBuilder) error {
	return node.Spec.AddToWorkspace(builder)
}

func (node *RAGAgentClient) AddInterfaces(builder golang.ModuleBuilder) error {
	return node.Spec.AddToModule(builder)
}

func (node *RAGAgentClient) GetInterface(ctx ir.BuildContext) (service.ServiceInterface, error) {
	return node.Spec.Iface.ServiceInterface(ctx), nil
}

func (node *RAGAgentClient) ImplementsGolangNode() {}
