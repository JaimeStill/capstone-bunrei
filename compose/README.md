# Compose

Bunrei's integration stack, defined in `compose.yml` at the repository root and the files under
this directory, one file per service. `compose.yml` names the project (`capstone-bunrei`) and
lists `include:` entries; each included file owns one service's port and volume.

The stack serves the integration tier only. Bunrei runs locally against the capstone's own stack
(capstone-data-intelligence's `context/stack.md`), and this stack duplicates that stack's image
pins and settings so the integration tier runs against the same servers. It gives Bunrei a server
to reach and nothing more: it creates no JetStream stream, database schema, or S3 bucket, since
Bunrei creates each of its own.

Every service builds from its own `compose/<service>/Dockerfile`, whose `FROM` line is the
service's one image pin. No `image:` line names an image, and `mise run currency` reads the `FROM`
lines. The Dockerfile bakes in the service's configuration, environment, command, and a
`HEALTHCHECK` that defines readiness, so compose bind-mounts no configuration and `--wait` waits
until each service can do its job. Every `up` passes `--build`, so an edited Dockerfile or
configuration file takes effect on the next start.

## Services

Every port binds `127.0.0.1` only, and the environment variable overrides the host port. The
defaults sit clear of the capstone stack's ports (4226, 5439, 5442, 8335) and of go-web-service's
integration ports, so the three stacks run side by side.

| Service | Host port | Variable | Reached from the host at |
|---|---|---|---|
| `nats` | 4227 | `NATS_PORT` | `nats://127.0.0.1:4227` |
| `postgres` | 5443 | `POSTGRES_PORT` | `postgres://app:app@127.0.0.1:5443/app` |
| `seaweedfs` | 8338 | `SEAWEEDFS_S3_PORT` | `http://127.0.0.1:8338` |

- **NATS** (`compose/nats/`) runs JetStream on file storage under `/data`, from
  `nats-server.conf`. It takes no credentials and keeps the default 1 MB `max_payload`. Its
  monitoring endpoint listens on the container's loopback only, where the health check waits for
  JetStream itself (`/healthz?js-enabled-only=true`), not merely the listener, so a stream Bunrei
  creates on start never races the server.
- **PostgreSQL** (`compose/postgres/`) runs as user, password, and database `app`. Its health
  check runs `pg_isready` over TCP, not the socket, because the first start's initialization runs
  a temporary server on the socket alone, which would report ready before the real server starts.
  The capstone's servers build pgvector in, which Mikoshi needs; Bunrei's schema uses no extension,
  so this server is the plain image.
- **SeaweedFS** (`compose/seaweedfs/`) runs `weed mini`, whose S3 gateway takes access key
  `admin`, secret `secret`, signed for region `us-east-1`. Bucket auto-creation is off, so a write
  never creates a bucket; Bunrei ensures its own. The health check signs a request to the gateway
  with curl, so the stack is ready only once the gateway accepts those credentials.

The credentials are published development values, committed in the clear as
`architecture/principles/development-secrets.md`, "What is not a secret", allows.

## `mise run integration`

The task runs the stack as its own compose project, `capstone-bunrei-integration`, so its
containers and volumes never touch a stack started by hand from this directory. It builds the
stack and waits for every health check (`docker compose up -d --wait --build`), runs the
integration suite, and on exit, pass or fail, tears the stack down with its volumes
(`docker compose down -v`), so every run starts empty. When the suite or the stack fails, it
prints the stack's logs first. The port variables above, set in the task, carry the stack's
addresses to the suite. CI's integration job runs the same task.
