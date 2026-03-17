package rag_plugin

import (
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/coreplugins/pointer"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/ir"
	"github.com/blueprint-uservices/blueprint/blueprint/pkg/wiring"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
)

type RAGAgentConfig struct {
	ReadOnly bool
}

func KnowledgeBase[Impl core.KnowledgeBase](spec wiring.WiringSpec, name string) string {
	backendName := name + ".knowledge_base"

	spec.Define(backendName, &KnowledgeBaseClient{}, func(ns wiring.Namespace) (ir.IRNode, error) {
		return newKnowledgeBaseClient[Impl](name)
	})

	pointer.CreatePointer[*KnowledgeBaseClient](spec, name, backendName)

	return name
}

func OpenAIKnowledgeBase(spec wiring.WiringSpec, name string, openaiURL string, openaiAPIKey string, embeddingModel string, vectorStoreName string) string {
	backendName := name + ".openai_knowledge_base"

	spec.Define(backendName, &OpenAIKnowledgeBaseClient{}, func(ns wiring.Namespace) (ir.IRNode, error) {
		var vectorStore ir.IRNode
		if err := ns.Get(vectorStoreName, &vectorStore); err != nil {
			return nil, err
		}
		return newOpenAIKnowledgeBaseClient(name, openaiURL, openaiAPIKey, embeddingModel, vectorStore)
	})

	pointer.CreatePointer[*OpenAIKnowledgeBaseClient](spec, name, backendName)

	return name
}

func RAGAgent(spec wiring.WiringSpec, name string, agentName string, kbName string, config RAGAgentConfig) string {
	backendName := name + ".rag_agent"

	spec.Define(backendName, &RAGAgentClient{}, func(ns wiring.Namespace) (ir.IRNode, error) {
		var innerAgent ir.IRNode
		if err := ns.Get(agentName, &innerAgent); err != nil {
			return nil, err
		}
		var kb ir.IRNode
		if err := ns.Get(kbName, &kb); err != nil {
			return nil, err
		}
		return newRAGAgentClient(name, innerAgent, kb, config.ReadOnly)
	})

	pointer.CreatePointer[*RAGAgentClient](spec, name, backendName)

	return name
}
