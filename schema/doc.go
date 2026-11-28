// Package schema is Bunrei's own migration set: the tables the service's
// domains keep, applied at startup by the composition root's schema node
// after docproc's, docinfer's, and go-messaging's sets, so a migration may
// reference their objects and never the reverse.
//
//   - [Migrations] returns the set, NNNN_name.{up,down}.sql under
//     migrations/, recorded under the set name [Source] in sqlate's
//     default history table.
//
// The set holds the upload table, one row per resumable upload in
// progress; the upload_chunk table, one row per committed chunk naming the
// object that holds its bytes; the submission table, one idempotency
// record per Submit ID naming the job it queued; and the completed_upload
// table, one record per completed upload naming its staged file and, once
// CompleteUpload queues it, its job.
package schema
