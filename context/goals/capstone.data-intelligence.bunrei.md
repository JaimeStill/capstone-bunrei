# goal · capstone.data-intelligence.bunrei

- **State:** brief ready
- **Task:** messaging-append
- **Branch:** messaging-append

## Tasks

1. [x] metadata
2. [x] metadata-text
3. [x] text-layer
4. [x] vision
5. [ ] messaging-append
6. [ ] messaging-work-queue
7. [ ] state
8. [ ] document-extraction
9. [ ] api
10. [ ] jobs
11. [ ] prompts
12. [ ] event-stream
13. [ ] tool-packaging
14. [ ] foundation-notes

## Task brief · messaging-append

**Problem**

Bunrei treats each job as a root Entity whose state is folded from the events on the job's own
subject, and a Command's transaction is one guarded append: it succeeds only when the subject's last
event is the one the caller has already folded. go-messaging can only publish an event under its
type and consume it through a durable subscription. It can't append to a subject the caller names,
check the expected position, or read one subject back in order, and Bunrei's `state` task needs all
three from a released go-messaging. JetStream already does the guarded append (nats.go's
`PublishMsg` with `WithExpectLastSequencePerSubject`) and reads a subject through an ordered
consumer, so go-messaging wraps both behind a standard-tier contract. The memory provider meets
the same contract as the test double.

**Behaviors**

1. The root package declares `Journal`, an interface beside `Broker`, not part of it, with `Append`
   and `Read`. `nats.Broker` and `memory.Broker` both implement it, and existing `Broker`
   implementations still compile.
2. `Append(ctx, subject, expect, e)` stores one event the caller has already stamped on the named
   subject. It succeeds only when the stream sequence of the subject's last event equals `expect`,
   where 0 means the subject has no event yet, and it returns the new event's stream sequence.
3. `Append` rejects an event that `messaging.Encode` rejects, a header value holding CR or LF, and a
   subject that breaks `CheckType` (an empty token or a wildcard). The nats broker also refuses a
   subject outside its `Prefix` as `event.Permanent`, naming the subject and the prefix. These
   checks run whether or not the broker is started; after them, an unstarted nats broker fails
   with `ErrNotStarted`.
4. When the subject's last sequence differs from `expect`, `Append` fails with an error matching the
   root sentinel `ErrConflict` under `errors.Is`, wrapped with the subject and the expected
   sequence. On nats, JetStream error codes 10071 and 10164 both map to it.
5. On a sequence mismatch, the provider first looks at the subject's last event. If that event has
   the appended event's source and id, `Append` succeeds and returns that event's sequence. A retry
   whose first ack was lost therefore succeeds on a single server and on a clustered stream alike.
6. The nats append sets `Nats-Msg-Id` exactly as `Publish` does, from the event's length-prefixed
   source and id. A repeat whose expected sequence matches returns the original's sequence; a stale
   one is `ErrConflict` unless the original is the subject's last (behavior 5). Memory checks the
   sequence before deduplicating, as one nats-server does, sharing a window with `Publish`.
7. `Read(ctx, subject, after)` returns an `iter.Seq2` over entries and errors. Each entry is a
   decoded event and its stream sequence. The read returns the subject's events after `after` (0
   for all), in sequence order, and ends at the subject's last event as it stood when the read
   began. An event appended during the read is not yielded. A subject with no events yields
   nothing.
8. When the read meets a message that doesn't decode, it yields the error and stops. Stopping
   iteration early releases whatever the read holds; on nats, that is the ordered consumer.
9. The nats read looks up the subject's last sequence once with `GetLastMsgForSubject`, then reads
   through an ordered consumer filtered to the subject and starting after `after`. An empty
   subject, or one whose last sequence is at or before `after`, creates no consumer.
10. Appended events reach the same broker's subscriptions, as they do on a JetStream stream. An
    unfiltered subscription receives them in sequence. A type filter matches an event's subject,
    so it receives an appended event only when that event's subject is one of its `Types`.
11. The memory broker keeps one log numbered like a stream, starting from 1. Each entry carries a
    subject: the caller's subject for an appended event and the type for a published one.
    Consumers match on the subject, so the behavior of `Publish` and `Subscribe` doesn't change.
12. `messagingtest.RunJournal(t, newJournal)` is an exported conformance entry point that proves
    behaviors 2 to 8 and 10, except 8's undecodable message, which each provider's own tests
    prove. Both providers call it; `Run` is unchanged.
13. The `messaging`, `memory`, `messagingtest` and `nats` package doc.go inventories list the new
    exports. The README and the usage page (appending to and folding a root) describe them. The
    internals page covers the conflict lookup, the order JetStream runs its sequence and
    deduplication checks in on a single server versus a cluster, and the read's bound. The
    context README's capability map gains an entry for the journal.
