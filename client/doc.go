// Package client is Bunrei's protocol in Go: the [Client] that calls Bunrei's API, and the types
// Bunrei and its callers exchange, as the Queries and Commands return them and as the job's
// events carry them. It is a nested module of its own, so a caller imports it without Bunrei's
// dependencies, and Bunrei imports none of its callers.
//
// The Client, which [New] builds over the API's base URL and the caller's http.Client, has one
// method per Command and Query, each sent under the base URL:
//
//   - [Client.Submit]: POST /jobs;
//   - [Client.CompleteUpload]: POST /uploads/{upload}/complete;
//   - [Client.CancelJob]: POST /jobs/{id}/cancel;
//   - [Client.GetJob]: GET /jobs/{id};
//   - [Client.GetOutput]: GET /outputs/{sha256}, the content streamed;
//   - [Client.ListProfiles]: GET /profiles, each [Profile] Bunrei serves.
//
// [Client.Upload] sends a large document as a resumable upload under /uploads, resuming a send
// cut off partway within its [UploadOptions], and returns the ID [Client.CompleteUpload] takes.
//
// The job's types:
//
//   - [Job]: a job as the fold of its events reads it, which GetJob returns;
//   - [State]: where a job stands, one of [StateQueued], [StateRunning], [StateCompleted],
//     [StateFailed], and [StateCancelled];
//   - [Manifest]: what a completed job's results add up to, which bunrei.job.completed carries;
//   - [Attempt]: one attempt's entry in the manifest;
//   - [Profile]: a processing profile and the docinfer Profile it resolves to, which
//     ListProfiles returns.
//
// The Commands:
//
//   - [Submit]: a whole document queued as a job under the caller's idempotency ID;
//   - [CompleteUpload]: a completed upload queued as a job, named by the [Upload] its completing
//     request answers.
//
// The data of the job's events, each bunrei.job. followed by the event's name:
//
//   - [Queued]: queued's, the job's input and its three extracts;
//   - [Started]: started's, the attempt, the replica, and the resolved docinfer Profile;
//   - [AttemptFailed]: attempt.failed's, the attempt and its error;
//   - [Failed]: failed's, the attempt and its error.
//
// A refusal is an RFC 9457 problem whose type is [ProblemPrefix] followed by its name:
// [ProblemInvalidRequest], [ProblemContentTooLarge], [ProblemExtractionTooLarge],
// [ProblemSubmitConflict], [ProblemJobNotFound], [ProblemJobEnded], [ProblemOutputNotFound], and
// [ProblemUploadNotFound]. The Client returns one as a [*Problem], and errors.Is matches it
// against the sentinel of its type: [ErrInvalidRequest], [ErrContentTooLarge],
// [ErrExtractionTooLarge], [ErrSubmitConflict], [ErrJobNotFound], [ErrJobEnded],
// [ErrOutputNotFound], and [ErrUploadNotFound].
//
// Each type's JSON is its wire shape, with snake_case member names, at the wire version
// [Version].
package client
