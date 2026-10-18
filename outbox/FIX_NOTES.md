# Outbox Dispatcher — Local Fix Notes

> **Audience:** Marvel (who owns `inflora-shared`) when reviewing what to upstream.
>
> **Scope:** a single bug fix in `dispatcher.go` that was crashing `tolkien` on every overlay-token rotation during local dev. The fix is intentionally minimal and reversible.

---

## 1. The bug

`requireOne` returned a fatal error for any UPDATE that affected 0 rows:

```go
// before
func requireOne(result sql.Result, eventID uuid.UUID) error {
    n, err := result.RowsAffected()
    if err != nil { return fmt.Errorf("outbox: rows affected: %w", err) }
    if n != 1 { return fmt.Errorf("outbox: claim ownership lost for %s", eventID) }
    return nil
}
```

The two callers — `DispatchOne` (post-publish) and `fail` (post-retry/dead-letter) — propagate that error to `runtime.Run`, which treats any worker error as fatal. The service shuts down cleanly. Observed in `tolkien`:

```
tolkien-1  | 2026-10-12T22:05:57.827Z  error   shutdown  {"service": "tolkien",
            "error": "runtime: worker \"tolkien-outbox\": outbox: claim ownership
                      lost for 1aa14ffa-3e5b-4622-a7ee-c6f85503f44b"}
tolkien-1 exited with code 0
```

### Why 0 rows is not a fatal error

The UPDATEs that call `requireOne` are CAS predicates:

```sql
UPDATE outbox_events
SET published_at = $4, claimed_at = NULL, claimed_by = NULL, last_error = NULL
WHERE event_id = $1 AND claimed_by = $2 AND claimed_at = $3 AND published_at IS NULL
```

The `WHERE claimed_at = $3` and `WHERE claimed_by = $2` clauses mean: *only update this row if the row is still owned by us at the exact `claimed_at` we read when we claimed it.* If anything else has touched the row in between (lease expiry, a manual DB poke, a concurrent worker that took over after a TTL miss), 0 rows match. The CAS guarantees 0 or 1, never 2+.

A 0-row result means one of:

1. Another worker already published this event → publish goal is achieved, nothing to do.
2. Another worker already dead-lettered this event → work is done, nothing to do.
3. The lease expired and a third worker now owns the row → whoever owns it now is responsible.
4. The row was already in `published_at IS NOT NULL` (predicate excluded it) → nothing to do.

In every case, the dispatcher's job for this row is finished. Killing the worker over it is a bug, not a safety property.

This is the third instance of the same shape in `inflora-shared` (after the two `events/nats.go` bugs fixed earlier in `LOCAL_DEV_NOTES.md`): brittle error handling in shared code that the `runtime` does not tolerate.

---

## 2. The fix

Change `requireOne` to distinguish *skip* from *error*:

```go
// after
func requireOne(result sql.Result, eventID uuid.UUID) (bool, error) {
    n, err := result.RowsAffected()
    if err != nil { return false, fmt.Errorf("outbox: rows affected: %w", err) }
    if n == 0   { return true, nil }                                            // skip
    if n != 1   { return false, fmt.Errorf("outbox: expected 1 row affected, got %d (event %s)", n, eventID) }
    return false, nil                                                            // success
}
```

Callers log the skip and continue:

```go
skipped, err := requireOne(res, event.eventID)
if err != nil { return true, err }
if skipped {
    log.Printf("outbox: post-publish CAS miss for %s, continuing (handled by another worker)", event.eventID)
}
return true, nil
```

The same shape replaces the `requireOne` call in `fail` (post-fail path).

### Why log to `log.Default()` instead of a struct field

The package is a library — it does not take a logger dependency today. Adding `*log.Logger` to `DispatcherConfig` would be a public API change, which is out of scope for a local unblock. `log.Default()` keeps the surface zero and gives a one-line warn in `journalctl` / `docker compose logs` for free. A future PR can move to `*zap.Logger` and accept it on `DispatcherConfig`; the contract change is independent.

### Test added

`outbox_test.go` gains `TestRequireOneContract` with four subtests: 0 rows is a skip, 1 row is success, 2 rows is a bug, negative rows is a bug. The bug-row case asserts the event ID is in the error message for debuggability.

---

## 3. Files touched

| File | Change |
|---|---|
| `inflora-shared/outbox/dispatcher.go` | `requireOne` signature change; both call sites updated to log+continue on skip. New `log` import. |
| `inflora-shared/outbox/outbox_test.go` | New `TestRequireOneContract` (4 subtests). New `strings` and `uuid` imports. |

No other files. No public API surface changes. No DB schema changes.

---

## 4. What Marvel needs to do for the upstream PR

1. **Review and ack the signature change** on `requireOne`. It is unexported, so it does not break consumers. But it is referenced in package documentation only by behavior, so the change is safe.
2. **Decide on logger** — accept the `log.Default()` choice for now, or bump the patch to inject `*zap.Logger` via `DispatcherConfig`. The latter is cleaner long-term but expands the API.
3. **Decide whether to keep the `log.Printf` lines** or replace with structured logging through the package's eventual logger.
4. **Audit the same pattern in `inflora-shared/inbox`** if one exists. The inbox/outbox pair is the same shape; if the inbox has a `requireOne` analog, it has the same bug.
5. **Consider a "claim lost is normal" metric** — if this race is rare, no metric is needed. If it fires often in production, add a `outbox_cas_miss_total` counter so the frequency is visible.

The fix is the minimum needed to keep `tolkien` up. It does not paper over the underlying race; it just stops the worker from dying when the race resolves successfully.
