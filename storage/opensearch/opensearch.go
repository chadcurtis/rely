package opensearch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	opensearch "github.com/opensearch-project/opensearch-go/v2"
	opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
)

const (
	defaultIndex = "nostr-events"
)

// Config holds the configuration for OpenSearch connection
type Config struct {
	Addresses []string
	Username  string
	Password  string
	Index     string
	// InsecureSkipVerify skips TLS certificate verification (not recommended for production)
	InsecureSkipVerify bool
}

// Storage implements Nostr event storage using OpenSearch
type Storage struct {
	client *opensearch.Client
	index  string
}

// extractTagValues extracts all values for a specific tag name
func extractTagValues(tags nostr.Tags, tagName string) []string {
	values := make([]string, 0)
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == tagName {
			values = append(values, tag[1])
		}
	}
	return values
}

// searchExtensions holds parsed NIP-50 search extensions
type searchExtensions struct {
	sortType          string // "hot", "top", "controversial", "rising"
	searchText        string
	hasMultipleSorts  bool
}

// parseSearchExtensions parses NIP-50 search extensions from search string
func parseSearchExtensions(search string) searchExtensions {
	// Simple regex to extract sort:value patterns
	sortRegex := regexp.MustCompile(`sort:(\w+)`)
	matches := sortRegex.FindAllStringSubmatch(search, -1)
	
	if len(matches) > 1 {
		return searchExtensions{hasMultipleSorts: true}
	}
	
	var sortType string
	if len(matches) == 1 {
		value := strings.ToLower(matches[0][1])
		if value == "hot" || value == "top" || value == "controversial" || value == "rising" {
			sortType = value
		}
	}
	
	// Remove sort: patterns to get clean search text
	searchText := sortRegex.ReplaceAllString(search, "")
	searchText = strings.TrimSpace(searchText)
	
	return searchExtensions{
		sortType:   sortType,
		searchText: searchText,
	}
}

// NewStorage creates a new OpenSearch storage instance
func NewStorage(cfg Config) (*Storage, error) {
	if len(cfg.Addresses) == 0 {
		return nil, fmt.Errorf("at least one OpenSearch address is required")
	}

	if cfg.Index == "" {
		cfg.Index = defaultIndex
	}

	// Configure OpenSearch client
	clientCfg := opensearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	}

	// Configure TLS if needed
	if cfg.InsecureSkipVerify {
		clientCfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	client, err := opensearch.NewClient(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}

	storage := &Storage{
		client: client,
		index:  cfg.Index,
	}

	// Test connection
	res, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to OpenSearch: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("OpenSearch returned error: %s", res.String())
	}

	// Create index if it doesn't exist
	if err := storage.createIndex(); err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	return storage, nil
}

// createIndex creates the index with appropriate mappings if it doesn't exist
func (s *Storage) createIndex() error {
	// Check if index exists
	req := opensearchapi.IndicesExistsRequest{
		Index: []string{s.index},
	}

	res, err := req.Do(context.Background(), s.client)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Index already exists
	if res.StatusCode == 200 {
		return nil
	}

	// Create index with mappings matching otherstuff-relay structure
	mapping := `{
		"settings": {
			"number_of_shards": 3,
			"number_of_replicas": 1,
			"refresh_interval": "5s",
			"index.mapping.total_fields.limit": 2000,
			"analysis": {
				"analyzer": {
					"nostr_content_analyzer": {
						"type": "standard",
						"stopwords": "_none_"
					}
				}
			}
		},
		"mappings": {
			"dynamic": "strict",
			"properties": {
				"id": { "type": "keyword" },
				"pubkey": { "type": "keyword" },
				"created_at": { "type": "long" },
				"kind": { "type": "integer" },
				"content": {
					"type": "text",
					"analyzer": "nostr_content_analyzer",
					"fields": {
						"keyword": {
							"type": "keyword",
							"ignore_above": 256
						}
					}
				},
				"sig": { "type": "keyword" },
				"tags": { "type": "keyword" },
				"indexed_at": { "type": "long" },
				"tag_e": { "type": "keyword" },
				"tag_p": { "type": "keyword" },
				"tag_a": { "type": "keyword" },
				"tag_d": { "type": "keyword" },
				"tag_t": { "type": "keyword" },
				"tag_r": { "type": "keyword" },
				"tag_g": { "type": "keyword" },
				"tags_flat": {
					"type": "nested",
					"properties": {
						"name": { "type": "keyword" },
						"value": { "type": "keyword" }
					}
				}
			}
		}
	}`

	createReq := opensearchapi.IndicesCreateRequest{
		Index: s.index,
		Body:  strings.NewReader(mapping),
	}

	createRes, err := createReq.Do(context.Background(), s.client)
	if err != nil {
		return err
	}
	defer createRes.Body.Close()

	if createRes.IsError() {
		body, _ := io.ReadAll(createRes.Body)
		return fmt.Errorf("failed to create index: %s", string(body))
	}

	return nil
}

