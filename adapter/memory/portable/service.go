// Package portable implements database-independent long-term memory.
package portable

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/izhw/ai-agent/contract"
	"github.com/izhw/ai-agent/internal/idgen"
	memoryspi "github.com/izhw/ai-agent/spi/memory"
	storagespi "github.com/izhw/ai-agent/spi/storage"
)

type Embedder interface {
	Embed(context.Context, string) ([]float32, error)
}

type Retriever struct {
	Store    storagespi.MemoryStore
	Embedder Embedder
}

func (r *Retriever) Retrieve(
	ctx context.Context,
	query memoryspi.Query,
) ([]contract.Memory, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 8
	}
	maxCandidates := query.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 500
	}
	candidates, err := r.Store.ListCandidates(
		ctx,
		query.TenantID,
		query.ProductID,
		query.UserID,
		query.AgentKey,
		maxCandidates,
	)
	if err != nil {
		return nil, err
	}
	var queryVector []float32
	if r.Embedder != nil {
		queryVector, err = r.Embedder.Embed(ctx, query.Text)
		if err != nil {
			return nil, err
		}
	}
	queryTokens := tokens(query.Text)
	type scored struct {
		memory contract.Memory
		score  float64
	}
	scores := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		score := lexicalScore(queryTokens, tokens(candidate.Content))
		if len(queryVector) > 0 && len(candidate.Embedding) == len(queryVector) {
			score = 0.8*cosine(queryVector, candidate.Embedding) + 0.2*score
		}
		score *= 0.5 + 0.5*candidate.Confidence
		if score > 0 {
			scores = append(scores, scored{candidate, score})
		}
	}
	slices.SortStableFunc(scores, func(left, right scored) int {
		return cmp.Compare(right.score, left.score)
	})
	if len(scores) > limit {
		scores = scores[:limit]
	}
	result := make([]contract.Memory, len(scores))
	for i := range scores {
		result[i] = scores[i].memory
	}
	return result, nil
}

type ExplicitExtractor struct{ Embedder Embedder }

func (e *ExplicitExtractor) Extract(
	ctx context.Context,
	input memoryspi.ExtractInput,
) ([]contract.Memory, error) {
	if !input.Policy.Mode.CanWrite() {
		return nil, nil
	}
	var expiresAt *time.Time
	if input.Policy.TTL != "" {
		if ttl, err := parseTTL(input.Policy.TTL); err == nil {
			v := time.Now().UTC().Add(ttl)
			expiresAt = &v
		}
	}
	result := make([]contract.Memory, 0)
	for _, message := range input.Messages {
		if message.Role != contract.RoleUser {
			continue
		}
		content, ok := explicitContent(message.Content)
		if input.Policy.RequireExplicit && !ok {
			continue
		}
		if !ok {
			continue
		} // Safe default: an LLM extractor can implement implicit extraction.
		var embedding []float32
		var err error
		if e.Embedder != nil {
			embedding, err = e.Embedder.Embed(ctx, content)
			if err != nil {
				return nil, err
			}
		}
		now := time.Now().UTC()
		result = append(
			result,
			contract.Memory{
				ID:          idgen.New(),
				TenantID:    input.Run.TenantID,
				ProductID:   input.Run.ProductID,
				UserID:      input.Run.UserID,
				AgentKey:    input.Run.AgentKey,
				Category:    "explicit",
				Content:     content,
				Confidence:  1,
				Embedding:   embedding,
				SourceRunID: input.Run.ID,
				ExpiresAt:   expiresAt,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		)
	}
	return result, nil
}

type Service struct {
	Store     storagespi.MemoryStore
	Retriever memoryspi.Retriever
	Extractor memoryspi.Extractor
}

func NewService(store storagespi.MemoryStore, embedder Embedder) *Service {
	return &Service{
		Store:     store,
		Retriever: &Retriever{Store: store, Embedder: embedder},
		Extractor: &ExplicitExtractor{Embedder: embedder},
	}
}

func (s *Service) Recall(ctx context.Context, query memoryspi.Query) ([]contract.Memory, error) {
	if s == nil || s.Retriever == nil {
		return nil, nil
	}
	return s.Retriever.Retrieve(ctx, query)
}

func (s *Service) Remember(ctx context.Context, input memoryspi.ExtractInput) error {
	if s == nil || s.Extractor == nil || s.Store == nil {
		return nil
	}
	items, err := s.Extractor.Extract(ctx, input)
	if err != nil {
		return err
	}
	existing, err := s.Store.ListCandidates(
		ctx,
		input.Run.TenantID,
		input.Run.ProductID,
		input.Run.UserID,
		input.Run.AgentKey,
		1000,
	)
	if err != nil {
		return err
	}
	byContent := make(map[string]contract.Memory, len(existing))
	for _, item := range existing {
		byContent[normalize(item.Content)] = item
	}
	for _, item := range items {
		if old, ok := byContent[normalize(item.Content)]; ok {
			item.ID = old.ID
			item.CreatedAt = old.CreatedAt
		}
		if err = s.Store.Save(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func explicitContent(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	prefixes := []string{"记住：", "记住:", "请记住：", "请记住:", "remember:", "remember that "}
	lower := strings.ToLower(trimmed)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			v := strings.TrimSpace(trimmed[len(prefix):])
			return v, v != ""
		}
	}
	return "", false
}

func normalize(
	value string,
) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func tokens(value string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, field := range strings.FieldsFunc(strings.ToLower(value), isTokenSeparator) {
		if field != "" {
			result[field] = struct{}{}
		}
	}
	// Chinese text often has no whitespace. Bigrams provide a cheap portable fallback.
	runes := []rune(strings.ToLower(value))
	for i := 0; i+1 < len(runes); i++ {
		if unicode.Is(unicode.Han, runes[i]) && unicode.Is(unicode.Han, runes[i+1]) {
			result[string(runes[i:i+2])] = struct{}{}
		}
	}
	return result
}

func isTokenSeparator(value rune) bool {
	return unicode.IsSpace(value) || unicode.IsPunct(value)
}

func lexicalScore(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	hits := 0
	for k := range a {
		if _, ok := b[k]; ok {
			hits++
		}
	}
	return float64(hits) / math.Sqrt(float64(len(a)*len(b)))
}

func cosine(a, b []float32) float64 {
	var dot, aa, bb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		aa += x * x
		bb += y * y
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}

func parseTTL(value string) (time.Duration, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(value, "d"), 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	return time.ParseDuration(value)
}
