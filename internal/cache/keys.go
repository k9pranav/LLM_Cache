//This creates stable Redis keys

package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	redisPrefix = "llmcache"
	indexName   = "idx:llmcache"
)

func EntryKey(tenantID string, exactKeyText string, model string, systemPromptHash string) string {
	raw := strings.Join([]string{
		tenantID,
		model,
		systemPromptHash,
		exactKeyText,
	}, "|")

	return redisPrefix + ":entry:" + sha256Hex(raw)
}

func ExactKey(tenantID string, exactKeyText string, model string, systemPromptHash string) string {
	raw := strings.Join([]string{
		tenantID,
		model,
		systemPromptHash,
		exactKeyText,
	}, "|")

	return redisPrefix + ":exact:" + sha256Hex(raw)
}

func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
