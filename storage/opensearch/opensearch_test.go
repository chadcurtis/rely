package opensearch

import (
	"testing"
)

func TestConfig(t *testing.T) {
	config := Config{
		Addresses: []string{"http://localhost:9200"},
		Index:     "test-index",
	}

	if len(config.Addresses) != 1 {
		t.Errorf("Expected 1 address, got %d", len(config.Addresses))
	}

	if config.Index != "test-index" {
		t.Errorf("Expected index 'test-index', got '%s'", config.Index)
	}
}

func TestDefaultIndex(t *testing.T) {
	config := Config{
		Addresses: []string{"http://localhost:9200"},
	}

	if config.Index == "" {
		config.Index = defaultIndex
	}

	if config.Index != defaultIndex {
		t.Errorf("Expected default index '%s', got '%s'", defaultIndex, config.Index)
	}
}
