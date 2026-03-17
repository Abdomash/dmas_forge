package vectorstore

import (
	"context"
	"math"
	"sort"
	"sync"

	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
)

type vectorEntry struct {
	vector   []float64
	metadata map[string]any
}

type InMemoryVectorStore struct {
	entries map[string]vectorEntry
	mu      sync.RWMutex
}

func NewInMemoryVectorStore(ctx context.Context) (*InMemoryVectorStore, error) {
	return &InMemoryVectorStore{
		entries: make(map[string]vectorEntry),
	}, nil
}

func (s *InMemoryVectorStore) Store(ctx context.Context, id string, vector []float64, metadata map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	vecCopy := make([]float64, len(vector))
	copy(vecCopy, vector)

	metaCopy := make(map[string]any)
	for k, v := range metadata {
		metaCopy[k] = v
	}

	s.entries[id] = vectorEntry{
		vector:   vecCopy,
		metadata: metaCopy,
	}
	return nil
}

func (s *InMemoryVectorStore) Query(ctx context.Context, vector []float64, topK int) ([]core.VectorMatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type scoredEntry struct {
		id       string
		score    float64
		metadata map[string]any
	}

	var results []scoredEntry
	for id, entry := range s.entries {
		score := cosineSimilarity(vector, entry.vector)
		results = append(results, scoredEntry{
			id:       id,
			score:    score,
			metadata: entry.metadata,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topK > len(results) {
		topK = len(results)
	}

	matches := make([]core.VectorMatch, topK)
	for i := 0; i < topK; i++ {
		metaCopy := make(map[string]any)
		for k, v := range results[i].metadata {
			metaCopy[k] = v
		}
		matches[i] = core.VectorMatch{
			ID:       results[i].id,
			Score:    results[i].score,
			Metadata: metaCopy,
		}
	}
	return matches, nil
}

func (s *InMemoryVectorStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
	return nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
