# AI Repository Guide: inflora-shared

## Purpose

`inflora-shared` is the non-deployable Go library shared by the active Inflora services. It defines canonical events and RPC contracts and provides reusable infrastructure for authentication, inbox/outbox delivery, retries, keys, runtime lifecycle, observability, providers, middleware, and ledger helpers.

Changes here have a wide blast radius. Favor backward-compatible, versioned contracts and small, reusable primitives over service-specific business logic.

## Important paths

- `events/`: versioned event names and typed payloads.
- `proto/palantir/v1/`: protobuf sources.
- `gen/go/palantir/v1/`: generated Go protobuf/gRPC code.
- `auth/`, `keys/`, `middleware/`: trust-boundary helpers.
- `inbox/`, `outbox/`, `retry/`: reliable messaging primitives.
- `runtime/`, `observability/`: process lifecycle, metrics, tracing, and logging.
- `provider/`: normalized payment-provider interfaces/adapters.
- `ledger/`: shared ledger types or helpers; service ownership remains in Saruman.

## Contract and correctness rules

- Canonical behavior is documented in `/Users/frederickmarvel/Inflora/almanac/planning/schemas/` and `almanac/planning/WIRE_GUIDE.md`.
- Do not introduce imports from a deployable service into this module.
- Event payloads are wire contracts: use explicit versions, stable JSON fields, deterministic serialization expectations, and compatibility tests.
- Do not hand-edit generated protobuf files. Change `.proto` sources and run generation.
- Avoid hiding service-specific policy in generic helpers.
- Security, retry, and messaging helpers must have failure-path and concurrency coverage.

## Commands

```sh
make generate
make tidy
make lint
make test
make build
```

Generation requires `buf`, `protoc-gen-go`, and `protoc-gen-go-grpc`. Run `go test ./...`, `go vet ./...`, and `git diff --check`, then test affected consuming services when a public contract changes.
