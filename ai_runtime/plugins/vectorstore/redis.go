package vectorstore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	redis_impl "github.com/go-redis/redis/v8"
	"github.com/vaastav/agentic_blueprint/ai_runtime/core"
)

const (
	indexName       = "vector_index"
	vectorFieldName = "vector"
	metadataField   = "metadata"
)

type RedisStackVectorStore struct {
	client      *redis_impl.Client
	dimensions  int
	initialized bool
}

func NewRedisStackVectorStore(ctx context.Context, addr string, dimensions int) (*RedisStackVectorStore, error) {
	client := redis_impl.NewClient(&redis_impl.Options{
		Addr: addr,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	store := &RedisStackVectorStore{
		client:      client,
		dimensions:  dimensions,
		initialized: false,
	}

	if err := store.ensureIndex(ctx); err != nil {
		return nil, fmt.Errorf("failed to create vector index: %w", err)
	}

	return store, nil
}

func (s *RedisStackVectorStore) ensureIndex(ctx context.Context) error {
	if s.initialized {
		return nil
	}

	exists, err := s.client.Do(ctx, "FT.INFO", indexName).Result()
	if err == nil && exists != nil {
		s.initialized = true
		return nil
	}

	createCmd := []interface{}{
		"FT.CREATE", indexName,
		"ON", "HASH",
		"PREFIX", 1, "doc:",
		"SCHEMA",
		vectorFieldName, "VECTOR", "HNSW", 6, "TYPE", "FLOAT32", "DIM", s.dimensions, "DISTANCE_METRIC", "COSINE",
		metadataField, "TEXT",
	}

	if err := s.client.Do(ctx, createCmd...).Err(); err != nil {
		return fmt.Errorf("failed to create FT index: %w", err)
	}

	s.initialized = true
	return nil
}

func (s *RedisStackVectorStore) Store(ctx context.Context, id string, vector []float64, metadata map[string]any) error {
	vectorBytes := make([]byte, len(vector)*4)
	for i, v := range vector {
		bits := math.Float32bits(float32(v))
		binary.LittleEndian.PutUint32(vectorBytes[i*4:], bits)
	}

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	key := "doc:" + id
	return s.client.HSet(ctx, key, map[string]interface{}{
		vectorFieldName: vectorBytes,
		metadataField:   string(metaJSON),
	}).Err()
}

func (s *RedisStackVectorStore) Query(ctx context.Context, vector []float64, topK int) ([]core.VectorMatch, error) {
	queryVector := make([]byte, len(vector)*4)
	for i, v := range vector {
		bits := math.Float32bits(float32(v))
		binary.LittleEndian.PutUint32(queryVector[i*4:], bits)
	}

	args := []interface{}{
		"FT.SEARCH", indexName,
		"*=>[KNN " + strconv.Itoa(topK) + " @" + vectorFieldName + " $BLOB]",
		"PARAMS", 2, "BLOB", queryVector,
		"RETURN", 2, vectorFieldName, metadataField,
		"DIALECT", 2,
	}

	result, err := s.client.Do(ctx, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("FT.SEARCH failed: %w", err)
	}

	return s.parseSearchResult(result)
}

func (s *RedisStackVectorStore) parseSearchResult(result interface{}) ([]core.VectorMatch, error) {
	results, ok := result.([]interface{})
	if !ok || len(results) < 1 {
		return nil, nil
	}

	count, ok := results[0].(int64)
	if !ok {
		return nil, fmt.Errorf("unexpected result format")
	}

	if count == 0 {
		return nil, nil
	}

	var matches []core.VectorMatch
	for i := 1; i < len(results); i += 2 {
		if i+1 >= len(results) {
			break
		}

		key, ok := results[i].(string)
		if !ok {
			continue
		}

		docID := key
		if len(key) > 4 && key[:4] == "doc:" {
			docID = key[4:]
		}

		fields, ok := results[i+1].([]interface{})
		if !ok {
			continue
		}

		var metadata map[string]any
		var score float64

		for j := 0; j < len(fields); j += 2 {
			if j+1 >= len(fields) {
				break
			}
			fieldName, _ := fields[j].(string)
			switch fieldName {
			case metadataField:
				if metaStr, ok := fields[j+1].(string); ok {
					json.Unmarshal([]byte(metaStr), &metadata)
				}
			case "__" + vectorFieldName + "_score":
				if scoreStr, ok := fields[j+1].(string); ok {
					score, _ = strconv.ParseFloat(scoreStr, 64)
				}
			}
		}

		metaCopy := make(map[string]any)
		for k, v := range metadata {
			metaCopy[k] = v
		}

		matches = append(matches, core.VectorMatch{
			ID:       docID,
			Score:    score,
			Metadata: metaCopy,
		})
	}

	return matches, nil
}

func (s *RedisStackVectorStore) Delete(ctx context.Context, id string) error {
	key := "doc:" + id
	return s.client.Del(ctx, key).Err()
}