// eventDocument represents how we store events in OpenSearch
type eventDocument struct {
	ID        string       `json:"id"`
	Pubkey    string       `json:"pubkey"`
	CreatedAt int64        `json:"created_at"`
	Kind      int          `json:"kind"`
	Tags      nostr.Tags   `json:"tags"` // Store full tag arrays
	Content   string       `json:"content"`
	Sig       string       `json:"sig"`
	IndexedAt int64        `json:"indexed_at"`
	
	// Flattened tag fields for fast filtering (common tags)
	TagE []string `json:"tag_e,omitempty"`
	TagP []string `json:"tag_p,omitempty"`
	TagA []string `json:"tag_a,omitempty"`
	TagD []string `json:"tag_d,omitempty"`
	TagT []string `json:"tag_t,omitempty"`
	TagR []string `json:"tag_r,omitempty"`
	TagG []string `json:"tag_g,omitempty"`
	
	// Generic tag storage for all other tags
	TagsFlat []tagFlat `json:"tags_flat,omitempty"`
}

type tagFlat struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SaveEvent stores a Nostr event in OpenSearch
func (s *Storage) SaveEvent(ctx context.Context, event *nostr.Event) error {
	doc := eventDocument{
		ID:        event.ID,
		Pubkey:    event.PubKey,
		CreatedAt: int64(event.CreatedAt),
		Kind:      event.Kind,
		Tags:      event.Tags,
		Content:   event.Content,
		Sig:       event.Sig,
		IndexedAt: time.Now().Unix(),
	}

	// Index common tags for fast filtering
	commonTags := map[string]bool{"e": true, "p": true, "a": true, "d": true, "t": true, "r": true, "g": true}
	
	if eValues := extractTagValues(event.Tags, "e"); len(eValues) > 0 {
		doc.TagE = eValues
	}
	if pValues := extractTagValues(event.Tags, "p"); len(pValues) > 0 {
		doc.TagP = pValues
	}
	if aValues := extractTagValues(event.Tags, "a"); len(aValues) > 0 {
		doc.TagA = aValues
	}
	if dValues := extractTagValues(event.Tags, "d"); len(dValues) > 0 {
		doc.TagD = dValues
	}
	if tValues := extractTagValues(event.Tags, "t"); len(tValues) > 0 {
		doc.TagT = tValues
	}
	if rValues := extractTagValues(event.Tags, "r"); len(rValues) > 0 {
		doc.TagR = rValues
	}
	if gValues := extractTagValues(event.Tags, "g"); len(gValues) > 0 {
		doc.TagG = gValues
	}

	// Index all other tags in a flattened structure for generic queries
	otherTags := make([]tagFlat, 0)
	for _, tag := range event.Tags {
		if len(tag) >= 2 && !commonTags[tag[0]] {
			otherTags = append(otherTags, tagFlat{
				Name:  tag[0],
				Value: tag[1],
			})
		}
	}
	if len(otherTags) > 0 {
		doc.TagsFlat = otherTags
	}

	// Marshal document
	docBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Index the document
	req := opensearchapi.IndexRequest{
		Index:      s.index,
		DocumentID: event.ID,
		Body:       strings.NewReader(string(docBytes)),
		Refresh:    "false", // async refresh for better performance
	}

	res, err := req.Do(ctx, s.client)
	if err != nil {
		return fmt.Errorf("failed to index event: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("OpenSearch error: %s", string(body))
	}

	return nil
}

// QueryEvents retrieves events matching the given filters
func (s *Storage) QueryEvents(ctx context.Context, filters nostr.Filters) ([]nostr.Event, error) {
	// Process each filter and combine results
	allEvents := make([]nostr.Event, 0)
	
	for _, filter := range filters {
		events, err := s.queryFilter(ctx, filter)
		if err != nil {
			return nil, err
		}
		allEvents = append(allEvents, events...)
	}
	
	return allEvents, nil
}

// queryFilter processes a single filter
func (s *Storage) queryFilter(ctx context.Context, filter nostr.Filter) ([]nostr.Event, error) {
	// If limit is 0, skip the query (realtime-only subscription)
	if filter.Limit == 0 {
		return []nostr.Event{}, nil
	}
	
	// Parse search extensions if present
	var sortType string
	var hasFullTextSearch bool
	
	if filter.Search != "" {
		ext := parseSearchExtensions(filter.Search)
		
		// If multiple sort tokens exist, return 0 events
		if ext.hasMultipleSorts {
			return []nostr.Event{}, nil
		}
		
		sortType = ext.sortType
		hasFullTextSearch = len(ext.searchText) > 0
		
		// Update filter with cleaned search text
		filter.Search = ext.searchText
	}
	
	// Default to 500, cap at 5000
	limit := 500
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	if limit > 5000 {
		limit = 5000
	}
	
	// For sort extensions, use aggregation-based queries
	if sortType != "" {
		return s.querySorted(ctx, filter, sortType, limit)
	}
	
	query := s.buildFilterQuery(filter)

	// For NIP-50 search queries with text, sort by relevance score first, then by created_at
	// For regular queries, sort by created_at only (newest first)
	sort := []map[string]interface{}{
		{"created_at": map[string]interface{}{"order": "desc"}},
	}
	if hasFullTextSearch {
		sort = []map[string]interface{}{
			{"_score": map[string]interface{}{"order": "desc"}},
			{"created_at": map[string]interface{}{"order": "desc"}},
		}
	}

	queryBody := map[string]interface{}{
		"query": query,
		"sort":  sort,
		"size":  limit,
		"_source": []string{"id", "pubkey", "created_at", "kind", "tags", "content", "sig"},
	}

	queryBytes, err := json.Marshal(queryBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req := opensearchapi.SearchRequest{
		Index: []string{s.index},
		Body:  strings.NewReader(string(queryBytes)),
	}

	res, err := req.Do(ctx, s.client)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("search error: %s", string(body))
	}

	// Parse response
	var searchRes struct {
		Hits struct {
			Hits []struct {
				Source eventDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&searchRes); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to Nostr events
	events := make([]nostr.Event, 0, len(searchRes.Hits.Hits))
	for _, hit := range searchRes.Hits.Hits {
		doc := hit.Source
		
		event := nostr.Event{
			ID:        doc.ID,
			PubKey:    doc.Pubkey,
			CreatedAt: nostr.Timestamp(doc.CreatedAt),
			Kind:      doc.Kind,
			Tags:      doc.Tags, // Tags are stored as full arrays
			Content:   doc.Content,
			Sig:       doc.Sig,
		}
		events = append(events, event)
	}

	return events, nil
}

// buildFilterQuery constructs an OpenSearch query from a single Nostr filter
func (s *Storage) buildFilterQuery(filter nostr.Filter) map[string]interface{} {
	mustClauses := make([]map[string]interface{}, 0)

	// IDs filter
	if len(filter.IDs) > 0 {
		mustClauses = append(mustClauses, map[string]interface{}{
			"terms": map[string]interface{}{
				"id": filter.IDs,
			},
		})
	}

	// Authors filter
	if len(filter.Authors) > 0 {
		mustClauses = append(mustClauses, map[string]interface{}{
			"terms": map[string]interface{}{
				"pubkey": filter.Authors,
			},
		})
	}

	// Kinds filter
	if len(filter.Kinds) > 0 {
		mustClauses = append(mustClauses, map[string]interface{}{
			"terms": map[string]interface{}{
				"kind": filter.Kinds,
			},
		})
	}

	// Tags filter - use flattened tag fields for common tags, nested for others
	commonTags := map[string]bool{"e": true, "p": true, "a": true, "d": true, "t": true, "r": true, "g": true}
	for tagName, tagValues := range filter.Tags {
		if len(tagValues) > 0 {
			if commonTags[tagName] {
				// Use optimized flattened field for common tags
				mustClauses = append(mustClauses, map[string]interface{}{
					"terms": map[string]interface{}{
						"tag_" + tagName: tagValues,
					},
				})
			} else {
				// Use nested query for other tags
				mustClauses = append(mustClauses, map[string]interface{}{
					"nested": map[string]interface{}{
						"path": "tags_flat",
						"query": map[string]interface{}{
							"bool": map[string]interface{}{
								"must": []map[string]interface{}{
									{
										"term": map[string]interface{}{
											"tags_flat.name": tagName,
										},
									},
									{
										"terms": map[string]interface{}{
											"tags_flat.value": tagValues,
										},
									},
								},
							},
						},
					},
				})
			}
		}
	}

	// Time range filter
	if filter.Since != nil || filter.Until != nil {
		rangeFilter := make(map[string]interface{})
		if filter.Since != nil {
			rangeFilter["gte"] = int64(*filter.Since)
		}
		if filter.Until != nil {
			rangeFilter["lte"] = int64(*filter.Until)
		}
		mustClauses = append(mustClauses, map[string]interface{}{
			"range": map[string]interface{}{
				"created_at": rangeFilter,
			},
		})
	}

	// Search filter (if content search is needed)
	if filter.Search != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"match": map[string]interface{}{
				"content": filter.Search,
			},
		})
	}

	// Return the query
	if len(mustClauses) > 0 {
		return map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustClauses,
			},
		}
	}
	
	// Empty filter matches all
	return map[string]interface{}{
		"match_all": map[string]interface{}{},
	}
}

