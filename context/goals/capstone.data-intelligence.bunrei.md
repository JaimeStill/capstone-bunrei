# goal · capstone.data-intelligence.bunrei

- **State:** idle
- **Task:** none
- **Branch:** none

## Tasks

1. [ ] metadata
2. [ ] text-layer
3. [ ] vision
4. [ ] messaging-append
5. [ ] messaging-work-queue
6. [ ] state
7. [ ] api
8. [ ] jobs
9. [ ] event-stream
10. [ ] tool-packaging

## Decisions

- capstone: contracts (`chunks`, `protocol`, `events`) are designed in `.foundation` at capstone-data-intelligence; Bunrei and Mikoshi build against them, and a task that reads one opens its summary with "Waits on".
- capstone: Bunrei's `client` module is released by tag (`client/v0.1.0` at `api`); Mikoshi imports the published version, never a replace directive.
- capstone: only `.bunrei` touches the exclusive group (docproc, docinfer, go-messaging); `.mikoshi` stays out of it, so its SSE relay and context tree live in Mikoshi, shaped to graduate.
- capstone: items join contexts by reference and a context covers its descendants; chunks and embeddings stay keyed by content and carry no context.

## Pending edits

- none
