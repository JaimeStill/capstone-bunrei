# goal · capstone.data-intelligence.bunrei

- **State:** building
- **Task:** api
- **Branch:** api

## Tasks

1. [x] metadata
2. [x] metadata-text
3. [x] text-layer
4. [x] vision
5. [x] messaging-append
6. [x] messaging-work-queue
7. [x] state
8. [x] document-extraction
9. [ ] api
10. [ ] jobs
11. [ ] prompts
12. [ ] event-stream
13. [ ] tool-packaging
14. [ ] foundation-notes

## Task brief · capstone.data-intelligence.bunrei · api

**Problem**

Bunrei holds the job root and its stores but serves no API: no caller can submit a document,
upload a large one, read a job, or cancel it, and Mikoshi has no `client` release to import. This
task serves the protocol's Commands and job Queries, mounts go-web-sdk's resumable upload over
Bunrei's blob storage, and releases the `client` module as `client/v0.1.0`.

**Behaviors**

1. **Current dependencies.** Bunrei and its `client` module build on docinfer v0.6.0,
   docinfer/postgres v0.1.4, go-core v0.14.0 and go-web-sdk v0.20.0.
2. **Bunrei's schema.** At startup Bunrei applies its own migration set after the three it
   applies today, creating its upload, completed-upload and submission tables.
3. **The upload store.** Bunrei's `upload.Store` passes go-web-sdk's `uploadtest` suite against
   PostgreSQL and blob storage. An upload is one row (metadata, offset, length, hash state,
   expiry); each committed chunk is an object under `upload/<upload>/<offset>/<append id>` whose
   key its chunk row records. Of two appends racing at one offset, one commits and the other's
   object never reads back. A missing or expired upload matches `fs.ErrNotExist`.
4. **Submit.** Submit takes a JSON body, `client.Submit`'s own JSON, with the content base64.
   It stages the content under `input/` by its SHA-256, appends `queued` (profile, name, media
   type, size, SHA-256, and the per-source, document and timeline extracts, each optional) to a
   new UUIDv7 job, enqueues `{"job"}` on BUNREI_WORK, and returns the job, `queued`. Content past
   the small-document limit (4 MiB, a setting) is refused 413; extracts that marshal past 256 KiB
   together are refused; an empty profile is refused. `application/json` content is a record and
   is accepted like any file.
5. **Submit is idempotent.** A Submit repeating an ID with an identical request (profile, name,
   media type, content SHA-256, extracts) returns the first job, completing any step a crash left
   undone, and its re-enqueue is stored once. The same ID with a different request is refused
   409. Concurrent Submits of one ID yield one job. The record is kept 30 days (a setting), then
   swept.
6. **GetJob** returns the job its events fold to; an unknown or invalid ID is 404 or 400.
7. **CancelJob** appends `cancel_requested`; when the job is still `queued`, it then appends
   `cancelled`, caused by it, and GetJob reads `cancelled`. An ended job is refused 409. Each
   append is guarded by the expected sequence.
8. **GetOutput** streams the content stored under an output's SHA-256; an absent one is 404.
9. **The upload.** go-web-sdk's handler is mounted under Bunrei's API with an upload lifetime of
   24 hours (a setting) and the chunk size as its append limit. Completing an upload stages the
   file under `input/` by its SHA-256, records the completed upload (SHA-256, name, media type,
   size), and answers with the upload ID; the handler then deletes the upload's rows and chunks.
10. **CompleteUpload** queues a job from a completed upload, a profile and the three optional
    extracts, as Submit does, and records the job on it; repeating it returns the same job. An
    unknown or expired completed upload is 404.
11. **Retention.** The sweeper reclaims expired uploads and their chunk rows, `upload/` objects
    at twice the upload lifetime, completed uploads past the lifetime, and submissions past their
    age.
12. **Problems.** Every refusal is an RFC 9457 problem whose type is
    `urn:standards-lab:bunrei:problem:<name>`, the names exported by the `client` module.
13. **The `client` module.** It holds `Client`, built over a base URL and an `http.Client`, with
    one method per Command and Query (Submit, CompleteUpload, CancelJob, GetJob, GetOutput,
    ListProfiles), each taking a `context.Context` and returning a refusal as an error carrying
    its problem. `Client.Upload` sends a file through go-web-sdk's upload client, resuming within
    its attempts, and returns the upload ID for CompleteUpload. `Submit`, `CompleteUpload` and
    `Queued` carry the three extracts. Its client tests, run against the served API, are the
    protocol's executable proof.

