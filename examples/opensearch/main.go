package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pippellia-btc/rely"
	"github.com/pippellia-btc/rely/storage/opensearch"
)

/*
Example of a relay using OpenSearch as the storage layer.
Configuration is loaded from a .env file.

Before running, ensure:
1. OpenSearch is running and accessible
2. Copy .env.example to .env and configure it
3. Run: go run examples/opensearch/main.go
*/

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Parse OpenSearch configuration from environment
	config := opensearch.Config{
		Addresses: parseAddresses(getEnv("OPENSEARCH_ADDRESSES", "https://localhost:9200")),
		Username:  getEnv("OPENSEARCH_USERNAME", "admin"),
		Password:  getEnv("OPENSEARCH_PASSWORD", "admin"),
		Index:     getEnv("OPENSEARCH_INDEX", "nostr-events"),
		InsecureSkipVerify: parseBool(getEnv("OPENSEARCH_INSECURE_SKIP_VERIFY", "false")),
	}

	// Create OpenSearch storage
	storage, err := opensearch.NewStorage(config)
	if err != nil {
		log.Fatalf("Failed to initialize OpenSearch storage: %v", err)
	}
	defer storage.Close()

	log.Println("Successfully connected to OpenSearch")

	// Setup relay
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	relayDomain := getEnv("RELAY_DOMAIN", "localhost")
	relay := rely.NewRelay(
		rely.WithDomain(relayDomain),
	)

	// Hook up storage to relay events
	relay.On.Event = func(c rely.Client, e *nostr.Event) error {
		if err := storage.SaveEvent(context.Background(), e); err != nil {
			log.Printf("Failed to save event %s: %v", e.ID, err)
			return err
		}
		log.Printf("Saved event %s (kind=%d) from %s", e.ID, e.Kind, e.PubKey[:8])
		return nil
	}

	relay.On.Req = func(ctx context.Context, c rely.Client, filters nostr.Filters) ([]nostr.Event, error) {
		events, err := storage.QueryEvents(ctx, filters)
		if err != nil {
			log.Printf("Failed to query events: %v", err)
			return nil, err
		}
		log.Printf("Query returned %d events for client %s", len(events), c.IP())
		return events, nil
	}

	// Optional: Support NIP-45 COUNT
	relay.On.Count = func(c rely.Client, filters nostr.Filters) (int64, bool, error) {
		count, err := storage.CountEvents(context.Background(), filters)
		if err != nil {
			log.Printf("Failed to count events: %v", err)
			return 0, false, err
		}
		log.Printf("Count query returned %d events for client %s", count, c.IP())
		return count, false, nil
	}

	// Start the relay
	host := getEnv("RELAY_HOST", "localhost")
	port := getEnv("RELAY_PORT", "3334")
	address := fmt.Sprintf("%s:%s", host, port)

	log.Printf("Starting relay on %s", address)
	if err := relay.StartAndServe(ctx, address); err != nil {
		log.Fatalf("Relay error: %v", err)
	}
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseAddresses(addresses string) []string {
	parts := strings.Split(addresses, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseBool(value string) bool {
	b, _ := strconv.ParseBool(value)
	return b
}