14. The base and nats changelogs each carry a v0.4.0 section listing the journal additions. The
    nats section also lists nats.go v1.56.0, which already sits under [Unreleased], and the
    `go-messaging` v0.4.0 requirement. The postgres changelog records the sqlate bumps under
    [Unreleased] only.

**Test seams**
- `messagingtest.RunJournal`, against the memory broker in the unit tier and the nats broker in
  the nats integration suite.
- The nats broker's unit tests for the checks that run before Start (behavior 3), and its
  integration tests for the ordered consumer: early stop, the read's bound, and no consumer for an
  empty subject.

**Slices**
1. **Upgrade go-messaging.** Move to sqlate v0.8.0, sqlate/postgres v0.9.0, sqlint v0.5.0 and
   marathon 0.24.1. Done when `mise run currency` exits 0 and `mise run check` passes. The
   postgres changelog notes the bumps under [Unreleased].
2. **The contract and the memory append.** Add `Journal` and `ErrConflict`. Memory log entries
   gain a subject and stream numbering, and memory consumers match on the subject. Implement the
   memory `Append` (behaviors 2 to 6). Add `RunJournal` with the append cases, and run the memory
   broker through it and through the unchanged `Run`.
3. **The memory read and subscription visibility.** Implement the memory `Read` (behaviors 7 and
   8). Add `RunJournal` cases for the read and for appended events reaching unfiltered and
   type-filtered subscriptions (behavior 10).
4. **The nats append.** Implement `Append` on `nats.Broker` (behaviors 2 to 6). The unit tests
   cover the checks that run before Start. The integration suite runs the append cases of
   `RunJournal`, including a retry whose ack was lost. Until the base module is tagged, the nats
   module builds against it through a temporary replace directive.
5. **The nats read.** Implement the ordered-consumer read (behaviors 7 to 9) and pass all of
   `RunJournal` in the integration suite.
6. **Documentation and release preparation.** Cover behaviors 13 and 14 and date the changelog
   sections. At SHIP, the base is tagged first, then the nats module's requirement moves to
   `go-messaging` v0.4.0, the replace directive is removed, and the nats module is tagged.

**Out of scope**
- Appending several events atomically, which would need JetStream's atomic batch publish.
- A durable subscription filtered by subject, in-order delivery within a delivery group, and
  following a subject live. These belong to Bunrei's `event-stream` and `state` tasks.
- Bunrei's streams, subjects, retention and `jobsequence`, which belong to the `state` task.
- The work-queue package, which belongs to `messaging-work-queue`.
- Any change to go-core: `Recorder.Emit` and `event.Sink` are not used.
- A postgres release.

**Door**

One-way for the `v0.4.0` and `nats/v0.4.0` tags: once the module proxy fetches them, they are
permanent and reach consumers outside these repositories. Everything before the tags is two-way.

**Release**

`v0.4.0`, `nats/v0.4.0`

## Progress

slices 6/6 committed · standards ✓ · spec ✓ (round 2: no gaps) · editor ✓
- escalation, spec review: a stale repeat returns ErrConflict unless the original is still the
  subject's last, memory checking the sequence first? yes. Behavior 8's undecodable case proven
  per provider, not in RunJournal? yes.

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
- messaging-append: the journal uses the broker's own stream, so an appended subject is under its
  `Prefix`, and a separate jobs stream means a second broker.
- messaging-append: appended events reach the same broker's subscriptions and a type filter
  matches on the subject, so a type-filtered subscription never receives a job event on the job's
  own subject.
- messaging-append: released as go-messaging `v0.4.0` and `nats/v0.4.0`; `messaging-work-queue`
  builds on them. The postgres sqlate bumps stay under [Unreleased].

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
- coordinator · `context/messaging.md`, "Open", "A stream-backed root": the expected-sequence
  append and the subject read are built in go-messaging v0.4.0 and nats/v0.4.0 as
  `messaging.Journal`; only the work-queue package stays planned. Add to "Open": a replicated
  stream deduplicates an append before checking its expected sequence, so a stale repeat within
  the window returns the original's sequence, not `ErrConflict`, which `state` relies on.
  "Assumptions", the outbox bullet: Bunrei appends a job's events through `messaging.Journal`.
- coordinator · `docs/messaging.md`, "A stream-backed root": drop "Planned."; the append is
  built as `messaging.Journal` (go-messaging's `docs/usage.md`, "Appending to a stream-backed
  root"), and the work-queue package stays planned under `messaging-work-queue`.
- coordinator · `roadmap/capstone.data-intelligence.bunrei.toml`: no task builds the go-messaging
  asks `messaging-append` left out (a durable consumer filtered by subject, in-order delivery
  within a delivery group); the `state` or `event-stream` summary sizes them.
- coordinator · roadmap: a backlog entry for appending several events atomically (JetStream
  atomic batch publish, once nats.go has a client API).
- docproc · `context/bunrei.md`, "Requests to go-messaging": go-messaging v0.4.0's
  `messaging.Journal` meets "A publish on a subject the caller names", with a guarded append and
  a bounded read.
