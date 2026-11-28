# goal · capstone.data-intelligence.bunrei

- **State:** building
- **Task:** state
- **Branch:** state

## Tasks

1. [x] metadata
2. [x] metadata-text
3. [x] text-layer
4. [x] vision
5. [x] messaging-append
6. [x] messaging-work-queue
7. [ ] state
8. [ ] document-extraction
9. [ ] api
10. [ ] jobs
11. [ ] prompts
12. [ ] event-stream
13. [ ] tool-packaging
14. [ ] foundation-notes

## Task brief · state

**Problem**

Bunrei is still the bare template: no database, no blob storage, no JetStream, no job. `api` and
`jobs` both need the job as a stream-backed root, folded from its events and appended to under the
expected sequence, and the stores docproc and docinfer write through, retained by age. This task
builds both, with the `client` types the job's events carry, so later tasks add Commands and runs
rather than infrastructure.

**Behaviors**

1. **Current dependencies.** Bunrei builds on go-core v0.13.0 and go-web-sdk v0.19.0.
2. **The `client` module.** The nested, unreleased `client` module holds `Job`, `State`
   (`queued`, `running`, `completed`, `failed`, `cancelled`), `Manifest` and `Attempt`, as the
   protocol note defines them, and the data of `queued`, `started`, `attempt.failed` and `failed`,
   as the events note describes them. Each type round-trips through JSON. Bunrei's root module
   builds against the module in-tree, and the check covers both modules.
3. **Loading a job.** Bunrei loads a job by reading `bunrei.job.<job>` in order, returning the
   folded `Job` and the subject's last stream sequence. A job with no events is not found; an
   event whose type is not one of the 13 job types fails the load, naming the type.
4. **The fold.** Every event advances the job's gap-free sequence by one, which becomes
   `Job.Sequence`.
   - `queued` sets profile, name, size and SHA-256, state `queued`.
   - `started` sets state `running` and the attempt, and adds an attempt entry.
   - `attempt.failed` records the error and returns the state to `queued`.
   - `cancel_requested` marks cancellation requested.
   - `completed`, `failed`, `cancelled` set their terminal state; `failed` sets the error.
   - `checkpoint` keeps the last `After`.
   - `batch` folds from its headers alone: counts the batch, adds `batchchunks`, extends the digest
     by the events note's manifest rule, across every attempt.
   - `part`, `output`, `warning`, `inference` only advance the sequence.
5. **Appending to a job.** The caller decides the next event from the folded job. Bunrei stamps
   `id` (UUIDv7 when absent), `source` `bunrei`, `subject` and `correlationid` as the job's ID,
   `jobsequence` as the folded count plus one, and `layer` from the events table. It appends with
   the folded last sequence as expected. When another writer appended first, Bunrei reads on from
   that sequence, folds again, and asks the caller to decide again. A retry of an append whose
   outcome is unknown, while the stored event is still the subject's last, returns that event's
   sequence. A decision that refuses appends nothing. Bunrei refuses any append to an ended job.
6. **PostgreSQL.** At startup, before any domain node, Bunrei applies docproc's index migration
   set, then docinfer's cache set, then go-messaging's outbox and inbox set. Database and schema
   each join readiness.
7. **Blob storage.** The S3 store joins readiness. Content goes through docproc's content store
   under `content/` with a renew age, and a put and an open by SHA-256 round-trip. A job's staged
   input is kept under `input/` by its declared SHA-256, and Bunrei can reopen it.
8. **JetStream.** Bunrei owns two streams on file storage, each joining readiness. BUNREI_JOBS:
   prefix `bunrei.job`, `max_age` 30 days; the job root reads and appends on it over NATS as it
   does in memory. BUNREI_WORK: work-queue stream, subject `bunrei.work`, dead letters
   `bunrei.work.dead`, `max_age` 0.
9. **Retention.** A sweep reactor runs docproc's index retention at 30 days; go-messaging's
   outbox and inbox retention at 7 days each; and a blob reclaimer that pages through a prefix and
   deletes objects whose `ModifiedAt` is older than the prefix's age: `content/` at 67 days (renew
   age 7 days), `input/` at 30 days. Each pass stays within its budget and reports whether more
   remains.
10. **Configuration.** Every connection, name, age and budget is a setting, with the starting
    values in behaviors 7–9, the stack's addresses in the local overlay, and environment overrides.

