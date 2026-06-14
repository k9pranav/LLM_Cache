//Turns a chat into cacheable text

package conversation

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/k9pranav/LLM_Cache/internal/normalizer"
	"github.com/k9pranav/LLM_Cache/pkg/types"
)

// BuildLastNContext converts a multi-turn chat request into one cacheable text.
// Approach 1: take the last N non-system messages.

// messages -> past messages including agent and client, NormalizedQuery -> Query type
func BuildLastNContext(messages []types.Message, stripper *normalizer.FillerStripper, lastN int) types.NormalizedQuery {
	if lastN <= 0 {
		lastN = 4
	}

	systemPrompt := collectSystemPrompt(messages)
	systemPromptHash := hashText(systemPrompt)

	nonSystem := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		nonSystem = append(nonSystem, msg)
	}

	start := 0
	if len(nonSystem) > lastN {
		start = len(nonSystem) - lastN
	}

	selected := nonSystem[start:]

	var semanticParts []string
	var exactParts []string

	for _, msg := range selected {
		normalizedContent := stripper.NormalizeQuery(msg.Content)
		if normalizedContent == "" {
			continue
		}

		line := msg.Role + ": " + normalizedContent
		semanticParts = append(semanticParts, line)
		exactParts = append(exactParts, line)
	}

	semanticText := strings.Join(semanticParts, "\n")
	exactKeyText := strings.Join(exactParts, "\n")

	return types.NormalizedQuery{
		SemanticText:     semanticText,
		ExactKeyText:     exactKeyText,
		SystemPromptHash: systemPromptHash,
	}
}

func collectSystemPrompt(messages []types.Message) string {
	var parts []string

	for _, msg := range messages {
		if msg.Role == "system" && strings.TrimSpace(msg.Content) != "" {
			parts = append(parts, strings.TrimSpace(msg.Content))
		}
	}

	return strings.Join(parts, "\n")
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
