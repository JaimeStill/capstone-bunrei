# Bunrei

capstone.data-intelligence's processing service: files and records in; metadata, extracted
text, inferences, chunks, and embeddings out as ordered events closed by a manifest. It never
knows its caller.

Generated from [go-web-sdk-template](https://github.com/standards-lab/go-web-sdk-template). The
capstone's architecture is in the coordinator, `standards-lab/context/data-intelligence.md`; this
repository's working context is [`context/README.md`](context/README.md).

## Getting started

[mise](https://mise.jdx.dev/) provisions the toolchain and runs the tasks:

```sh
mise trust && mise install
mise run serve
```

The service logs `server ready` on `localhost:8080` (the `local` overlay binds loopback and runs
debug logging). From a second shell:

```sh
curl localhost:8080/healthz   # 200 {"status":"ok"}
curl localhost:8080/readyz    # 200 {"status":"ready","checks":[...]}
```

Ctrl-C drains in-flight requests and exits with `server stopped`.

## Tasks

Each task wraps a plain command, so the repository works without mise:

| Task | Command | What it does |
|------|---------|--------------|
| `mise run check` | build, vet, `gofmt -l`, `go fix -diff`, `go mod tidy -diff`, `go test -race`, `golangci-lint run`, `lint-context`, `lint-references` | The one read-only check, as CI runs it; the lints run at the project root and, when the project is nested in a git repository, at its top level |
| `mise run currency` | `bash scripts/currency.sh` | Report requirements, Go version, tools, and action pins behind their latest |
| `mise run upgrade` | `go mod edit -go=<current minor> -toolchain=none`, `go get <direct>@latest`, `go mod tidy`, `mise upgrade --bump --local` | Upgrade the go directive, requirements, and tools, rewriting `go.mod` and `mise.toml` |
| `mise run build` | `go build ./...` | Build the module |
| `mise run vet` | `go vet -tags integration ./...` | Compile-check and vet, the integration suite included |
| `mise run serve` | `go run ./cmd/server` | Run the service locally |
| `mise run test` | `go test -race ./...` | Run the unit tier |
| `mise run integration` | `docker compose up -d --wait --build`, then `go test -race -count=1 -tags integration ./integration/` | Run the integration tier against the built service on the isolated compose stack, then tear the stack down |
| `mise run fmt` | `gofmt -w .` | Format the source |
| `mise run tidy` | `go mod tidy` | Reconcile module requirements |
| `mise run lint` | `golangci-lint run --build-tags integration ./...` | Lint, the integration suite included |

## Testing

Two tiers. The unit tier is hermetic and runs on every pull request: `go test -race ./...`,
touching no service, network, or disk. The integration tier is the `integration` package: an
untagged harness that builds `cmd/server` once, runs it as a subprocess configured by `BUNREI_*`
variables on a port it reserved, and drives it through its HTTP surface, over the toolkit the
SDKs ship for the purpose (go-core's `process/processtest`, go-web-sdk's `webtest`). Where a
capability has no route, the harness composes the service's own nodes in the test process, and a
test opens its own PostgreSQL pool or NATS connection to read beneath the API. Under the
`integration` build tag, the suite asserts the service's behavior through its API. It runs on
merge to main and on demand, below the per-PR rate by design; a release is cut only from a main
whose run has passed.

Tests are black-box: golangci-lint's `testpackage` keeps every test in its package's `_test`
package, driving the exported API. `export_test.go` is the one white-box file allowed, and only
to export a clock or probe hook the API cannot reach deterministically.

`internal/app`'s graph tests check the composition itself. Each `Nodes` field names its node
(go-core's `graphtest.CheckNames`). A Build of the root set, `App.Roots` plus the lifecycle
configuration node, builds exactly what `Run` builds; in it the server is alone in the top layer,
a reactor sits below the server by its membership of the reactors group alone, and no order
between nodes is dropped but one: the server's order after the reactors group, while the group
is empty. Each check is shown failing on a fixture that breaks it, with the layer table.

The suite asserts the baseline (boot, both probes, and the drain) and, beside it, each built
capability: the schema's tables, the content and input stores' round-trips, the provisioned
streams, the job root over NATS, and a sweep pass deleting an aged object. The suite runs on
Bunrei's compose stack (NATS, PostgreSQL, and SeaweedFS): `mise run integration` boots it as an
isolated compose project on its own ports, runs the suite, and tears it down with its volumes on
exit, so a developer's stack and data are never touched. [Compose](compose/README.md) describes the stack.

## Configuration

Configuration layers in a fixed precedence, later sources winning:

1. `config.json` — the base file, every knob at its default.
2. `config.<BUNREI_ENV>.json` — the environment overlay; `mise.toml` sets `BUNREI_ENV=local`, which
   activates the committed `config.local.json` (loopback host, debug logging, and the addresses
   of the capstone's development stack: NATS on 4226, `postgres-bunrei` on 5439, and the
   SeaweedFS S3 gateway on 8335).
3. `secrets.json`, `secrets.<BUNREI_ENV>.json` — gitignored secret layers, holding the database
   password and the storage key; `secrets.example.json` shows their shape.
4. `BUNREI_*` environment variables — the final override, each named for its block and key:
   - `BUNREI_LOG_LEVEL`, `BUNREI_LOG_FORMAT`
   - `BUNREI_SERVER_HOST`, `BUNREI_SERVER_PORT`, the four server timeout variables, and
     `BUNREI_SERVER_TRANSFER_RATE`
   - `BUNREI_DATABASE_*`: go-database's PostgreSQL connection (`database`)
   - `BUNREI_STORAGE_*`: go-storage's S3 connection (`storage`), whose container is the bucket
     Bunrei ensures at startup
   - `BUNREI_NATS_*`, `BUNREI_NATS_QUEUE_*`: go-messaging's NATS broker for the BUNREI_JOBS stream
     (`nats`), and its work queue for the BUNREI_WORK stream (`nats_queue`), each with its own URL
     and owning its stream
   - `BUNREI_OBJECTS_CONTENT_PREFIX`, `BUNREI_OBJECTS_INPUT_PREFIX`,
     `BUNREI_OBJECTS_UPLOAD_PREFIX`, `BUNREI_OBJECTS_RENEW_AGE`
   - `BUNREI_SUBMISSIONS_DOCUMENT_LIMIT`, `BUNREI_SUBMISSIONS_RETENTION`
   - `BUNREI_UPLOADS_LIFETIME`, `BUNREI_UPLOADS_CHUNK_SIZE`
   - `BUNREI_SWEEP_INTERVAL`, `BUNREI_SWEEP_BATCH`, and `BUNREI_SWEEP_<KEY>_AGE` for `outbox`,
     `inbox`, `index`, `content`, and `input`
   - `BUNREI_READS_DEFAULT_SIZE`, `BUNREI_READS_MAX_SIZE`
   - `BUNREI_SHUTDOWN_TIMEOUT`

Bunrei owns four blocks besides `reads`. `objects` sets the bucket prefixes that docproc's
content (`content/`), each job's staged input (`input/`), and the resumable uploads' chunks
(`upload/`) are kept under, and the renew age (7 days) past which a put of an object already
present writes it again. `submissions` sets the small-document limit (`document_limit`, 4 MiB),
past which a document goes through an upload instead of Submit, and how long a Submit's
idempotency record is kept (`retention`, 30 days). `uploads` sets how long an upload, and then
its completed record, lasts (`lifetime`, 24 hours), and the most bytes one append carries
(`chunk_size`, 4 MiB); the sweeper reclaims `upload/` at twice the lifetime. `sweep` sets how often
the sweeper runs each reclaimer, the budget of one pass, and the age past which each reclaimer
deletes: published outbox rows and handled inbox rows at 7 days, docproc's index entries at 30,
objects under `content/` at 67, and objects under `input/` at 30. Startup refuses a content or
input age at or below the renew age, since an object still in use is rewritten only once it is
older than that.

Every file is optional — a deployment can run on the base file and environment variables alone,
or on environment variables only. Each file present decodes strictly: a key the `Config` types do
not declare fails the load.

## The client module

Bunrei serves its Commands, Queries, and resumable upload under `/api`. The nested `client`
module, `github.com/JaimeStill/capstone-bunrei/client`, is that protocol in Go; its
[package documentation](client/doc.go) lists the whole API and the route each method calls.

The module is released by tag, `client/v0.1.0` and on, with its own
[changelog](client/CHANGELOG.md). A caller such as Mikoshi imports the published version:

```sh
go get github.com/JaimeStill/capstone-bunrei/client@v0.1.0
```

```go
c, err := client.New("http://bunrei:8080/api", &http.Client{Timeout: 30 * time.Second})
if err != nil {
	return err
}
job, err := c.Submit(ctx, client.Submit{
	ID:        id,
	Profile:   "standard@1",
	Name:      "report.pdf",
	MediaType: "application/pdf",
	Content:   content,
})
if errors.Is(err, client.ErrContentTooLarge) {
	upload, uerr := c.Upload(ctx, "report.pdf", "application/pdf", file, client.UploadOptions{Attempts: 5})
	if uerr != nil {
		return uerr
	}
	job, err = c.CompleteUpload(ctx, client.CompleteUpload{Upload: upload, Profile: "standard@1"})
}
```

Bunrei's root module builds against the client in this repository through `replace ./client`
in its `go.mod`, which never reaches a caller: a caller requires the published tag, never a
`replace`. The client's `go.mod` has no `replace` and does not require the root module.

## Building out the service

The composition root, `internal/app`, is laid out as one file per layer of the architecture,
and those files are the build points:

- `infrastructure.go` for the services the application composes on
- `admin.go` for the administrative services
- `domain.go` for the domain services
- `reactors.go` for the process-lifetime entry points an occurrence drives
- `middleware.go` for the router-level middleware

The domain layer holds the job root, the `domain/jobs` package, which folds a job's events and
appends the next under the expected sequence, and the packages behind the API's routes. The
reactors layer holds the sweeper, whose object reclaimers are the `expiry` package. The nested
`client` module holds the protocol and its `Client`. Each package's documentation describes it.

[Building out the service](docs/building-out.md) covers how the layer files define and mount
their nodes, and walks through adding a domain service, an infrastructure service, an admin
service, a reactor, middleware, and configuration. It also lists the packages that may import a
provider.
