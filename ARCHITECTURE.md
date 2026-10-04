# Architecture

`inflora-shared` is the only permitted code-level dependency between Inflora services. It owns cross-service protobuf definitions, event types, and reusable infrastructure helpers. It contains no service executable and no business workflow orchestration.

Proto sources live under `proto/palantir/v1`; generated Go code will live under `gen/go/palantir/v1` from Phase 1. Schema documents in the almanac remain authoritative.