// querySorted handles special sorting modes (hot, top, controversial, rising)
func (s *Storage) querySorted(ctx context.Context, filter nostr.Filter, sortType string, limit int) ([]nostr.Event, error) {
	// Build base query from filter
	baseQuery := s.buildFilterQuery(filter)
	
	switch sortType {
	case "top":
		return s.queryTop(ctx, baseQuery, filter, limit)
	case "hot":
		return s.queryHot(ctx, baseQuery, filter, limit)
	default:
		// For unsupported sort types, fall back to regular query
		return s.queryFilter(ctx, filter)
	}
}

// queryTop returns events with most references (e-tags pointing to them)
func (s *Storage) queryTop(ctx context.Context, baseQuery map[string]interface{}, filter nostr.Filter, limit int) ([]nostr.Event, error) {
	// Get events that are referenced by others via e-tags
	// This is a simplified version - in production you'd use aggregations
	
	// Just return regular sorted results for now
	// A full implementation would aggregate on tag_e to find most referenced events
	return s.queryFilter(ctx, filter)
}

// queryHot returns trending events (recent + popular)
func (s *Storage) queryHot(ctx context.Context, baseQuery map[string]interface{}, filter nostr.Filter, limit int) ([]nostr.Event, error) {
	// Hot = recent events with engagement
	// Simplified: just return recent events
	// A full implementation would calculate hot score based on age and engagement
	
	return s.queryFilter(ctx, filter)
}

