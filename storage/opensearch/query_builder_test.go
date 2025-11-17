package opensearch

import (
	"encoding/json"
	"testing"
	
	"github.com/nbd-wtf/go-nostr"
)

func TestBuildFilterQuery(t *testing.T) {
	storage := &Storage{index: "test-index"}
	
	tests := []struct {
		name   string
		filter nostr.Filter
		want   func(map[string]interface{}) bool
	}{
		{
			name: "empty filter matches all",
			filter: nostr.Filter{},
			want: func(query map[string]interface{}) bool {
				_, hasMatchAll := query["match_all"]
				return hasMatchAll
			},
		},
		{
			name: "filter by IDs",
			filter: nostr.Filter{
				IDs: []string{"id1", "id2"},
			},
			want: func(query map[string]interface{}) bool {
				boolQuery, ok := query["bool"].(map[string]interface{})
				if !ok {
					return false
				}
				must, ok := boolQuery["must"].([]map[string]interface{})
				if !ok || len(must) == 0 {
					return false
				}
				// Check for terms query on id
				for _, clause := range must {
					if terms, ok := clause["terms"].(map[string]interface{}); ok {
						if _, hasID := terms["id"]; hasID {
							return true
						}
					}
				}
				return false
			},
		},
		{
			name: "filter by authors",
			filter: nostr.Filter{
				Authors: []string{"pubkey1", "pubkey2"},
			},
			want: func(query map[string]interface{}) bool {
				boolQuery, ok := query["bool"].(map[string]interface{})
				if !ok {
					return false
				}
				must, ok := boolQuery["must"].([]map[string]interface{})
				if !ok || len(must) == 0 {
					return false
				}
				for _, clause := range must {
					if terms, ok := clause["terms"].(map[string]interface{}); ok {
						if _, hasPubkey := terms["pubkey"]; hasPubkey {
							return true
						}
					}
				}
				return false
			},
		},
		{
			name: "filter by kinds",
			filter: nostr.Filter{
				Kinds: []int{1, 3},
			},
			want: func(query map[string]interface{}) bool {
				boolQuery, ok := query["bool"].(map[string]interface{})
				if !ok {
					return false
				}
				must, ok := boolQuery["must"].([]map[string]interface{})
				if !ok || len(must) == 0 {
					return false
				}
				for _, clause := range must {
					if terms, ok := clause["terms"].(map[string]interface{}); ok {
						if _, hasKind := terms["kind"]; hasKind {
							return true
						}
					}
				}
				return false
			},
		},
		{
			name: "filter by common tag (e)",
			filter: nostr.Filter{
				Tags: map[string][]string{
					"e": {"event1", "event2"},
				},
			},
			want: func(query map[string]interface{}) bool {
				boolQuery, ok := query["bool"].(map[string]interface{})
				if !ok {
					return false
				}
				must, ok := boolQuery["must"].([]map[string]interface{})
				if !ok || len(must) == 0 {
					return false
				}
				// Should use flattened field tag_e
				for _, clause := range must {
					if terms, ok := clause["terms"].(map[string]interface{}); ok {
						if _, hasTagE := terms["tag_e"]; hasTagE {
							return true
						}
					}
				}
				return false
			},
		},
		{
			name: "filter by uncommon tag (i)",
			filter: nostr.Filter{
				Tags: map[string][]string{
					"i": {"github:user/repo"},
				},
			},
			want: func(query map[string]interface{}) bool {
				boolQuery, ok := query["bool"].(map[string]interface{})
				if !ok {
					return false
				}
				must, ok := boolQuery["must"].([]map[string]interface{})
				if !ok || len(must) == 0 {
					return false
				}
				// Should use nested query
				for _, clause := range must {
					if nested, ok := clause["nested"].(map[string]interface{}); ok {
						if path, ok := nested["path"].(string); ok && path == "tags_flat" {
							return true
						}
					}
				}
				return false
			},
		},
		{
			name: "filter by time range",
			filter: nostr.Filter{
				Since: func() *nostr.Timestamp { t := nostr.Timestamp(1234567890); return &t }(),
				Until: func() *nostr.Timestamp { t := nostr.Timestamp(1234567999); return &t }(),
			},
			want: func(query map[string]interface{}) bool {
				boolQuery, ok := query["bool"].(map[string]interface{})
				if !ok {
					return false
				}
				must, ok := boolQuery["must"].([]map[string]interface{})
				if !ok || len(must) == 0 {
					return false
				}
				for _, clause := range must {
					if rangeQuery, ok := clause["range"].(map[string]interface{}); ok {
						if _, hasCreatedAt := rangeQuery["created_at"]; hasCreatedAt {
							return true
						}
					}
				}
				return false
			},
		},
		{
			name: "filter with search",
			filter: nostr.Filter{
				Search: "bitcoin",
			},
			want: func(query map[string]interface{}) bool {
				boolQuery, ok := query["bool"].(map[string]interface{})
				if !ok {
					return false
				}
				must, ok := boolQuery["must"].([]map[string]interface{})
				if !ok || len(must) == 0 {
					return false
				}
				for _, clause := range must {
					if matchQuery, ok := clause["match"].(map[string]interface{}); ok {
						if _, hasContent := matchQuery["content"]; hasContent {
							return true
						}
					}
				}
				return false
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := storage.buildFilterQuery(tt.filter)
			
			// Debug: print the query
			queryJSON, _ := json.MarshalIndent(query, "", "  ")
			t.Logf("Query: %s", queryJSON)
			
			if !tt.want(query) {
				t.Errorf("Query validation failed")
			}
		})
	}
}

