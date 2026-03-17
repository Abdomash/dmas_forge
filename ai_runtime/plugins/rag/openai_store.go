package rag

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
)

const (
	defaultOpenAIChunkSize = 1000
	defaultOpenAIOverlap   = 100
)

type OpenAIKnowledgeBase struct {
	vectorStore core.VectorStore
	client      *openai.Client
	model       string
	chunkSize   int
	overlap     int
}

func NewOpenAIKnowledgeBase(ctx context.Context, url, apiKey, embeddingModel string, vectorStore core.VectorStore) (*OpenAIKnowledgeBase, error) {
	client := openai.NewClient(
		option.WithBaseURL(url),
		option.WithAPIKey(apiKey),
	)

	return &OpenAIKnowledgeBase{
		vectorStore: vectorStore,
		client:      &client,
		model:       embeddingModel,
		chunkSize:   defaultOpenAIChunkSize,
		overlap:     defaultOpenAIOverlap,
	}, nil
}

func (kb *OpenAIKnowledgeBase) Index(ctx context.Context, doc core.Document) error {
	kb.Delete(ctx, doc.ID)

	chunks := kb.chunkDocument(doc)

	for i, chunk := range chunks {
		embedding, err := kb.getEmbedding(ctx, chunk.content)
		if err != nil {
			return fmt.Errorf("failed to get embedding for chunk %d: %w", i, err)
		}

		metaCopy := make(map[string]any)
		for k, v := range chunk.metadata {
			metaCopy[k] = v
		}
		metaCopy["content"] = chunk.content
		metaCopy["source_doc_id"] = chunk.sourceDocID
		metaCopy["chunk_index"] = i

		chunkID := fmt.Sprintf("%s_chunk_%d", doc.ID, i)
		if err := kb.vectorStore.Store(ctx, chunkID, embedding, metaCopy); err != nil {
			return fmt.Errorf("failed to store chunk %d: %w", i, err)
		}
	}

	return nil
}

type chunkData struct {
	content     string
	sourceDocID string
	metadata    map[string]any
}

func (kb *OpenAIKnowledgeBase) chunkDocument(doc core.Document) []chunkData {
	content := doc.Content
	if len(content) == 0 {
		return nil
	}

	var chunks []chunkData
	start := 0
	chunkIndex := 0

	for start < len(content) {
		end := start + kb.chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunkContent := content[start:end]

		metaCopy := make(map[string]any)
		for k, v := range doc.Metadata {
			metaCopy[k] = v
		}

		chunks = append(chunks, chunkData{
			content:     chunkContent,
			sourceDocID: doc.ID,
			metadata:    metaCopy,
		})

		chunkIndex++
		start = end - kb.overlap
		if start < 0 {
			start = 0
		}
		if start >= len(content) {
			break
		}
	}

	return chunks
}

func (kb *OpenAIKnowledgeBase) getEmbedding(ctx context.Context, text string) ([]float64, error) {
	input := openai.EmbeddingNewParamsInputUnion{
		OfArrayOfStrings: []string{text},
	}

	resp, err := kb.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(kb.model),
		Input: input,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	embedding := make([]float64, len(resp.Data[0].Embedding))
	for i, v := range resp.Data[0].Embedding {
		embedding[i] = float64(v)
	}

	return embedding, nil
}

func (kb *OpenAIKnowledgeBase) Query(ctx context.Context, query string, topK int) ([]core.Chunk, error) {
	embedding, err := kb.getEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get query embedding: %w", err)
	}

	matches, err := kb.vectorStore.Query(ctx, embedding, topK)
	if err != nil {
		return nil, fmt.Errorf("vector store query failed: %w", err)
	}

	chunks := make([]core.Chunk, len(matches))
	for i, match := range matches {
		content := ""
		sourceDocID := ""
		if c, ok := match.Metadata["content"].(string); ok {
			content = c
		}
		if s, ok := match.Metadata["source_doc_id"].(string); ok {
			sourceDocID = s
		}

		metaCopy := make(map[string]any)
		for k, v := range match.Metadata {
			if k != "content" && k != "source_doc_id" {
				metaCopy[k] = v
			}
		}

		chunks[i] = core.Chunk{
			Content:     content,
			Score:       match.Score,
			SourceDocID: sourceDocID,
			Metadata:    metaCopy,
		}
	}

	return chunks, nil
}

func (kb *OpenAIKnowledgeBase) Delete(ctx context.Context, docID string) error {
	prefix := docID + "_chunk_"

	for i := 0; i < 10000; i++ {
		chunkID := fmt.Sprintf("%s%d", prefix, i)
		if err := kb.vectorStore.Delete(ctx, chunkID); err != nil {
			break
		}
	}

	return nil
}