**Test seams**
- The job root's load and append over `messaging.Journal`: memory broker in the unit tier,
  BUNREI_JOBS in the integration tier.
- The blob reclaimer's `Pass` over a go-storage client: storagetest's `Fake` with a set clock in
  the unit tier, SeaweedFS in the integration tier.
- The composed service through the integration harness, on an isolated compose stack (NATS,
  PostgreSQL, SeaweedFS) on its own ports, started and torn down by `mise run integration`, which
  CI's integration job runs.

**Slices**
1. **Upgrade** to go-core v0.13.0 and go-web-sdk v0.19.0; done when `mise run currency` exits 0 or
   reports only held items, and `mise run check` passes.
2. **The `client` module**: job and event data types, JSON round-trips, check covers it (2).
3. **The job root** on the memory broker: load, fold, stamp, guarded append and its retry after a
   conflict, refusal after the job ends (3–5).
4. **The isolated integration stack** and `mise run integration`, CI running it; the existing
   lifecycle suite passes on it.
5. **PostgreSQL and the schema**: database node, admin service applying the three sets, readiness;
   the integration tier finds the tables (6).
6. **Blob storage**: S3 store, bucket ensured, content store and staged input, integration
   round-trips (7).
7. **JetStream**: BUNREI_JOBS broker and BUNREI_WORK queue as owned streams; the job root's
   integration cases pass over NATS (8).
8. **Retention**: sweep reactor with docproc index, outbox and inbox retention and the blob
   reclaimer; unit cases on the fake, an integration pass deleting an aged object (9).

**Out of scope**
- Uploads: `upload.Store`, the `upload/` prefix's code, BUNREI_UPLOADS (`api`).
- Submit, CancelJob, Submit's idempotency record, GetJob's route, `client`'s `Client`, and
  releasing `client/v0.1.0` (`api`).
- Running jobs: workers, docproc/docinfer wiring, fencing, the dead-letter worker, `completed`'s
  `Done`, the messaging runtime (`jobs`).
- go-messaging changes: in-order delivery within a group; following one subject continuously.
- Retention for docinfer's cache, which has no retention API.
- Any release.

**Door**

Two-way. Nothing is tagged and `client` stays unreleased until `api`. Names, ages and schema are
configuration or migrations a later task can change while no deployment holds data.

## Progress

slices 0/8 committed · standards — · spec — · editor —

## Decisions

