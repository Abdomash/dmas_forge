package rag

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
)

const (
	defaultChunkSize = 500
	defaultOverlap   = 50
)

type chunkEntry struct {
	content     string
	sourceDocID string
	metadata    map[string]any
}

type InMemoryKnowledgeBase struct {
	chunks    []chunkEntry
	docIDs    map[string]bool
	mu        sync.RWMutex
	chunkSize int
	overlap   int
}

func NewInMemoryKnowledgeBase(ctx context.Context) (*InMemoryKnowledgeBase, error) {
	return &InMemoryKnowledgeBase{
		chunks:    make([]chunkEntry, 0),
		docIDs:    make(map[string]bool),
		chunkSize: defaultChunkSize,
		overlap:   defaultOverlap,
	}, nil
}

func (kb *InMemoryKnowledgeBase) Index(ctx context.Context, doc core.Document) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	if kb.docIDs[doc.ID] {
		kb.deleteChunksForDoc(doc.ID)
	}

	chunks := kb.chunkDocument(doc)
	for _, chunk := range chunks {
		kb.chunks = append(kb.chunks, chunk)
	}
	kb.docIDs[doc.ID] = true

	return nil
}

func (kb *InMemoryKnowledgeBase) chunkDocument(doc core.Document) []chunkEntry {
	content := doc.Content
	if len(content) == 0 {
		return nil
	}

	var chunks []chunkEntry
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
		metaCopy["chunk_index"] = chunkIndex

		chunks = append(chunks, chunkEntry{
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

func (kb *InMemoryKnowledgeBase) deleteChunksForDoc(docID string) {
	var newChunks []chunkEntry
	for _, chunk := range kb.chunks {
		if chunk.sourceDocID != docID {
			newChunks = append(newChunks, chunk)
		}
	}
	kb.chunks = newChunks
	delete(kb.docIDs, docID)
}

func (kb *InMemoryKnowledgeBase) Query(ctx context.Context, query string, topK int) ([]core.Chunk, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	if len(kb.chunks) == 0 {
		return nil, nil
	}

	queryTerms := tokenize(query)

	type scoredChunk struct {
		chunk chunkEntry
		score float64
	}

	var scored []scoredChunk
	for _, chunk := range kb.chunks {
		score := kb.scoreChunk(queryTerms, chunk.content)
		if score > 0 {
			scored = append(scored, scoredChunk{
				chunk: chunk,
				score: score,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if topK > len(scored) {
		topK = len(scored)
	}

	results := make([]core.Chunk, topK)
	for i := 0; i < topK; i++ {
		metaCopy := make(map[string]any)
		for k, v := range scored[i].chunk.metadata {
			metaCopy[k] = v
		}
		results[i] = core.Chunk{
			Content:     scored[i].chunk.content,
			Score:       scored[i].score,
			SourceDocID: scored[i].chunk.sourceDocID,
			Metadata:    metaCopy,
		}
	}

	return results, nil
}

func (kb *InMemoryKnowledgeBase) scoreChunk(queryTerms []string, content string) float64 {
	contentLower := strings.ToLower(content)
	contentTerms := tokenize(content)

	termFreq := make(map[string]int)
	for _, term := range contentTerms {
		termFreq[term]++
	}

	var score float64
	for _, qt := range queryTerms {
		if termFreq[qt] > 0 {
			score += float64(termFreq[qt])
		} else if strings.Contains(contentLower, qt) {
			score += 0.5
		}
	}

	return score
}

func tokenize(text string) []string {
	text = strings.ToLower(text)
	words := strings.Fields(text)
	var tokens []string
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:()[]\"'")
		if len(word) > 1 {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

func (kb *InMemoryKnowledgeBase) Delete(ctx context.Context, docID string) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	kb.deleteChunksForDoc(docID)
	return nil
}
