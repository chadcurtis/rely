package opensearch

import (
	"testing"
	
	"github.com/nbd-wtf/go-nostr"
)

func TestExtractTagValues(t *testing.T) {
	tags := nostr.Tags{
		{"e", "event1", "relay1"},
		{"e", "event2"},
		{"p", "pubkey1"},
		{"t", "bitcoin"},
		{"t", "nostr"},
	}
	
	eValues := extractTagValues(tags, "e")
	if len(eValues) != 2 {
		t.Errorf("Expected 2 e tag values, got %d", len(eValues))
	}
	if eValues[0] != "event1" || eValues[1] != "event2" {
		t.Errorf("Unexpected e tag values: %v", eValues)
	}
	
	pValues := extractTagValues(tags, "p")
	if len(pValues) != 1 {
		t.Errorf("Expected 1 p tag value, got %d", len(pValues))
	}
	if pValues[0] != "pubkey1" {
		t.Errorf("Unexpected p tag value: %s", pValues[0])
	}
	
	tValues := extractTagValues(tags, "t")
	if len(tValues) != 2 {
		t.Errorf("Expected 2 t tag values, got %d", len(tValues))
	}
}

func TestParseSearchExtensions(t *testing.T) {
	tests := []struct {
		input      string
		wantSort   string
		wantText   string
		wantMulti  bool
	}{
		{
			input:    "bitcoin sort:hot",
			wantSort: "hot",
			wantText: "bitcoin",
		},
		{
			input:    "sort:top",
			wantSort: "top",
			wantText: "",
		},
		{
			input:    "nostr sort:controversial",
			wantSort: "controversial",
			wantText: "nostr",
		},
		{
			input:    "sort:rising bitcoin",
			wantSort: "rising",
			wantText: "bitcoin",
		},
		{
			input:    "just text",
			wantSort: "",
			wantText: "just text",
		},
		{
			input:     "sort:hot sort:top",
			wantMulti: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ext := parseSearchExtensions(tt.input)
			
			if ext.hasMultipleSorts != tt.wantMulti {
				t.Errorf("hasMultipleSorts = %v, want %v", ext.hasMultipleSorts, tt.wantMulti)
			}
			
			if !tt.wantMulti {
				if ext.sortType != tt.wantSort {
					t.Errorf("sortType = %v, want %v", ext.sortType, tt.wantSort)
				}
				if ext.searchText != tt.wantText {
					t.Errorf("searchText = %v, want %v", ext.searchText, tt.wantText)
				}
			}
		})
	}
}

func TestEventDocumentStructure(t *testing.T) {
	// Test that we can create an event document
	event := &nostr.Event{
		ID:        "test-id",
		PubKey:    "test-pubkey",
		CreatedAt: 1234567890,
		Kind:      1,
		Tags: nostr.Tags{
			{"e", "ref1"},
			{"p", "user1"},
			{"t", "bitcoin"},
		},
		Content: "Hello Nostr",
		Sig:     "test-sig",
	}
	
	doc := eventDocument{
		ID:        event.ID,
		Pubkey:    event.PubKey,
		CreatedAt: int64(event.CreatedAt),
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
		Sig:       event.Sig,
		IndexedAt: 1234567890,
	}
	
	// Add flattened tags
	doc.TagE = extractTagValues(event.Tags, "e")
	doc.TagP = extractTagValues(event.Tags, "p")
	doc.TagT = extractTagValues(event.Tags, "t")
	
	if len(doc.TagE) != 1 || doc.TagE[0] != "ref1" {
		t.Errorf("TagE not set correctly: %v", doc.TagE)
	}
	if len(doc.TagP) != 1 || doc.TagP[0] != "user1" {
		t.Errorf("TagP not set correctly: %v", doc.TagP)
	}
	if len(doc.TagT) != 1 || doc.TagT[0] != "bitcoin" {
		t.Errorf("TagT not set correctly: %v", doc.TagT)
	}
}
