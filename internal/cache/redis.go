package cache

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/k9pranav/LLM_Cache/pkg/types"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	Client *redis.Client
}

func NewRedisCache(addr string, password string, db int) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,

		Protocol: 2,
	})

	return &RedisCache{
		Client: client,
	}
}

func (r *RedisCache) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}

func (r *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.Client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return r.Client.Get(ctx, key).Result()
}

func (r *RedisCache) CreateIndex(ctx context.Context, dim int) error {

	//If index exists, do nothing
	if err := r.Client.Do(ctx, "FT.INFO", indexName).Err(); err == nil {
		return nil
	}

	return r.Client.Do(
		ctx,
		"FT.CREATE", indexName,
		"ON", "HASH",
		"PREFIX", "1", redisPrefix+":entry:",
		"SCHEMA",
		"tenant_id", "TAG",
		"provider", "TAG",
		"model", "TAG",
		"system_prompt_hash", "TAG",
		"normalized_query", "TEXT",
		"response", "TEXT",
		"embedding", "VECTOR", "HNSW", "6",
		"TYPE", "FLOAT32",
		"DIM", dim,
		"DISTANCE_METRIC", "COSINE",
	).Err()
}

//Stores the cache entry has and its vector

func (r *RedisCache) StoreEntry(ctx context.Context, entry types.CacheEntry, vector []float32, exactKey string, ttl time.Duration) error {
	vectorBytes, err := float32VectorToBytes(vector)

	if err != nil {
		return err
	}

	fields := map[string]interface{}{
		"id":                 entry.ID,
		"tenant_id":          entry.TenantID,
		"scope":              entry.Scope,
		"raw_query":          entry.RawQuery,
		"normalized_query":   entry.NormalizedQuery,
		"response":           entry.Response,
		"provider":           entry.Provider,
		"model":              entry.Model,
		"system_prompt_hash": entry.SystemPromptHash,
		"created_at":         entry.CreatedAt.Format(time.RFC3339),
		"expires_at":         entry.ExpiresAt.Format(time.RFC3339),
		"reuse_count":        entry.ReuseCount,
		"embedding":          vectorBytes,
	}

	if err := r.Client.HSet(ctx, entry.ID, fields).Err(); err != nil {
		return err
	}

	if ttl > 0 {
		if err := r.Client.Expire(ctx, entry.ID, ttl).Err(); err != nil {
			return err
		}
	}

	//Exact key map to redis entry key. Actually storing the key here
	if err := r.Client.Set(ctx, exactKey, entry.ID, ttl).Err(); err != nil {
		return err
	}

	return nil
}

//GetEaxctKey follows exactKey -> entryID -> Redis Hash

func (r *RedisCache) GetExactEntry(ctx context.Context, exactKey string) (*types.CacheEntry, error) {
	entryID, err := r.Client.Get(ctx, exactKey).Result()

	if err != nil {
		return nil, err
	}

	return r.GetEntry(ctx, entryID)
}

func (r *RedisCache) GetEntry(ctx context.Context, entryID string) (*types.CacheEntry, error) {
	values, err := r.Client.HMGet(
		ctx,
		entryID,
		"id",
		"tenant_id",
		"scope",
		"raw_query",
		"normalized_query",
		"response",
		"provider",
		"model",
		"system_prompt_hash",
		"created_at",
		"expires_at",
		"reuse_count",
	).Result()

	if err != nil {
		return nil, err
	}

	if values[0] == nil {
		return nil, redis.Nil
	}

	entry := &types.CacheEntry{
		ID:               asString(values[0]),
		TenantID:         asString(values[1]),
		Scope:            asString(values[2]),
		RawQuery:         asString(values[3]),
		NormalizedQuery:  asString(values[4]),
		Response:         asString(values[5]),
		Provider:         asString(values[6]),
		Model:            asString(values[7]),
		SystemPromptHash: asString(values[8]),
		CreatedAt:        parseTime(asString(values[9])),
		ExpiresAt:        parseTime(asString(values[10])),
		ReuseCount:       parseInt(asString(values[11])),
	}

	return entry, nil
}

//Semantic search performs vector KNN search in Redis

