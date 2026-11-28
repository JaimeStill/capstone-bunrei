# goal · capstone.data-intelligence.bunrei

- **State:** building
- **Task:** document-extraction
- **Branch:** document-extraction

## Tasks

1. [x] metadata
2. [x] metadata-text
3. [x] text-layer
4. [x] vision
5. [x] messaging-append
6. [x] messaging-work-queue
7. [x] state
8. [ ] document-extraction
9. [ ] api
10. [ ] jobs
11. [ ] prompts
12. [ ] event-stream
13. [ ] tool-packaging
14. [ ] foundation-notes

## Task brief · document-extraction

**Problem**

docinfer only extracts per source: once for each output text, image description, and transcript.
Athena's document summary and its video summary and key moments need two more scopes: one over a
job's whole text, and one along a video's timeline of frames and transcript. protocol.md and
events.md plan both for docinfer's next minor. Bunrei's `api` and `jobs` and Mikoshi's `ingest`
read their shapes. A long document can exceed the extraction model's 131,072-token context, and
the gateway can't count that model's tokens.

**Behaviors**
1. `Options` gains `Document` and `Timeline`, each an optional `*Extract` beside `Extract`. Each
   is checked as `Extract` is. When either is set without `Config.Extraction`, the run stops,
   permanent, before reading an input.
2. `Extraction` gains `WindowRunes`, with no default. `New` refuses a value below 1 when
   Extraction is set, matching `ErrRequest`. The value is not part of the identity.
3. A job's document text is every non-empty per-source text joined in pull order: output texts,
   image descriptions, and transcripts, but never metadata texts. A page whose layer skipped
   vision contributes its layer text. Once inputs end and every input has settled, the document
   text is extracted once with `Document`, before the counts.
4. A text within `WindowRunes` takes one call. A longer text is cut by `chunk.Splitter` under
   `WindowRunes`, and each window is extracted with the same `Extract`. A combine call then reads
   the windows' replies in order, under docinfer's fixed combine instruction and the `Extract`'s
   schema. Combine calls repeat over runs of replies until one reply remains.
5. Every window and combine call is keyed by the extraction's identity and the SHA-256 of that
   call's own input. The result comes from the cache or the model, and is then held. A cached
   rerun asks the model nothing. The combine instruction's version is part of the identity's
   request version.
6. Each call follows the existing failure table and `Final` rule. A window that is refused,
   failed, or incomplete is listed by index in `Failed`, and combine reads only the windows with
   text. The document is extracted when every call extracted. Otherwise its outcome is failed,
   refused, or incomplete, in the transcript rule's order.
7. With `Timeline` set, each video-stage part gets one timeline extraction:
   - Frame descriptions sit at their `MetaTime`.
   - The transcript is cut at the join's seams into each window's words, placed at that window's
     `MetaStart`–`MetaEnd`.
   - Lines are ordered by time as `[m:ss–m:ss] heard: …`, the window's span, and
     `[m:ss] seen: …`.
   - A long timeline is windowed and combined as in behavior 4.
   - An audio-stage part gets no timeline. A video part with neither descriptions nor a
     transcript gets no extraction.
8. A timeline extraction is yielded after its video part's own event and after its frames' and
   transcript's inferences. The video part's own event settles once its timeline is yielded;
   `Pending` is unchanged.
9. `Inference` gains `scope`, `document` or `timeline`, omitted for per-source inferences:
   - Only each scope's final inference is yielded. Window results are cached, not yielded.
   - The document inference's Part is the depth-0 part. The timeline inference's Part is the
     video part.
   - `ContentSHA256` lists the SHA-256s of the texts read, in order.
   - Usage and requests are summed over every call.
   - Neither scope's text is chunked.
10. `Progress` streams for each call, carrying that call's input SHA-256. A cached call yields no
    progress.
11. `ExtractionCounts` counts each final inference under its outcome, and every call in Requests
    and Held.
