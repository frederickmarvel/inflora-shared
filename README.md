# inflora-shared

Shared Go contracts and helpers for all Inflora backend services. This repository is a library and is not deployed.

Phase 1 provides canonical event and RPC contracts plus reusable infrastructure helpers. The module is imported directly by services and has no runtime deployment.

```sh
make generate # requires buf, protoc-gen-go, and protoc-gen-go-grpc
make tidy
make lint
make test
make build
```

Protobuf sources live in `proto/palantir/v1` and generated Go clients live in `gen/go/palantir/v1`.

Source of truth: `almanac/planning/schemas/` and `almanac/planning/WIRE_GUIDE.md`.
