// Package client is Bunrei's protocol in Go: the types Bunrei and its callers exchange, as the
// Queries and Commands return them and as the job's events carry them. It is a nested module of
// its own, so a caller imports it without Bunrei's dependencies, and Bunrei imports none of its
// callers.
//
// The job's types:
//
//   - [Job]: a job as the fold of its events reads it, which GetJob returns;
//   - [State]: where a job stands, one of [StateQueued], [StateRunning], [StateCompleted],
//     [StateFailed], and [StateCancelled];
//   - [Manifest]: what a completed job's results add up to, which bunrei.job.completed carries;
//   - [Attempt]: one attempt's entry in the manifest.
//
// The Commands:
//
//   - [Submit]: a whole document queued as a job under the caller's idempotency ID;
//   - [CompleteUpload]: a completed upload queued as a job.
//
// The data of the job's events, each bunrei.job. followed by the event's name:
//
//   - [Queued]: queued's, the job's input and its three extracts;
//   - [Started]: started's, the attempt, the replica, and the resolved docinfer Profile;
//   - [AttemptFailed]: attempt.failed's, the attempt and its error;
//   - [Failed]: failed's, the attempt and its error.
//
// A refusal is an RFC 9457 problem whose type is [ProblemPrefix] followed by its name:
// [ProblemInvalidRequest], [ProblemContentTooLarge], [ProblemExtractionTooLarge], and
// [ProblemSubmitConflict].
//
// Each type's JSON is its wire shape, with snake_case member names, at the wire version
// [Version].
package client
