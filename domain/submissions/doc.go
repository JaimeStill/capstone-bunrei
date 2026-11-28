// Package submissions is the Commands that queue a job: Submit, a whole document sent inline as
// the base64 content of [client.Submit]'s JSON, served as POST /api/jobs; and the upload's two
// steps, Complete, the upload handler's completion of a resumable upload, and CompleteUpload, a
// completed upload queued with [client.CompleteUpload]'s profile and extracts, served as POST
// /api/uploads/{id}/complete.
//
// Submit stages the document under the input prefix by its SHA-256, appends the job's
// bunrei.job.queued event through the job root, and puts the job on the work queue with a
// bunrei.work event whose id is the job's ID, so a repeated enqueue is stored once within the
// queue's deduplication window. It is idempotent per the caller's ID: a PostgreSQL row per ID,
// the submission record, names the job the first Submit minted and a digest of its request, so
// a retry with the same request returns that job, finishing any step a crash left undone, and a
// retry with another request is refused. The record expires after the retention, and the
// [Reclaimer] go-database's sweeper runs deletes it.
//
// Complete stages a completed upload's file under the input prefix by its SHA-256 and records
// the completed upload in a PostgreSQL row keyed by the upload's ID, with the file's SHA-256,
// name, media type, and size; a second completion of the upload finds the row in place.
// CompleteUpload queues a job from that row as Submit does, its queued event Submit's shape, and
// records the job and a digest of its request on the row, so a repeat with the same profile and
// extracts returns that job and one with others is refused. The row expires the upload lifetime
// after its completion, and the [Reclaimer] [Service.UploadReclaimer] returns deletes it.
//
// The package's API:
//
//   - [Service], built by [New]: [Service.Submit], [Service.Complete],
//     [Service.CompleteUpload], [Service.Limit], [Service.Reclaimer], and
//     [Service.UploadReclaimer].
//   - [Reclaimer]: the go-database sweep.Reclaimer of the expired submission records, and of the
//     expired completed upload records.
//   - [Routes] and [UploadRoutes]: the route groups the composition root mounts, each refusal a
//     problem typed by one of client's Problem constants.
//   - [ExtractionLimit]: the most bytes the three extracts marshal to together.
//   - [ErrInvalid], [ErrContentTooLarge], [ErrExtractionTooLarge], [ErrConflict], and
//     [ErrUploadNotFound].
package submissions