func (r *RedisCache) SemanticSearch(ctx context.Context, req types.SemanticLookupRequest) ([]types.CacheCandidate, error) {
	vectorBytes, err := float32VectorToBytes(req.Vector)

	if err != nil {
		return nil, err
	}

	model := req.Filters["model"]
	provider := req.Filters["provider"]
	systemPromptHash := req.Filters["system_prompt_hash"]

	filter := fmt.Sprintf(
		"(@tenant_id:{%s} @model:{%s} @provider:{%s} @system_prompt_hash:{%s})=>[KNN %d @embedding $vec AS vector_score]",
		escapeTag(req.TenantID),
		escapeTag(model),
		escapeTag(provider),
		escapeTag(systemPromptHash),
		req.TopK,
	)

	raw, err := r.Client.Do(
		ctx,
		"FT.SEARCH", indexName,
		filter,
		"PARAMS", "2", "vec", vectorBytes,
		"SORTBY", "vector_score",
		"RETURN", "13",
		"id",
		"tenant_id",
		"scope",
		"raw_query",
		"normalized_query",
		"response",
		"provider",
		"model",
		"system_prompt_hash",
		"created_at",
		"expires_at",
		"reuse_count",
		"vector_score",
		"DIALECT", "2",
	).Result()

	if err != nil {
		return nil, err
	}

	return parseSearchResults(raw)

}

func (r *RedisCache) IncrementReuseCount(ctx context.Context, entryID string) error {
	return r.Client.HIncrBy(ctx, entryID, "reuse_count", 1).Err()
}

func float32VectorToBytes(vector []float32) ([]byte, error) {
	buf := new(bytes.Buffer)

	for _, v := range vector {
		if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func parseSearchResults(raw interface{}) ([]types.CacheCandidate, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected redis search result type")
	}

	if len(items) <= 1 {
		return []types.CacheCandidate{}, nil
	}

	candidates := make([]types.CacheCandidate, 0)

	rank := 1

	for i := 1; i < len(items); i += 2 {
		if i+1 >= len(items) {
			break
		}

		fields, ok := items[i+1].([]interface{})
		if !ok {
			continue
		}

		fieldMap := redisFieldListToMap(fields)

		distance := parseFloat(fieldMap["vector_score"])
		similarity := 1.0 - distance

		entry := types.CacheEntry{
			ID:               fieldMap["id"],
			TenantID:         fieldMap["tenant_id"],
			Scope:            fieldMap["scope"],
			RawQuery:         fieldMap["raw_query"],
			NormalizedQuery:  fieldMap["normalized_query"],
			Response:         fieldMap["response"],
			Provider:         fieldMap["provider"],
			Model:            fieldMap["model"],
			SystemPromptHash: fieldMap["system_prompt_hash"],
			CreatedAt:        parseTime(fieldMap["created_at"]),
			ExpiresAt:        parseTime(fieldMap["expires_at"]),
			ReuseCount:       parseInt(fieldMap["reuse_count"]),
		}

		candidates = append(candidates, types.CacheCandidate{
			Entry:      entry,
			Similarity: similarity,
			Rank:       rank,
		})

		rank++
	}

	return candidates, nil
}

func redisFieldListToMap(fields []interface{}) map[string]string {
	result := make(map[string]string)

	for i := 0; i < len(fields); i += 2 {
		if i+1 >= len(fields) {
			break
		}

		key := asString(fields[i])
		value := asString(fields[i+1])
		result[key] = value
	}

	return result
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func parseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339, value)
	return t
}

func parseInt(value string) int {
	i, _ := strconv.Atoi(value)
	return i
}

func parseFloat(value string) float64 {
	f, _ := strconv.ParseFloat(value, 64)
	return f
}

func escapeTag(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		",", "\\,",
		".", "\\.",
		"<", "\\<",
		">", "\\>",
		"{", "\\{",
		"}", "\\}",
		"[", "\\[",
		"]", "\\]",
		"\"", "\\\"",
		"'", "\\'",
		":", "\\:",
		";", "\\;",
		"!", "\\!",
		"@", "\\@",
		"#", "\\#",
		"$", "\\$",
		"%", "\\%",
		"^", "\\^",
		"&", "\\&",
		"*", "\\*",
		"(", "\\(",
		")", "\\)",
		"-", "\\-",
		"+", "\\+",
		"=", "\\=",
		"~", "\\~",
		" ", "\\ ",
	)

	return replacer.Replace(value)
}
