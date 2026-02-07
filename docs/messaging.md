# Messaging Design for Multi-Agent Task Execution

## Summary
This document defines a broker-backed execution model for agents, while keeping task state authoritative in the database.

- **Control plane:** existing HTTP API (create/list/show/update)
- **Data plane:** message broker (NATS JetStream or RabbitMQ)
- **Safety model:** at-least-once delivery + idempotent handlers + DB-atomic claim/lease

This prevents claim races and supports horizontal scale.

---

## 1) Core principles

1. **Database is source of truth** for task lifecycle.
2. **Broker is transport**, not authority.
3. **All workers are idempotent** (duplicate deliveries are expected).
4. **Claims are atomic** (single SQL mutation with guards).
5. **Leases expire** so crashed workers do not permanently hold tasks.
6. **Retries are explicit** with backoff and terminal dead-letter behavior.

---

## 2) Task execution state model

Recommended execution fields (in the task table or a dedicated execution table):

- `status`: `queued | running | retry_wait | succeeded | failed | dead`
- `owner_agent_id` (nullable)
- `lease_until` (nullable RFC3339)
- `attempt` (int)
- `max_attempts` (int)
- `next_attempt_at` (nullable RFC3339)
- `last_error` (nullable text/json)
- `trace_id` (string)
- `updated_at`

State transitions:

- `queued -> running` (claim succeeds)
- `running -> succeeded` (terminal success)
- `running -> retry_wait` (transient failure)
- `retry_wait -> queued` (retry scheduler republish)
- `running -> dead` (attempt >= max_attempts)
- `running -> queued` (lease expiry recovery)

---

## 3) Message envelope and schemas

Use one envelope for all commands/events:

```json
{
  "schema_version": "v1",
  "message_id": "7dc6e5af-0776-4f37-8f16-5767f5f9471f",
  "trace_id": "tr-8f678c2a",
  "causation_id": "optional-parent-message-id",
  "kind": "cmd.task.execute",
  "emitted_at": "2026-02-06T21:00:00Z",
  "payload": {}
}
```

### Command: `cmd.task.execute`

```json
{
  "task_id": "gr-1a2b",
  "target_agent_type": "worker",
  "reason": "new_task|retry_due|lease_expired"
}
```

### Event: `evt.task.claimed`

```json
{
  "task_id": "gr-1a2b",
  "agent_id": "agent-17",
  "attempt": 2,
  "lease_until": "2026-02-06T21:01:00Z"
}
```

### Event: `evt.task.retry_scheduled`

```json
{
  "task_id": "gr-1a2b",
  "attempt": 2,
  "next_attempt_at": "2026-02-06T21:03:12Z",
  "backoff_ms": 132000,
  "error_class": "upstream_timeout"
}
```

### Event: `evt.task.completed`

```json
{
  "task_id": "gr-1a2b",
  "agent_id": "agent-17",
  "attempt": 2,
  "result_ref": "s3://... or db://...",
  "duration_ms": 8412
}
```

---

## 4) Topic / queue names

### Canonical logical names

Commands:
- `cmd.task.execute.v1`
- `cmd.agent.<agent_type>.<action>.v1`

Events:
- `evt.task.claimed.v1`
- `evt.task.completed.v1`
- `evt.task.failed.v1`
- `evt.task.retry_scheduled.v1`
- `evt.task.dead.v1`
- `evt.task.lease_expired.v1`
- `evt.agent.<agent_type>.<event>.v1`

`v1` is part of routing key/subject for schema evolution.

---

## 5) Atomic claim (race-safe)

When a worker receives `cmd.task.execute` for `task_id`, it attempts claim:

```sql
UPDATE tasks
SET status = 'running',
    owner_agent_id = :agent_id,
    lease_until = :lease_until,
    attempt = attempt + 1,
    updated_at = :now
WHERE id = :task_id
  AND status IN ('queued', 'retry_wait')
  AND (next_attempt_at IS NULL OR next_attempt_at <= :now)
  AND (lease_until IS NULL OR lease_until < :now);
```

Then check rows affected:
- `1`: claim success.
- `0`: another worker already owns it / not due / already terminal. Ack and stop.

On success, emit `evt.task.claimed.v1`.

---

## 6) Lease + heartbeat

- Lease duration example: **60s**.
- Heartbeat interval example: **20s**.

Heartbeat update:

```sql
UPDATE tasks
SET lease_until = :new_lease_until,
    updated_at = :now
WHERE id = :task_id
  AND owner_agent_id = :agent_id
  AND status = 'running';
```

If rows affected is `0`, ownership is lost; worker must abort processing.

### Lease reaper (background loop, every ~10s)

1. Select `running` tasks where `lease_until < now`.
2. Move back to `queued`, clear owner/lease.
3. Emit `evt.task.lease_expired.v1`.
4. Publish `cmd.task.execute.v1` (reason=`lease_expired`).

---

## 7) Retries

### 7.1 Transient failure retry

On retryable failure:

1. Compute backoff with jitter:
   - `base = 5s`
   - `delay = min(base * 2^(attempt-1), 15m)`
   - `delay = delay * random(0.8..1.2)`
2. Update task:
   - `status='retry_wait'`
   - `next_attempt_at=now+delay`
   - `last_error=...`