12. `job.Runner` yields no `Checkpoint` while `Options.Document` is set, so a redelivery replays
    the job from the start through docproc's index and docinfer's cache.
13. A golden key pins the combine request. The package documentation and the failure table cover
    both scopes. The changelog's v0.6.0 section is the release notes.

**Test seams**
- `Pipeline.Run` over `MemoryCache` and dispatch's memory content, with go-ai's `aitest` serving
  the extraction replies. It covers window and combine calls, the failure-table rows, a cached
  rerun, timeline order from window and frame metadata, and outcome and `Failed`.
- `job.Runner.Run`, for the checkpoint rule.
- The live extraction tier (`integration` tag, agent row): one document longer than a small
  `WindowRunes`, and one short video.

**Slices**
1. cmd/docinfer: go-core v0.12.0 → v0.13.0. Done when docinfer's currency command exits 0 and the
   check passes.
2. The new `Options` fields, `Extraction.WindowRunes`, `Inference.scope`, and their checks in
   `New` and `Run`.
3. Document scope within one window: assembling the text, the call, the cache, and the outcome.
4. Windowing and combining: the windows, the combine calls, `Failed`, and summed usage and
   requests.
5. Timeline scope: the timeline built from frames and transcript windows, through the same call
   path.
6. `job`: no checkpoints while `Document` is set.
7. Package documentation, the golden key, the live tier, and the changelog.

**Out of scope**
- Extraction configuration in cmd/docinfer (backlog entry).
- Bunrei's starting `WindowRunes` and its client shapes (`api`, `jobs`).
- Timelines for audio-only parts.
- Word-level transcript times.
- Athena's "detected" events.
- Chunking or embedding either new scope's text.

**Door**

One-way at release: the tags publish v0.6.0's shapes, and `Inference.scope` and the new `Options`
fields become wire and API contracts. The code before the tags is two-way.

**Release**
- docinfer `v0.6.0`
- `postgres/v0.1.4`: requires docinfer v0.6.0
- `cmd/docinfer/v0.4.1`: go-core v0.13.0, docinfer v0.6.0
- `acceptance` stays untagged; its requirement moves to v0.6.0

## Progress

slices 7/7 committed · standards ✓ · spec ✓ (2 gaps settled by escalation) · editor ✓
- escalation, spec round 1: video part settlement hold on its timeline; heard-line span format: keep the hold, keep the span (both recs)
- live tier: checkHeld fixed for held extraction calls and inference order (14fc8d1); passes on the gateway

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
  `upload.Store` (a PostgreSQL row with chunks at `upload/<upload>/<offset>`, or a BUNREI_UPLOADS
  stream, settled there); `jobs` owns running jobs. GetJob finds no job past BUNREI_JOBS's 30 days.
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
- docs-corpus · `docs/baseline.md`, "v0.2.0": the run directory also holds docproc v0.10.0's run,
  `docproc-v0.10.0/`, with its binaries in `bin-v0.10.0/`, and docinfer's text-layer run in
  `docinfer/text-layer/`; link docinfer's
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
- coordinator · roadmap: `state` summary drops "uploads"; `api` takes `upload.Store` and Submit's
  idempotency record; `event-stream` takes in-order delivery within a group; `jobs` takes
  following one job's subject for fencing and emptying docproc's working directory at startup.
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
  missed) as metadata text; a note that go-ai's built-in chat-mode transcription instruction, which
  docinfer sends when its transcription prompt is empty, is not in docinfer's transcription cache
  key, so a go-ai change to it doesn't miss held transcripts; a Mikoshi prompt-tuning view over
  Bunrei's prompt override Queries and Commands; appending several events atomically (JetStream
  atomic batch publish, once nats.go has a client API); a Postgres `SKIP LOCKED` work-queue provider
  that meets go-messaging's work-queue contract and conformance suite; docinfer cache retention;
  extraction in cmd/docinfer.