// CountEvents returns the count of events matching the filters
func (s *Storage) CountEvents(ctx context.Context, filters nostr.Filters) (int64, error) {
	// Count events for each filter and sum them
	totalCount := int64(0)
	
	for _, filter := range filters {
		query := s.buildFilterQuery(filter)
		
		queryBody := map[string]interface{}{
			"query": query,
		}

		queryBytes, err := json.Marshal(queryBody)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal query: %w", err)
		}

		req := opensearchapi.CountRequest{
			Index: []string{s.index},
			Body:  strings.NewReader(string(queryBytes)),
		}

		res, err := req.Do(ctx, s.client)
		if err != nil {
			return 0, fmt.Errorf("count failed: %w", err)
		}

		if res.IsError() {
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			return 0, fmt.Errorf("count error: %s", string(body))
		}

		var countRes struct {
			Count int64 `json:"count"`
		}

		if err := json.NewDecoder(res.Body).Decode(&countRes); err != nil {
			res.Body.Close()
			return 0, fmt.Errorf("failed to decode count response: %w", err)
		}
		res.Body.Close()

		totalCount += countRes.Count
	}

	return totalCount, nil
}

// DeleteEvent removes an event from OpenSearch
func (s *Storage) DeleteEvent(ctx context.Context, eventID string) error {
	req := opensearchapi.DeleteRequest{
		Index:      s.index,
		DocumentID: eventID,
	}

	res, err := req.Do(ctx, s.client)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() && res.StatusCode != 404 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("delete error: %s", string(body))
	}

	return nil
}

// Close closes the OpenSearch client
func (s *Storage) Close() error {
	// OpenSearch client doesn't have a Close method
	// Connection pooling is handled automatically
	return nil
}