3. Emit `evt.task.retry_scheduled.v1`.
4. Ack current message.

### Retry scheduler loop (every ~5s)

Find tasks with:
- `status='retry_wait'`
- `next_attempt_at <= now`

Transition to `queued` and publish `cmd.task.execute.v1` (reason=`retry_due`).

### 7.2 Terminal failure / dead-letter

If `attempt >= max_attempts`:

- set `status='dead'`
- set `last_error`
- emit `evt.task.dead.v1`
- publish details to DLQ for ops triage

---

## 8) Inter-agent communication pattern

Prefer **event-driven collaboration** over direct RPC chat.

- Agent publishes facts (`evt.*`) for broad consumption.
- Agent sends directed requests via command channel (`cmd.agent.<type>.<action>.v1`).
- Optional per-agent mailbox:
  - `cmd.agent.instance.<agent_id>.v1` for targeted delivery.

This gives decentralized behavior with auditable, replayable coordination.

---

## 9) NATS JetStream profile

### Streams

- `GRNS_CMD` subjects: `cmd.>`
- `GRNS_EVT` subjects: `evt.>`
- optional `GRNS_DLQ` subjects: `dlq.>`

### Consumers

- Worker durable consumer on `cmd.task.execute.v1` (queue group by worker pool)
- Explicit ack mode
- Use `InProgress()` while long-running to extend redelivery timer

### Notes

- Redelivery is expected; DB claim gate prevents duplicate execution.
- Keep `AckWait` shorter than lease duration unless sending `InProgress()` heartbeats.

Pseudo worker loop (NATS):

```go
for msg := range sub.Messages() {
    cmd := decode(msg)
    claimed := tryClaim(cmd.TaskID, agentID, now, leaseUntil)
    if !claimed {
        msg.Ack()
        continue
    }

    publish("evt.task.claimed.v1", ...)

    done := make(chan error, 1)
    go func() { done <- runTask(cmd.TaskID) }()

    hb := time.NewTicker(20 * time.Second)
    defer hb.Stop()

    for {
        select {
        case err := <-done:
            if err == nil {
                markSucceeded(cmd.TaskID, agentID)
                publish("evt.task.completed.v1", ...)
            } else if isRetryable(err) {
                scheduleRetry(cmd.TaskID, err)
                publish("evt.task.retry_scheduled.v1", ...)
            } else {
                markDeadOrFailed(cmd.TaskID, err)
                publish("evt.task.dead.v1", ...)
            }
            msg.Ack()
            goto next
        case <-hb.C:
            _ = renewLease(cmd.TaskID, agentID)
            _ = msg.InProgress()
        }
    }
next:
}
```

---

## 10) RabbitMQ profile

### Exchanges

- `grns.cmd` (topic)
- `grns.evt` (topic)
- `grns.dlq` (topic)

### Queues / bindings

- `grns.cmd.task.execute` bound to routing key `task.execute.v1`
- `grns.cmd.agent.<type>` bound to `agent.<type>.*.v1`
- observability consumers bind to `evt.#`

### Ack + QoS

- Manual ack (`autoAck=false`)
- Prefetch tuned to worker concurrency (e.g., 8)
- Ack only after DB state transition is durably committed

Pseudo worker loop (RabbitMQ):

```go
msgs, _ := ch.Consume("grns.cmd.task.execute", "", false, false, false, false, nil)
for d := range msgs {
    cmd := decode(d.Body)
    claimed := tryClaim(cmd.TaskID, agentID, now, leaseUntil)
    if !claimed {
        d.Ack(false)
        continue
    }

    publishTopic("grns.evt", "task.claimed.v1", ...)

    err := runWithLeaseHeartbeat(cmd.TaskID, agentID)
    switch {
    case err == nil:
        markSucceeded(cmd.TaskID, agentID)
        publishTopic("grns.evt", "task.completed.v1", ...)
    case isRetryable(err):
        scheduleRetry(cmd.TaskID, err)
        publishTopic("grns.evt", "task.retry_scheduled.v1", ...)
    default:
        markDeadOrFailed(cmd.TaskID, err)
        publishTopic("grns.evt", "task.dead.v1", ...)
    }

    d.Ack(false)
}
```

---

## 11) Idempotency and dedupe

Required safeguards:

1. **Task claim gate** (atomic SQL) is primary dedupe.
2. Optional `processed_messages` table keyed by `message_id` for event handlers.
3. Make side effects idempotent by natural keys (e.g., `UNIQUE(task_id, artifact_type)`).
4. All emitted events include `trace_id` + `causation_id`.

---

## 12) Operations and observability

Track at minimum:

- claim success rate / claim conflicts
- lease renew failures
- lease expirations
- retry count by error class
- dead-letter count
- end-to-end latency (`queued -> terminal`)
- queue lag / consumer lag

Recommended alerts:

- `dead` tasks > threshold
- lease expirations spiking
- retry scheduler backlog growing
- broker consumer lag above SLO

---

## 13) Practical recommendation

Both brokers work with this design. Choose based on team preference:

- **NATS JetStream**: strong fit for event-driven inter-agent systems.
- **RabbitMQ**: strong fit for traditional queue workflows and broad operational familiarity.

The reliability properties come from the same core mechanics in both cases:
**atomic claim + lease heartbeat + explicit retries + idempotent handlers**.
