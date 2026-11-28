# capstone-bunrei context

Bunrei (分霊, a kami's spirit divided so it can live in a new shrine while the original stays
whole) is capstone.data-intelligence's processing service. It never knows its caller: it turns
files and records into metadata, extracted text, inferences, chunks, and embeddings with docproc,
docinfer, and go-ai through Torii, and emits them as ordered events closed by a manifest, leaving
the source untouched.

It is generated from go-web-sdk-template. Its goal, `capstone.data-intelligence.bunrei`, lives in the coordinator's
roadmap, and its record is [`goals/capstone.data-intelligence.bunrei.md`](goals/capstone.data-intelligence.bunrei.md). The capstone's
architecture and scope are `standards-lab/context/data-intelligence.md`, and its design detail
`standards-lab/docs/data-intelligence.md`.

## Capability map

- **Protocol and client module** (planned): the nested `client` module holds the job's types; the `capstone.data-intelligence.bunrei` goal's `api` task adds the Queries, Commands, and `Client`, `standards-lab/roadmap/capstone.data-intelligence.bunrei.toml`.
- **Chunked upload** (planned): the `capstone.data-intelligence.bunrei` goal, `standards-lab/roadmap/capstone.data-intelligence.bunrei.toml`.
- **Job root** (built): the [`domain/jobs`](../domain/jobs/doc.go) package loads a job by folding the events on its `bunrei.job.<job>` subject and appends the next event under the subject's expected last sequence.
- **Stores and retention** (built): PostgreSQL holds docproc's index, docinfer's cache, and go-messaging's outbox and inbox, migrated at startup by the schema node; S3 blob storage holds docproc's content under `content/` and each job's staged input under `input/`; JetStream holds the BUNREI_JOBS and BUNREI_WORK streams; the sweeper reactor deletes rows and objects past their configured ages, through the [`expiry`](../expiry/doc.go) package for blobs; the composition root's [package documentation](../internal/app/doc.go) describes each.
- **Jobs and work queue** (planned): workers will run jobs from the BUNREI_WORK stream, go-messaging's `messaging.Queue`, which Bunrei provisions; the `capstone.data-intelligence.bunrei` goal's `jobs` task, `standards-lab/roadmap/capstone.data-intelligence.bunrei.toml`.
- **Processing profiles** (planned): the `capstone.data-intelligence.bunrei` goal, `standards-lab/roadmap/capstone.data-intelligence.bunrei.toml`.
- **Prompt overrides** (planned): an active override stored at runtime replaces a profile's default vision or transcription prompt; the `capstone.data-intelligence.bunrei` goal's `prompts` task, `standards-lab/roadmap/capstone.data-intelligence.bunrei.toml`.
- **Event and result-batch stream** (planned): the `capstone.data-intelligence.bunrei` goal, `standards-lab/roadmap/capstone.data-intelligence.bunrei.toml`.
- **Tool image** (planned): the `capstone.data-intelligence.bunrei` goal, `standards-lab/roadmap/capstone.data-intelligence.bunrei.toml`.
- **Metadata capture** (planned): docproc's format stages capture metadata and Bunrei parses no format; Bunrei's output events carry docproc's metadata records, `docproc/context/bunrei.md`; the `capstone.data-intelligence.bunrei` goal, `standards-lab/roadmap/capstone.data-intelligence.bunrei.toml`.
