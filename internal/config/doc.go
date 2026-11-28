// Package config declares the service's layered configuration.
//
//   - [Config] is the root: the library capability blocks (log, server,
//     go-database's connection block, whose name is required, go-storage's
//     connection block, whose container is required, and go-messaging's NATS
//     broker and work-queue blocks, nats and nats_queue, each owning its
//     stream and requiring its stream name and subjects), the service-owned
//     object layout, read policy, upload policy, submission policy, and
//     sweep policy, and the shutdown timeout.
//     The timeout comes from go-core's lifecycle.Config, embedded by value
//     and untagged, so shutdown_timeout is a top-level key, and Config.Config
//     is the block the composition root hands the lifecycle Coordinator. Its
//     Merge and Finalize delegate to each block's own, and its Finalize
//     checks the sweep's object ages against the objects block's renew age.
//   - [ObjectsConfig] is the service-owned object layout, with its own Merge
//     and Finalize: the content, input, and upload prefixes in the storage
//     container and the content and input stores' shared renew age. [ObjectsEnv] records the
//     environment-variable names its Finalize read.
//   - [ReadsConfig] is the service-owned read policy, with its own Merge and
//     Finalize; its Limits method hands the policy to a handler as
//     web.Limits. [ReadsEnv] records the environment-variable names its
//     Finalize read.
//   - [UploadsConfig] is the service-owned upload policy, with its own
//     Merge and Finalize: the lifetime of a resumable upload, and through
//     its ObjectAge method the age the sweeper reclaims the upload prefix's
//     objects at, twice the lifetime. [UploadsEnv] records the
//     environment-variable name its Finalize read.
//   - [SubmissionsConfig] is the service-owned submission policy, with its
//     own Merge and Finalize: the small-document limit, the most bytes a
//     submitted document decodes to, and the retention of a Submit's
//     idempotency record. [SubmissionsEnv] records the environment-variable
//     names its Finalize read.
//   - [SweepConfig] is the service-owned sweep policy, with its own Merge
//     and Finalize: the sweeper's interval and pass budget, and the age of
//     each reclaimer, the outbox, the inbox, docproc's index, and the
//     content and input prefixes. [SweepEnv] records the
//     environment-variable names its Finalize read.
//   - [Load] reads the layered files, decoding each strictly, and finalizes
//     the result under the service's env prefix.
//
// The unexported envPrefix const is the single place a seeded service
// renames its environment namespace.
package config