**Test seams**
- The upload store through `uploadtest.Run`, in the integration tier on PostgreSQL and SeaweedFS.
- The domain services over the memory broker and storage fakes in the unit tier.
- The `client` module's `Client` against the running service through the integration harness,
  covering every behavior above end to end, including an upload resumed after an interruption.

**Slices**
1. **Upgrade** capstone-bunrei: docinfer v0.6.0 in the root and `client`, docinfer/postgres
   v0.1.4, go-core v0.14.0, and go-web-sdk v0.20.0 fetched direct, as docproc/postgres v0.1.8
   was, since the module proxy doesn't list the tag yet; done when `mise run currency` exits 0
   or reports only held items, and `mise run check` passes (1).
2. **Schema and upload store**: Bunrei's migration set, the store passing `uploadtest`, its
   reclaimer and the `upload/` expiry (2, 3, 11).
3. **Submit**: staging, `queued` with three extracts, enqueue, the idempotency record and its
   sweep, the 413, 409 and cap refusals, problem types (4, 5, 11, 12).
4. **GetJob, CancelJob, GetOutput** routes (6–8).
5. **Upload and CompleteUpload**: the mounted handler, the Complete function, the
   completed-upload record and its sweep (9–11).
6. **The `client` module's `Client`**: every method, `Client.Upload`, problem constants, the
   integration tests through it (13).
7. **Release readiness**: `client`'s requirements tidy for a standalone import (no replace
   reaches a consumer), its package documentation lists the full API, and the README states how
   Mikoshi imports `client/v0.1.0`.

**Out of scope**
- ListProfiles' route and refusing an unknown profile (`jobs`); `Client.ListProfiles` ships and
  its route answers 404 until then.
- Running jobs: workers, pipelines, fencing a running job's cancellation, dead letters (`jobs`).
- Prompt override Queries and Commands (`prompts`).
- The event and result-batch stream (`event-stream`).
- Authentication, and the admin surface.
- Changes to any other repository; the contract notes' edits go to this goal's pending edits.

**Door**

One-way for the `client/v0.1.0` tag only: once pushed, Mikoshi and the module proxy hold it. The
routes, schema and settings stay two-way while no deployment holds data.

**Release**
- capstone-bunrei: `client/v0.1.0`.

## Progress

slices 3/7 committed · standards — · spec — · editor —

## Decisions

- capstone: contracts (`chunks`, `protocol`, `events`) are designed in `.foundation` at capstone-data-intelligence; Bunrei and Mikoshi build against them, and a task that reads one opens its summary with "Waits on".
- capstone: Bunrei's `client` module is released by tag (`client/v0.1.0` at `api`); Mikoshi imports the published version, never a replace directive.
- capstone: items join contexts by reference and a context covers its descendants; chunks and embeddings stay keyed by content and carry no context.
- metadata-text: Skype conversation records are not rendered (backlog).
- text-layer: a page's text layer stands in for its vision text when it holds at least the
  threshold's words, a word being a run of letters or digits. A page with no layer always goes
  to vision. 100 is a starting value, to be tuned from the live run's measurements of thin
  pages skipped and vision words lost. Rejected OCRmyPDF's any-text rule, which skips every thin
  page, and bitmap coverage, which needs a Python layout pass or a new docproc adapter.
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
- messaging-work-queue: the queue extends a running delivery itself and ends the handler's
  context once no extension has succeeded within `AckWait`; Bunrei's fencing (`jobs`) covers the
  overlap.
- messaging-work-queue: a work-queue stream takes one worker group per subject, so two kinds of
  work need two queues.
- messaging-work-queue: a dead letter is the original event on `bunrei.work.dead`, with attempt
  and last-error headers; `jobs`' worker on it appends `failed` and acks each.
- messaging-work-queue: `jobs` sets `docinfer.Options.Final` from `messaging.Worker.Final`.
- state: `api` owns Submit, CancelJob, GetJob's route, Submit's idempotency record, and
  `upload.Store`; `jobs` owns running jobs. GetJob finds no job past BUNREI_JOBS's 30 days.
- state: the root module builds against `client` through `replace ./client`; `api` adds `Client`
  and the Commands and tags `client/v0.1.0`.
