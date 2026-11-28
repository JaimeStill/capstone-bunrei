// Package submissions is the Submit Command: a whole document, sent inline as the base64 content
// of [client.Submit]'s JSON, queued as one job, served as POST /api/jobs.
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
// The package's API:
//
//   - [Service], built by [New]: [Service.Submit], [Service.Limit], and [Service.Reclaimer].
//   - [Reclaimer]: the expired submission records' go-database sweep.Reclaimer.
//   - [Routes]: the route group the composition root mounts, each refusal a problem typed by
//     one of client's Problem constants.
//   - [ExtractionLimit]: the most bytes the three extracts marshal to together.
//   - [ErrInvalid], [ErrContentTooLarge], [ErrExtractionTooLarge], and [ErrConflict].
package submissions
