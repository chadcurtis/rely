# OpenSearch Storage for Rely

This package provides OpenSearch integration for the rely Nostr relay framework.

## Features

- **Event Storage**: Efficiently store Nostr events in OpenSearch
- **Advanced Querying**: Leverage OpenSearch's powerful query capabilities for filtering events
- **Tag Indexing**: Nested tag structure for efficient tag-based queries
- **NIP-45 Support**: Built-in COUNT query support
- **Time-based Queries**: Optimized time range filtering
- **Full-text Search**: Content search capabilities using OpenSearch's text analysis

## Installation

```bash
go get github.com/opensearch-project/opensearch-go/v2
go get github.com/joho/godotenv
```

## Configuration

### Environment Variables

Create a `.env` file in your project root:

```env
OPENSEARCH_ADDRESSES=https://localhost:9200
OPENSEARCH_USERNAME=admin
OPENSEARCH_PASSWORD=admin
OPENSEARCH_INDEX=nostr-events
OPENSEARCH_INSECURE_SKIP_VERIFY=false
```

### Multiple Addresses

For clustered OpenSearch deployments, provide comma-separated addresses:

```env
OPENSEARCH_ADDRESSES=https://node1:9200,https://node2:9200,https://node3:9200
```

## Usage

```go
package main

import (
    "context"
    "log"
    
    "github.com/pippellia-btc/rely"
    "github.com/pippellia-btc/rely/storage/opensearch"
)

func main() {
    // Configure OpenSearch
    config := opensearch.Config{
        Addresses: []string{"https://localhost:9200"},
        Username:  "admin",
        Password:  "admin",
        Index:     "nostr-events",
    }
    
    // Create storage
    storage, err := opensearch.NewStorage(config)
    if err != nil {
        log.Fatal(err)
    }
    defer storage.Close()
    
    // Create relay
    relay := rely.NewRelay()
    
    // Hook up storage
    relay.On.Event = func(c rely.Client, e *nostr.Event) error {
        return storage.SaveEvent(context.Background(), e)
    }
    
    relay.On.Req = func(ctx context.Context, c rely.Client, f nostr.Filters) ([]nostr.Event, error) {
        return storage.QueryEvents(ctx, f)
    }
    
    relay.On.Count = func(c rely.Client, f nostr.Filters) (int64, bool, error) {
        count, err := storage.CountEvents(context.Background(), f)
        return count, false, err
    }
    
    // Start relay
    relay.StartAndServe(context.Background(), "localhost:3334")
}
```

## Index Structure

The package automatically creates an index with optimized mappings:

```json
{
  "settings": {
    "number_of_shards": 3,
    "number_of_replicas": 1,
    "refresh_interval": "5s",
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
    "properties": {
      "id": { "type": "keyword" },
      "pubkey": { "type": "keyword" },
      "created_at": { "type": "long" },
      "kind": { "type": "integer" },
      "content": {
        "type": "text",
        "analyzer": "nostr_content_analyzer",
        "fields": {
          "keyword": { "type": "keyword", "ignore_above": 256 }
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
}
```

### Tag Storage Strategy

For optimal performance, tags are stored in two ways:

1. **Flattened fields** for common tags (e, p, a, d, t, r, g):
   - Direct keyword fields like `tag_e`, `tag_p`, etc.
   - Fastest query performance
   - Used for most common tag queries

2. **Nested structure** for all other tags:
   - Stored in `tags_flat` as nested documents
   - Accurate matching for uncommon tags
   - Slightly slower but still efficient

3. **Full tag arrays** in `tags` field:
   - Complete tag data preserved
   - Used for event reconstruction

## Supported Filters

The storage layer supports all standard Nostr filter types:

- **IDs**: Filter by event IDs
- **Authors**: Filter by public keys
- **Kinds**: Filter by event kinds
- **Tags**: Filter by tag names and values
  - Common tags (e, p, a, d, t, r, g) use optimized flattened fields
  - Other tags use nested queries for accurate matching
- **Since/Until**: Time range filtering
- **Limit**: Result pagination (default 500, max 5000)
- **Search**: Full-text content search (NIP-50)
  - Basic text search
  - Sort extensions: `sort:hot`, `sort:top`, `sort:controversial`, `sort:rising`

### NIP-50 Search Extensions

The storage supports NIP-50 search extensions for advanced sorting:

```
# Search with trending sort
search: "bitcoin sort:hot"

# Search with top posts
search: "nostr sort:top"

# Just sort without text search
search: "sort:rising"
```

Supported sort types:
- **hot**: Trending events (recent + popular)
- **top**: Most referenced events
- **controversial**: Events with mixed engagement
- **rising**: Recently trending events

## Performance Considerations

### Indexing

- Events are indexed asynchronously (`refresh: false`) for better write performance
- The default refresh interval is 1 second (configurable in index settings)

### Sharding

- Default: 3 primary shards, 1 replica
- Adjust based on your cluster size and data volume

### Querying

- Queries are optimized with proper field types (keyword vs text)
- Nested queries for tags ensure accurate matching
- Time-based sorting uses the indexed `created_at` field

## Production Recommendations

1. **TLS**: Always use HTTPS in production and set `InsecureSkipVerify: false`
2. **Authentication**: Use strong credentials and rotate them regularly
3. **Index Lifecycle**: Implement ILM policies for old events
4. **Monitoring**: Monitor OpenSearch cluster health and query performance
5. **Backup**: Regular snapshots of the index
6. **Scaling**: Use multiple shards for horizontal scaling

## Docker Compose Example

```yaml
version: '3'
services:
  opensearch:
    image: opensearchproject/opensearch:2.11.0
    environment:
      - discovery.type=single-node
      - OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m
      - OPENSEARCH_INITIAL_ADMIN_PASSWORD=Admin123!
    ports:
      - "9200:9200"
    volumes:
      - opensearch-data:/usr/share/opensearch/data

  relay:
    build: .
    environment:
      - OPENSEARCH_ADDRESSES=https://opensearch:9200
      - OPENSEARCH_USERNAME=admin
      - OPENSEARCH_PASSWORD=Admin123!
      - OPENSEARCH_INSECURE_SKIP_VERIFY=true
    ports:
      - "3334:3334"
    depends_on:
      - opensearch

volumes:
  opensearch-data:
```

## Troubleshooting

### Connection Issues

If you get certificate errors with self-signed certificates:
```go
config.InsecureSkipVerify = true  // Development only!
```

### Index Already Exists

The package automatically handles existing indexes. If you need to recreate:
```bash
curl -X DELETE https://localhost:9200/nostr-events -u admin:admin
```

### Performance Tuning

For high-throughput scenarios:
- Increase `number_of_shards`
- Adjust `refresh_interval` to a higher value
- Use bulk indexing for batch operations
- Enable index caching

## License

Same as the rely project.