- state: `jobs` writes `completed`'s `Manifest` from `Job.Results` plus each attempt's `Done`.
- state: the job root's callers pass UUID job IDs and set `causationid`; `Append` folds the
  decided event before appending, so one the fold can't apply appends nothing, and retries an
  unknown outcome itself (3 times); its subject prefix is `nats.prefix`, through `Options.Prefix`.
- state: blob objects expire through the root-level `expiry` package (no library pages a storage
  prefix by age) on go-database's `sweep`; startup refuses a content or input age at or below the
  renew age. Rejected a bucket lifecycle rule (docproc/storage's recommendation): go-storage can't
  set one, the stack creates no bucket, SeaweedFS support unconfirmed.
- state: docproc/postgres v0.1.8 is fetched direct: the module proxy and sumdb 404 the tag, which
  exists on the public remote.
- state: go-messaging unchanged; in-order delivery within a group goes to `event-stream`;
  following one job's subject for fencing goes to `jobs` (or polling `Read`).
- document-extraction: docinfer's `Options` carries `Document` and `Timeline` as optional
  `*Extract`s beside the per-source `Extract`, each with its own prompt and schema; Bunrei's
  Submit, CompleteUpload, and `queued` carry all three (`api`). Rejected a scope setting on one
  Extract.
- document-extraction: a text past `Extraction.WindowRunes` is windowed by `chunk.Splitter` and
  combined by repeated calls with the same Extract under a fixed combine instruction, every call
  cached by its own input's SHA-256. A rune bound, since the gateway can't tokenize for `agent`.
  No default; Bunrei's profile sets it at `jobs`. Rejected Athena's 10,000-character truncation,
  marking long texts incomplete, and langchaingo's map-reduce chain.
- document-extraction: `job` yields no checkpoint while `Document` is set; Bunrei's `jobs` resumes
  such a job from the start. Rejected carrying text SHA-256s in `Checkpoint`.

- api: `upload.Store` is a PostgreSQL row per upload with chunk objects at
  `upload/<upload>/<offset>/<append id>`, the key recorded in a chunk row, since go-storage has no
  conditional write. Rejected BUNREI_UPLOADS: a 4 MiB chunk exceeds NATS's 1 MB message limit.
- api: Complete stages the file under `input/` and records a completed upload; CompleteUpload
  queues from it and is idempotent per upload; the handler then deletes the upload; abandoned
  upload objects are swept at twice the 24-hour lifetime.
- api: Submit's idempotency record is a PostgreSQL row kept 30 days (BUNREI_JOBS's age); the same
  ID with a different request is refused 409.
- api: Submit and CompleteUpload append `queued`, then enqueue `{"job"}` with an ID derived from
  the job; `jobs` only consumes BUNREI_WORK.
- api: CancelJob appends `cancelled` itself while the job is `queued`; `jobs`' fencing ends a
  started job.
- api: ListProfiles' route and the unknown-profile refusal are `jobs`'; `client/v0.1.0` ships
  `Client.ListProfiles`.
- api: problem types are `urn:standards-lab:bunrei:problem:<name>`, exported by `client`.
- api: Submit's wire encoding is `client.Submit`'s JSON with base64 content; rejected multipart.
- api: the 256 KiB extraction cap covers the three extracts together.
- api: `Client.Upload` returns the upload ID and resumes within its attempts; it doesn't expose
  go-web-sdk's `Resource`.

## Pending edits

- coordinator · `docs/data-intelligence.md`, "Tool packaging": the image holds poppler-utils 22.02
  or later, whose pdfinfo docproc v0.10.0's document stage requires beside pdftotext and pdfdetach,
  and ImageMagick 7.1 or later, whose magick its vision stage requires, as the picture and
  office stages do. "Open items": `metadata` resolved by docproc v0.10.0.
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
- coordinator · `docs/data-intelligence.md`, "docinfer's configuration": add the vision
  layer-word threshold, cmd/docinfer's `vision.layer_words`, starting at 100, to be tuned from
  the thresholds docinfer's `docs/acceptance.md` measures ("The layer-word threshold").
- docs-corpus · `docs/baseline.md`, "v0.2.0": add the runs `docproc-v0.10.0/` (binaries
  `bin-v0.10.0/`) and `docinfer/text-layer/`; link docinfer's
  `docs/acceptance.md#docs-corpus-v020-with-the-text-layer-skip`.
