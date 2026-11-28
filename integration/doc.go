// Package integration is the service's integration tier: the harness that
// runs the composed service as the binary and drives it through its
// production seams, and, under the integration build tag, the suite that
// asserts the service's behavior through its API. The harness is built on
// the SDKs' test toolkits, go-core's processtest and go-web-sdk's webtest;
// nothing in the runtime exists for the tests' sake.
//
//   - [Main] is the suite's TestMain: it builds cmd/server once per run.
//   - [Options] holds the BUNREI_* overrides one service process adds.
//     The harness points every process at the compose database through
//     BUNREI_DATABASE_HOST, BUNREI_DATABASE_PORT, and
//     BUNREI_DATABASE_PASSWORD, at the compose S3 gateway through
//     BUNREI_STORAGE_ENDPOINT, BUNREI_STORAGE_ACCOUNT, and
//     BUNREI_STORAGE_KEY, and at the compose NATS server through
//     BUNREI_NATS_URL and BUNREI_NATS_QUEUE_URL, taking each from its own
//     environment when set, as mise run integration sets them, and
//     otherwise from the compose stack's defaults.
//   - [NATSURL] is that NATS server, so a test can inspect the streams the
//     service provisions.
//   - [Database] opens the harness's own pool on that database, to read
//     beneath the API.
//   - [Compose] loads the configuration as the binary does, under the
//     same variables, and returns the App over it, unbuilt, so a test can
//     build and drive a node no route reaches.
//   - [Start] runs the service on a reserved port and returns once it is
//     live; [Launch] returns without waiting, and [Service.Ready] waits.
//   - [Service] is one running process: [Service.Addr], [Service.URL], and
//     [Service.Client] reach it, and the embedded processtest.Process
//     stops it and reads its output and exit code.
//
// The harness is untagged, so the unit tier type-checks it on every pull
// request; the suite files carry the integration tag.
package integration
