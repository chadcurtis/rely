# OpenSearch Storage - Data Flow Documentation

This document explains how data flows through the OpenSearch storage layer.

## Event Storage Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. CLIENT SENDS EVENT                                           │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ ["EVENT", {...}]
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. RELAY RECEIVES & VALIDATES                                   │
│    - Parse WebSocket message                                    │
│    - Validate event ID and signature                            │
│    - Apply Reject.Event hooks                                   │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ *nostr.Event
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. ON.EVENT HOOK (storage.SaveEvent)                            │
│    - Convert tags to nested structure                           │
│    - Add indexed_at timestamp                                   │
│    - Marshal to JSON                                            │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ JSON document
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. OPENSEARCH INDEX REQUEST                                     │
│    PUT /nostr-events/_doc/{event.ID}                            │
│    {                                                             │
│      "id": "abc123...",                                          │
│      "pubkey": "def456...",                                      │
│      "created_at": 1234567890,                                   │
│      "kind": 1,                                                  │
│      "tags": [                                                   │
│        {"name": "e", "values": ["ref1"]},                        │
│        {"name": "p", "values": ["user1"]}                        │
│      ],                                                          │
│      "content": "Hello Nostr!",                                  │
│      "sig": "sig123...",                                         │
│      "indexed_at": "2024-01-01T00:00:00Z"                        │
│    }                                                             │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ HTTP POST
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. OPENSEARCH PROCESSING                                        │
│    - Analyze text fields                                        │
│    - Index keyword fields                                       │
│    - Store nested tags                                          │
│    - Update inverted indexes                                    │
│    - Replicate to replica shards (if configured)                │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ Success/Error
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 6. RELAY RESPONDS TO CLIENT                                     │
│    ["OK", "abc123...", true, ""]                                │
└─────────────────────────────────────────────────────────────────┘
```

## Query Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. CLIENT SENDS REQ                                             │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ ["REQ", "sub-id", {...filters}]
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. RELAY RECEIVES & VALIDATES                                   │
│    - Parse WebSocket message                                    │
│    - Apply Reject.Req hooks                                     │
│    - Calculate available response budget                        │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ nostr.Filters
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. ON.REQ HOOK (storage.QueryEvents)                            │
│    - Build OpenSearch query from filters                        │
│    - Handle IDs, authors, kinds, tags, time ranges              │
│    - Apply limits                                               │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ OpenSearch Query DSL
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. OPENSEARCH QUERY                                             │
│    POST /nostr-events/_search                                   │
│    {                                                             │
│      "query": {                                                  │
│        "bool": {                                                 │
│          "should": [                                             │
│            {                                                     │
│              "bool": {                                           │
│                "must": [                                         │
│                  {"terms": {"kind": [1]}},                       │
│                  {"terms": {"pubkey": ["abc..."]}},              │
│                  {"range": {"created_at": {"gte": 123456}}}     │
│                ]                                                 │
│              }                                                   │
│            }                                                     │
│          ]                                                       │
│        }                                                         │
│      },                                                          │
│      "size": 100,                                                │
│      "sort": [{"created_at": {"order": "desc"}}]                │
│    }                                                             │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ HTTP POST
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. OPENSEARCH QUERY EXECUTION                                   │
│    - Query planner selects optimal shards                       │
│    - Execute query on relevant shards                           │
│    - Merge results from multiple shards                         │
│    - Apply sorting and limits                                   │
│    - Return matching documents                                  │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ Search Results
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 6. STORAGE CONVERTS TO NOSTR EVENTS                             │
│    - Parse OpenSearch response                                  │
│    - Convert documents to nostr.Event                           │
│    - Reconstruct tags from nested structure                     │
│    - Return []nostr.Event                                       │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 │ []nostr.Event
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 7. RELAY SENDS EVENTS TO CLIENT                                 │
│    ["EVENT", "sub-id", {...event1}]                             │
│    ["EVENT", "sub-id", {...event2}]                             │
│    ...                                                           │
│    ["EOSE", "sub-id"]                                            │
└─────────────────────────────────────────────────────────────────┘
```

## Tag Query Flow (Nested Query)

```
Filter: {"#e": ["event-id-123"]}

                 │
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ BUILD NESTED QUERY                                              │
│    {                                                             │
│      "nested": {                                                 │
│        "path": "tags",                                           │
│        "query": {                                                │
│          "bool": {                                               │
│            "must": [                                             │
│              {"term": {"tags.name": "e"}},                       │
│              {"terms": {"tags.values": ["event-id-123"]}}        │
│            ]                                                     │
│          }                                                       │
│        }                                                         │
│      }                                                           │
│    }                                                             │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ OPENSEARCH NESTED QUERY EXECUTION                               │
│    - Enter nested document context                              │
│    - Match tag.name = "e"                                        │
│    - Match "event-id-123" in tag.values                          │
│    - Return parent documents where both match                   │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
                Results
```