func TestBuildFilterQueryMultipleConditions(t *testing.T) {
	storage := &Storage{index: "test-index"}
	
	since := nostr.Timestamp(1234567890)
	filter := nostr.Filter{
		Authors: []string{"pubkey1"},
		Kinds:   []int{1},
		Tags: map[string][]string{
			"e": {"event1"},
			"p": {"user1"},
		},
		Since: &since,
	}
	
	query := storage.buildFilterQuery(filter)
	
	// Should have a bool query with multiple must clauses
	boolQuery, ok := query["bool"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected bool query")
	}
	
	must, ok := boolQuery["must"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected must clauses")
	}
	
	// Should have 5 clauses: authors, kinds, tag_e, tag_p, since
	if len(must) != 5 {
		t.Errorf("Expected 5 must clauses, got %d", len(must))
	}
	
	queryJSON, _ := json.MarshalIndent(query, "", "  ")
	t.Logf("Complex query: %s", queryJSON)
}

func TestTagStorageStrategy(t *testing.T) {
	// Test that common tags use flattened fields
	commonTags := []string{"e", "p", "a", "d", "t", "r", "g"}
	storage := &Storage{index: "test-index"}
	
	for _, tagName := range commonTags {
		filter := nostr.Filter{
			Tags: map[string][]string{
				tagName: {"value1"},
			},
		}
		
		query := storage.buildFilterQuery(filter)
		queryJSON, _ := json.Marshal(query)
		queryStr := string(queryJSON)
		
		// Should use tag_X field, not nested query
		if !contains(queryStr, "tag_"+tagName) {
			t.Errorf("Common tag %s should use flattened field tag_%s", tagName, tagName)
		}
		if contains(queryStr, "tags_flat") {
			t.Errorf("Common tag %s should not use nested tags_flat", tagName)
		}
	}
	
	// Test that uncommon tags use nested structure
	uncommonTags := []string{"i", "x", "custom"}
	for _, tagName := range uncommonTags {
		filter := nostr.Filter{
			Tags: map[string][]string{
				tagName: {"value1"},
			},
		}
		
		query := storage.buildFilterQuery(filter)
		queryJSON, _ := json.Marshal(query)
		queryStr := string(queryJSON)
		
		// Should use nested query on tags_flat
		if !contains(queryStr, "tags_flat") {
			t.Errorf("Uncommon tag %s should use nested tags_flat", tagName)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
