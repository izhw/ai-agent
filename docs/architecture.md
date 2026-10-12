# Architecture

## Boundaries

`contract` and `spi` are the stable application boundary. They do not expose Eino, GORM, Hertz, or model-vendor types. Framework-specific code belongs in `adapter`.

```text
Product -> HTTP/Go API -> agentkit.Engine -> SPI -> adapters
                                             |- Eino ADK runtime
                                             |- GORM/custom storage
                                             |- portable/vector memory
                                             `- model/tool providers
```

## Agent lifecycle

Agent versions are immutable. Publishing validates the selected Runtime Factory before persistence. Activating a version changes the alias used by new runs; every run pins its resolved version so deployment, rollback, and resume are reproducible.

Runtime selection is registry-based. The built-in `chat_model` and `deep_agent` factories both produce the same stable event stream. New strategies implement `spi/runtime.Factory` without changing the engine.

## Persistence

`spi/storage.Storage` groups repositories and transaction boundaries. The default GORM implementation accepts an existing `*gorm.DB`; applications can therefore inject any tested GORM Dialector. A completely different persistence technology can implement the SPI directly.

`spi/observability.Observer` receives run start, durable event, and run end callbacks. It is deliberately synchronous at the engine boundary so implementations can choose whether to enqueue to an asynchronous exporter; it never changes the product event protocol.

Portable schema rules:

- string ULIDs rather than database sequences;
- UTC timestamps;
- JSON serialized as bytes where native JSON is not required;
- checkpoint payloads carry framework and codec versions;
- tenant and product are part of every business lookup;
- production migrations are versioned and database-specific when required.

MySQL and PostgreSQL are first-class built-in dialects. Dameng uses an injected Dialector and must be integration-tested against the exact driver/server version used by the adopter.

## Memory

Conversation history, execution checkpoints, and long-term memory are separate concepts. Long-term memory is disabled by default but its full read/write pipeline is present.

The portable retriever loads a bounded, tenant-scoped candidate set and ranks it in Go using lexical similarity or optional embeddings. This keeps MySQL, PostgreSQL, and Dameng equivalent. At larger scale, only the retriever/index adapter is replaced by a vector engine.

The safe default extractor stores only explicit requests such as `记住：...`. Automatic LLM extraction is an extension point and should apply confidence, consent, PII, deduplication, and expiration policies.

## Checkpoint compatibility

Eino checkpoints use framework-owned serialization. Stored records therefore include the Eino and codec versions. An Eino upgrade must run checkpoint compatibility tests before rollout. Existing runs stay on their pinned Agent version; incompatible checkpoints must fail safely rather than be silently discarded.