## Count Query Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. CLIENT SENDS COUNT                                           │
│    ["COUNT", "sub-id", {...filters}]                            │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. ON.COUNT HOOK (storage.CountEvents)                          │
│    - Build same query as QueryEvents                            │
│    - Remove size and sort                                       │
│    - Use COUNT API instead of SEARCH                            │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. OPENSEARCH COUNT REQUEST                                     │
│    POST /nostr-events/_count                                    │
│    { "query": {...} }                                            │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. OPENSEARCH RETURNS COUNT                                     │
│    { "count": 42 }                                               │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. RELAY SENDS COUNT TO CLIENT                                  │
│    ["COUNT", "sub-id", {"count": 42}]                           │
└─────────────────────────────────────────────────────────────────┘
```

## Index Structure

```
nostr-events (index)
│
├── Shard 0
│   ├── Segment 1
│   │   ├── Inverted Index (id)
│   │   ├── Inverted Index (pubkey)
│   │   ├── Inverted Index (kind)
│   │   ├── Inverted Index (content - analyzed)
│   │   ├── Doc Values (created_at)
│   │   └── Nested Documents (tags)
│   │
│   └── Segment 2
│       └── ...
│
├── Shard 1
│   └── ...
│
└── Shard 2
    └── ...

Replicas (if configured):
├── Replica of Shard 0
├── Replica of Shard 1
└── Replica of Shard 2
```

## Query Optimization Path

```
Query: Get kind=1 events from author "abc..." since timestamp 123456

┌─────────────────────────────────────────────────────────────────┐
│ 1. QUERY PLANNER                                                │
│    - Analyze filter conditions                                  │
│    - Identify which shards might contain data                   │
│    - Determine optimal execution order                          │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. SHARD-LEVEL EXECUTION                                        │
│    - Use inverted index for kind=1 (fast)                       │
│    - Use inverted index for pubkey="abc..." (fast)              │
│    - Use doc values for created_at >= 123456 (fast)             │
│    - Intersect posting lists (very fast)                        │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. RESULT AGGREGATION                                           │
│    - Collect results from all shards                            │
│    - Merge sorted results                                       │
│    - Apply global limit                                         │
│    - Return top documents                                       │
└─────────────────────────────────────────────────────────────────┘
```

## Error Handling Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ OPERATION (Save/Query/Count)                                    │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
         ┌───────────────┐
         │  Try Execute  │
         └───────┬───────┘
                 │
        ┌────────┴────────┐
        │                 │
        ▼                 ▼
   ┌─────────┐      ┌──────────┐
   │ Success │      │  Error   │
   └────┬────┘      └─────┬────┘
        │                 │
        │                 ▼
        │         ┌──────────────────┐
        │         │ Check Error Type │
        │         └────────┬─────────┘
        │                  │
        │         ┌────────┴────────┐
        │         │                 │
        │         ▼                 ▼
        │    ┌─────────┐      ┌──────────┐
        │    │Network  │      │ OpenSearch│
        │    │ Error   │      │  Error    │
        │    └────┬────┘      └─────┬─────┘
        │         │                 │
        │         ▼                 ▼
        │    ┌──────────────────────────┐
        │    │   Log Error Details      │
        │    └────────┬─────────────────┘
        │             │
        │             ▼
        │    ┌──────────────────────────┐
        │    │ Return Formatted Error   │
        │    └────────┬─────────────────┘
        │             │
        └─────────────┴─────────────────┐
                      │
                      ▼
         ┌────────────────────────────┐
         │  Relay Handles Response    │
         │  - Send OK/NOTICE to client│
         │  - Log if needed           │
         └────────────────────────────┘
```

## Performance Characteristics

### Write Path
- **Latency**: 1-10ms per event (async refresh)
- **Throughput**: 1,000-5,000 events/sec
- **Bottleneck**: Network I/O, disk I/O

### Read Path
- **Latency**: 10-100ms for typical queries
- **Throughput**: 100-1,000 queries/sec
- **Bottleneck**: Query complexity, shard count

### Optimization Points

1. **Indexing**
   - Use keyword type for exact matches (id, pubkey, sig)
   - Use text type for full-text search (content)
   - Use nested type for complex tag queries
   - Use date type for time range queries

2. **Querying**
   - Leverage inverted indexes for fast lookups
   - Use doc values for sorting and aggregations
   - Minimize shard count for small datasets
   - Use routing for multi-tenant scenarios

3. **Caching**
   - Query results cached by OpenSearch
   - Filter cache for frequently used filters
   - Request cache for aggregations

## Data Lifecycle

```
Event Created
    ↓
Indexed in OpenSearch
    ↓
Available for Queries (after refresh)
    ↓
Moved to Warm Tier (after 7 days - optional)
    ↓
Archived/Deleted (after 30 days - optional)
```

## Monitoring Points

1. **Write Path**
   - Index rate (events/sec)
   - Index latency (ms)
   - Index errors

2. **Read Path**
   - Query rate (queries/sec)
   - Query latency (ms)
   - Query errors
   - Cache hit rate

3. **System**
   - JVM heap usage
   - Disk usage
   - CPU usage
   - Network I/O

4. **Cluster**
   - Cluster health (green/yellow/red)
   - Shard allocation
   - Node availability
   - Replication lag

## Best Practices

1. **Event Storage**
   - Use async refresh for better write performance
   - Batch events when possible
   - Monitor index size and shard count

2. **Querying**
   - Use appropriate filter types
   - Avoid wildcards at the start of terms
   - Use pagination for large result sets
   - Cache frequent queries at application level

3. **Maintenance**
   - Monitor cluster health regularly
   - Set up index lifecycle policies
   - Regular snapshots for backups
   - Force merge old segments periodically

4. **Scaling**
   - Add more shards for write scaling
   - Add more replicas for read scaling
   - Use separate clusters for different environments
   - Consider time-based indices for very large datasets