- coordinator · `context/messaging.md`: "Open", "A stream-backed root": drop "(planned)";
  go-messaging builds the append and subject read as `messaging.Journal` (v0.4.0, nats/v0.4.0)
  and the work-queue package as `messaging.Queue` (v0.5.0, nats/v0.5.0); Bunrei's job root
  appends and reads through `messaging.Journal`, and its workers on the queue are `jobs`'. Retire
  "A dead-letter stream, deferred until a consumer needs replay": the queue dead-letters onto a
  subject on its own stream, and replay stays unbuilt. Add to "Open": a
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
- docproc · `context/bunrei.md`: "A job": the work queue, not the replica, extends a running
  delivery, and `docinfer.Options.Final` comes from `messaging.Worker.Final`. "A dead last
  delivery": closed; the queue dead-letters a delivery past `MaxDeliver` without running its
  handler, and Bunrei appends `failed` from the dead-letter subject. The stores diagram:
  BUNREI_WORK also holds the queue's dead-letter subject and the subjects under it. "Requests to
  go-messaging": v0.4.0's `messaging.Journal` meets "A publish on a subject the caller names" with
  a guarded append and a bounded read, and v0.5.0's queue meets the in-progress acknowledgement
  with its own extension of a running delivery.
- coordinator · roadmap: `state` summary drops "uploads"; `event-stream` takes in-order delivery
  within a group; `jobs` takes following one job's subject for fencing, emptying docproc's working
  directory at startup, ListProfiles, the unknown-profile refusal, cancelling a started job,
  and drops the enqueue.
- capstone-data-intelligence · `context/protocol.md` "Ownership" and "The client module";
  `context/events.md` "Ownership": `state` creates `client` with the job types; `api` completes and
  releases it. `client` requires docinfer v0.5.0 and docproc v0.10.0, and its `Version`, `1`, is
  the wire version `urn:standards-lab:bunrei:client:1` names.
- docproc · `context/bunrei.md` "State and subjects": keys are `content/ab/<sha256>` and
  `input/ab/<sha256>`, as docproc/storage lays them out; the retentions in the next edit; Bunrei's
  sweep replaces "The host sweeps content by age"; drop "The names and retentions are the `state`
  task's".
- coordinator · `docs/data-intelligence.md`: "Open items": drop `state`'s question; go-messaging
  v0.5.0's queue dead-letters a job whose last delivery died. "Bunrei", detail "State": BUNREI_JOBS
  keeps 30 days, BUNREI_WORK no max age; `content/` 67 days (renewed after 7), `input/` 30,
  `upload/` `api`'s; docproc's index 30, outbox and inbox 7; a sweep reactor deletes past each age,
  reclaiming blobs by prefix, not by a lifecycle rule. "Tool packaging": a deployment
  supplies the database password and storage key as secrets (Bunrei's README, "Configuration").
- coordinator · `context/data-intelligence.md` go-messaging asks: append and work queue built
  (v0.4.0, v0.5.0); remaining asks move to `event-stream` and `jobs`.
- capstone-data-intelligence · `context/protocol.md`, `events.md`, `athena.md`: docinfer v0.6.0
  builds both scopes as `Options.Document` and `Options.Timeline`; Submit, CompleteUpload, and
  `queued` carry all three extracts under the 256 KiB cap; `inference` carries `scope`.
- docproc · `context/bunrei.md`: "the extraction spec" becomes the three optional extracts.
- coordinator · `docs/data-intelligence.md`: add docinfer's `Extraction.WindowRunes`, which Bunrei
  sets at `jobs`.
- coordinator · roadmap backlog entries: picture metadata beyond EXIF (IPTC, maker notes, reverse
  geocoding); rendering Skype conversation records (`MediaMailConversation`: topic, start, end,
  missed) as metadata text; go-ai's default transcription instruction (sent when docinfer's
  prompt is empty) absent from docinfer's transcription cache key; a Mikoshi prompt-tuning view
  over Bunrei's prompt override Queries and Commands; appending several events atomically
  (JetStream atomic batch publish, once nats.go has a client API); a Postgres `SKIP LOCKED`
  provider of go-messaging's work-queue contract; docinfer cache retention; extraction in
  cmd/docinfer.
- capstone-data-intelligence · `context/protocol.md`: `api`'s 409s, problem URNs, extract cap,
  base64 Submit; `client` requires docinfer v0.6.0.
- docproc · `context/bunrei.md`; coordinator · `docs/data-intelligence.md` "State": `api`'s
  chunk key, `upload/` age; no BUNREI_UPLOADS.
