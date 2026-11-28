// Package client is Bunrei's protocol in Go: the [Client] that calls Bunrei's API, and the types
// Bunrei and its callers exchange, as the Queries and Commands return them and as the job's
// events carry them. It is a nested module of its own, so a caller imports it without Bunrei's
// dependencies, and Bunrei imports none of its callers.
//
// [New] builds a [Client] over the base URL the API's routes hang from, an absolute http or https
// URL such as http://bunrei:8080/api, and an http.Client the caller owns and configures. The
// Client has one method per Command and Query, each sent under the base URL:
//
//   - [Client.Submit]: POST /jobs;
//   - [Client.CompleteUpload]: POST /uploads/{upload}/complete;
//   - [Client.CancelJob]: POST /jobs/{id}/cancel;
//   - [Client.GetJob]: GET /jobs/{id};
//   - [Client.GetOutput]: GET /outputs/{sha256}, the content streamed;
//   - [Client.ListProfiles]: GET /profiles, each [Profile] Bunrei serves.
//
// [Client.Upload] sends a document past the small-document limit as a resumable upload: POST
// /uploads creates it, PATCH /uploads/{upload} appends each chunk, and HEAD /uploads/{upload}
// reads the offset the server holds. A send cut off partway, or answered 5xx, 429, or 409,
// resumes from that offset, and [UploadOptions] sets how many such failures in a row it bears,
// the delay before each retry, and the chunk size. It returns the completed upload's ID, which
// [CompleteUpload] names and [Client.CompleteUpload] queues as a job. When the attempts run out,
// the server keeps the partial upload until its lifetime ends.
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
// The error type is [Problem]. A refusal is an RFC 9457 problem whose type is [ProblemPrefix]
// followed by its name: [ProblemInvalidRequest], [ProblemContentTooLarge],
// [ProblemExtractionTooLarge], [ProblemSubmitConflict], [ProblemJobNotFound], [ProblemJobEnded],
// [ProblemOutputNotFound], and [ProblemUploadNotFound]. The Client returns one as a [*Problem],
// and errors.Is matches it against the sentinel of its type: [ErrInvalidRequest],
// [ErrContentTooLarge], [ErrExtractionTooLarge], [ErrSubmitConflict], [ErrJobNotFound],
// [ErrJobEnded], [ErrOutputNotFound], and [ErrUploadNotFound].
//
// Each type's JSON is its wire shape, with snake_case member names, at the wire version
// [Version], 1, which grows by addition only. Bunrei's events name it in their dataschema,
// urn:standards-lab:bunrei:client:1.
package client