- capstone: contracts (`chunks`, `protocol`, `events`) are designed in `.foundation` at capstone-data-intelligence; Bunrei and Mikoshi build against them, and a task that reads one opens its summary with "Waits on".
- capstone: Bunrei's `client` module is released by tag (`client/v0.1.0` at `api`); Mikoshi imports the published version, never a replace directive.
- capstone: items join contexts by reference and a context covers its descendants; chunks and embeddings stay keyed by content and carry no context.
- metadata: PDF metadata through pdfinfo; rejected qpdf (its JSON reaches the dictionary and XMP only by object id, and leaves PDF `D:` date parsing to docproc).
- metadata: EXIF through magick; rejected exiftool (a new Perl tool in Bunrei's image) and Go EXIF
  libraries (none ecosystem-standard).
- metadata: docinfer renders the metadata text; rejected a docproc text rendering (chunked as
  content), Bunrei (`cmd/docinfer` would lack it), and widening `MetadataText` to take the record
  (every host would handle an error a part can't have); never defer a layer's change to avoid its
  release.
- metadata: capstone-bunrei requires docproc and docinfer only from the first task that imports them.
- metadata-text: Skype conversation records are not rendered (backlog).
- text-layer: a page's text layer stands in for its vision text when it holds at least the
  threshold's words, a word being a run of letters or digits. A page with no layer always goes
  to vision. 100 is a starting value, to be tuned from the live run's measurements of thin
  pages skipped and vision words lost. Rejected OCRmyPDF's any-text rule, which skips every thin
  page, and bitmap coverage, which needs a Python layout pass or a new docproc adapter.
- text-layer: docinfer's `Pipeline.Run` decides the skip by matching a page image to the text
  output of its page that comes just before it. docproc keeps emitting every page image.
  Rejected dropping the image in docproc's document stage, which would put inference policy
  into preparation.
- text-layer: Bunrei's profile carries the threshold, set at `jobs`.
- text-layer: a OneNote picture is an image and uses vision. Its OCR text reaches chunks only
  through its page's Markdown, and there is no backlog entry for it.
- vision: Bunrei's profile prompts are defaults; an active override stored at runtime replaces
  each, as ~/code/herald does. Overrides are scoped per profile and stage (`vision`,
  `transcription`), at most one active per pair; the extraction prompt stays the caller's.
  Rejected: prompts fixed in code or configuration only, and per-request prompt overrides.
- vision: Bunrei rebuilds a profile's docinfer pipeline when its resolved prompts change; docinfer
  gains no per-run prompt, so `Profile` stays known before any job runs.
- vision: a retried job's attempt uses the prompts active when it starts; its `started` records
  them.
- vision: the override Queries and Commands join the protocol and the `client` module.
- vision: a new task, `prompts`, after `jobs`, builds the override store, API, and resolution;
  `jobs` builds pipelines from configured prompts only. Whether Bunrei and Mikoshi share an
  override library is decided when `prompts` is planned.
- vision: Mikoshi's chat agent system prompt (`.mikoshi` `context-tools`) follows the same pattern.
- vision: kept card sampling; rejected the finding's lower vision temperature (temperature 0 loops
  at 1,120 image tokens, and overflow is tested only at 1.0).
- messaging-append: a stream-backed root appends and reads through `messaging.Journal`, beside
  `Broker`. Rejected: a method on the nats provider alone, which would make `state` import nats,
  and go-core's `event.Sink`, since `Emit` overwrites the IDs Bunrei mints ahead and `Sink.Write`
  returns no sequence.
- messaging-append: memory, like a single nats-server, checks an append's expected sequence before
  deduplicating, so a stale repeat is `messaging.ErrConflict` unless the original is still the
  subject's last event, and a retry whose ack was lost succeeds; `state` relies on it. A
  replicated stream deduplicates first, so there a stale repeat within the window returns the
  original's sequence.
- messaging-append: Bunrei's gap-free `jobsequence` is a count its own fold derives, not the
  stream sequence `Journal` returns.
- messaging-work-queue: jobs go through go-messaging's `messaging.Queue`, apart from `Broker`, on
  a stream of its own with work-queue retention; the contract names nothing from JetStream, and
  no Postgres provider is built (backlog).
- messaging-work-queue: a worker runs through `messaging.Runtime.Work`. go-core's reactor detaches
  a handler from its source's cancellation, so `Runtime.Work` ends the handler's context from
  `messaging.DeliveryEnd` and `reactor.Draining`; go-core is unchanged.
- messaging-work-queue: the queue extends a running delivery itself; the handler takes no extend
  function, and its context ends once no extension has succeeded within `AckWait`. Bunrei's
  fencing covers the overlap.
- messaging-work-queue: `AckWait` bounds a delivery between extensions, not a job's length. The
  NATS queue rejects `AckWait` plus `nats.AckMargin` at or past `messaging.DefaultDuplicates`
  (2m), so a repeated dead letter is deduplicated; a non-owner bound to a stream with a smaller
  window isn't protected.
- messaging-work-queue: the queue enforces `MaxDeliver` itself, and the NATS consumer sets no
  server delivery limit, so a dead letter whose publish fails is retried on every later delivery.
  A failed final attempt, an `event.Permanent` error, or a delivery past the limit is
  dead-lettered, so `state` needs no advisory or sweep. Rejected the server limit at `MaxDeliver`
  + 1, which strands a job whose dead-letter publish fails on the extra delivery, and the
  MAX_DELIVERIES advisory, lost when no subscriber listens and lacking the message.
- messaging-work-queue: a dead letter is the original event, unchanged, with attempt and
  last-error headers, on a subject inside the queue's own stream; a worker on it (Bunrei
  appending `failed`) takes each off with an ack.
- messaging-work-queue: a work-queue stream takes one worker group per subject, so two kinds of
  work need two queues.
- messaging-work-queue: on drain, a handler's nil or permanent error settles its job, and any
  other error leaves the job for redelivery.
- messaging-work-queue: the final attempt is attempt == `MaxDeliver`, which `Worker.Final` reports;
  `jobs` sets `docinfer.Options.Final` from it.
- messaging-work-queue: released as go-messaging `v0.5.0` and `nats/v0.5.0`; `state` builds on
  them. The postgres sqlate bumps stay under [Unreleased].

- state: `state` builds Bunrei's stores and the job root (fold and guarded append); Submit,
  CancelJob and routes stay `api`'s, running jobs `jobs`'.
- state: the job's types live in the nested `client` module, created unreleased at `state` via
  `replace ./client`; `api` adds `Client` and the Commands and tags `client/v0.1.0`.
- state: the fold applies all 13 job types; batches fold from headers into `Batches`, `Chunks`,
  `Digest`; `checkpoint` keeps the last `After`; `jobs` writes `completed`'s `Manifest` from the
  fold and adds `Done`.
- state: the job root stamps `id`, `source`, `subject`, `correlationid`, `jobsequence`, `layer`;
  the caller sets `causationid`; appends to an ended job are refused.
- state: blob storage is retained by a Bunrei reclaimer (list prefix, delete by `ModifiedAt`) on
  go-database's `sweep`, beside docproc index and go-messaging outbox/inbox retention. Rejected a
  bucket lifecycle rule (docproc/storage's recommendation): go-storage can't set one, the stack
  creates no bucket, SeaweedFS support unconfirmed.
- state: starting retentions: BUNREI_JOBS 30d (GetJob loses the job after); docproc index 30d;
  content renew 7d; `content/` 67d; `input/` 30d; outbox and inbox 7d; BUNREI_WORK 0.
- state: Bunrei's `upload.Store` is `api`'s (PostgreSQL row with chunks at
  `upload/<upload>/<offset>`, or a BUNREI_UPLOADS stream, settled there).
- state: the integration tier runs on Bunrei's own compose stack via `mise run integration`, in CI.
- state: go-messaging unchanged; in-order delivery within a group goes to `event-stream`;
  following one job's subject for fencing goes to `jobs` (or polling `Read`).
- state: names: BUNREI_JOBS (prefix `bunrei.job`), BUNREI_WORK (`bunrei.work`, dead letters
  `bunrei.work.dead`), file storage; blob prefixes `content/`, `input/`, `upload/`.

## Pending edits

- coordinator · `docs/data-intelligence.md`, "Tool packaging": the image holds poppler-utils 22.02
  or later, whose pdfinfo docproc v0.10.0's document stage requires beside pdftotext and pdfdetach,
  and ImageMagick 7.1 or later, whose magick its vision stage now requires, as the picture and
  office stages do. "Open items": `metadata` resolved by docproc v0.10.0.
- coordinator · roadmap: a backlog entry for picture metadata beyond EXIF (IPTC, maker notes,
  reverse geocoding).
- coordinator · roadmap: a backlog entry for rendering Skype conversation records
  (`MediaMailConversation`: topic, start, end, missed) as metadata text.
- capstone-data-intelligence · `context/chunks.md`, "Fields" and "Source occurrence": a source's
  text SHA-256 is its `sha256` when present, else its `output`'s; a metadata source may carry the
  record output it renders, whose `sha256` is the record's, not the text's; links move to docinfer
  v0.4.0 and docproc v0.10.0. The change reaches `.mikoshi`'s `ingest`, `contexts` and `lineage`
  tasks and `.bunrei`'s `api` task.
- coordinator · `docs/data-intelligence.md`, "Open items": `text-layer` resolved. A OneNote
  picture is an image and uses vision.
- coordinator · `docs/data-intelligence.md`, "Text layer and vision": cite docs-corpus v0.2.0
  rather than the retired ~/docs pool. The `text-layer` bullet: resolved by docinfer v0.5.0's
  layer-word skip, starting at 100 words; correct the claim that two-column layers interleave
  their columns: the finding shows pdftotext reads column by column, and the misses are words
  split across columns. The `vision` bullet: resolved by keeping the starting vision prompt and
  card sampling, since over docs-corpus v0.2.0 only 2 of 1,234 pages with at least 100 layer
  words come back described, both one unreadable table (docinfer's `docs/acceptance.md`,
  "Described pages").
- coordinator · `docs/data-intelligence.md`, "Bunrei", "Models and stages": a profile's vision
  and transcription prompts in Bunrei's configuration are defaults, and an active override stored
  at runtime replaces each, per profile and stage.
- docproc · `context/bunrei.md`, "The profile": a profile holds the vision prompt and sampling,
  and its vision and transcription prompts are defaults that an active override, stored at
  runtime, replaces.
- coordinator · roadmap: a backlog note that go-ai's built-in chat-mode transcription
  instruction, which docinfer sends when its transcription prompt is empty, is not in docinfer's
  transcription cache key, so a go-ai change to it doesn't miss held transcripts.
- coordinator · roadmap: a backlog entry for a Mikoshi prompt-tuning view over Bunrei's prompt
  override Queries and Commands.
- coordinator · `docs/data-intelligence.md`, "docinfer's configuration": add the vision
  layer-word threshold, cmd/docinfer's `vision.layer_words`, starting at 100, to be tuned from
  the thresholds docinfer's `docs/acceptance.md` measures ("The layer-word threshold").
- docs-corpus · `docs/baseline.md`, "v0.2.0": the run directory also holds docproc v0.10.0's run,
  `docproc-v0.10.0/`, with its binaries in `bin-v0.10.0/`, and docinfer's text-layer run in
  `docinfer/text-layer/`; link docinfer's
  `docs/acceptance.md#docs-corpus-v020-with-the-text-layer-skip`.
- coordinator · roadmap: a backlog entry for appending several events atomically (JetStream
  atomic batch publish, once nats.go has a client API).
- coordinator · `context/messaging.md`: "Open", "A stream-backed root": drop "(planned)";
  go-messaging builds the append and subject read as `messaging.Journal` (v0.4.0, nats/v0.4.0)
  and the work-queue package as `messaging.Queue` (v0.5.0, nats/v0.5.0); Bunrei's use stays
  planned. Retire "A dead-letter stream, deferred until a consumer needs replay": the queue
  dead-letters onto a subject on its own stream, and replay stays unbuilt. Add to "Open": a
  replicated stream deduplicates an append before checking its expected sequence, so a stale
  repeat within the window returns the original's sequence, not `ErrConflict`. "Decisions": a
  work-queue worker runs through `messaging.Runtime.Work`, since go-core's reactor detaches a
  handler from its source's cancellation; go-core is unchanged. "Assumptions", the outbox bullet:
  Bunrei appends a job's events through `messaging.Journal`.
- coordinator · `docs/messaging.md`, "A stream-backed root": drop "Planned."; go-messaging builds
  the append as `messaging.Journal` (its `docs/usage.md`, "Appending to a stream-backed root") and
  the work-queue package as `messaging.Queue` (its `docs/usage.md`, "The work queue"). "The
  capability ledger": add a row, a work queue of durable jobs with extension, a retry limit,
  backoff, and dead letters, classed Standard as `messaging.Queue`, apart from `Broker`.
- coordinator · `docs/data-intelligence.md`, "Open items": drop `state`'s question of how a job
  whose last delivery died is found and failed; go-messaging v0.5.0's work queue dead-letters it.
- docproc · `context/bunrei.md`: "A job": the work queue, not the replica, extends a running
  delivery, and `docinfer.Options.Final` comes from `messaging.Worker.Final`. "A dead last
  delivery": closed; the queue dead-letters a delivery past `MaxDeliver` without running its
  handler, and Bunrei appends `failed` from the dead-letter subject. The stores diagram:
  BUNREI_WORK also holds the queue's dead-letter subject and the subjects under it. "Requests to
  go-messaging": v0.4.0's `messaging.Journal` meets "A publish on a subject the caller names" with
  a guarded append and a bounded read, and v0.5.0's queue meets the in-progress acknowledgement
  with its own extension of a running delivery.
- coordinator · roadmap: a backlog entry for a Postgres `SKIP LOCKED` work-queue provider that
  meets go-messaging's work-queue contract and conformance suite.
- coordinator · roadmap: `state` summary drops "uploads"; `api` takes `upload.Store` and Submit's
  idempotency record; `event-stream` takes in-order delivery within a group; `jobs` takes
  following one job's subject for fencing and emptying docproc's working directory at startup.
  Retires the existing edit about the asks `messaging-append` left out.
- coordinator · roadmap: backlog entry for docinfer cache retention.
- capstone-data-intelligence · `context/protocol.md` "Ownership" and "The client module";
  `context/events.md` "Ownership": `state` creates `client` with the job types; `api` completes and
  releases it.
- docproc · `context/bunrei.md` "State and subjects": settled names and retentions; Bunrei's
  reclaimer replaces "host sweeps content by age"; drop "names and retentions are `state`'s".
- coordinator · `docs/data-intelligence.md`: "Open items" `state` names/retentions resolved;
  "Bunrei", "State": reclaimer, not lifecycle rule.
- coordinator · `context/data-intelligence.md` go-messaging asks: append and work queue built
  (v0.4.0, v0.5.0); remaining asks move to `event-stream` and `jobs`.
