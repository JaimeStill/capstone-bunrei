// Package jobs is the job root: a job's state is its events, kept in order on the subject prefix,
// bunrei.job unless [Options] sets another, followed by a dot and the job's ID, and read and
// appended through go-messaging's Journal.
//
// Loading a job folds its events. Every event advances the job's gap-free sequence by one, which
// becomes the job's Sequence and the next event's jobsequence: a count the fold derives, not the
// stream sequence. queued sets the job's input and state queued; started sets state running and
// the attempt, and adds an attempt entry; attempt.failed records its error and returns the job to
// queued; cancel_requested marks cancellation requested; completed, failed, and cancelled end the
// job; checkpoint keeps its After. A batch folds from its batchdigest and batchchunks headers
// alone, never its data, into the manifest's batch count, chunk count, and digest, across every
// attempt. part, output, warning, and inference only advance the sequence.
//
// Appending asks the caller to decide the next event from the folded job, stamps it, and appends
// it guarded by the subject's last stream sequence, deciding again after another writer's append.
//
// Over them the job root serves two routes: GetJob, the job its events fold to, and CancelJob,
// which appends cancel_requested and, while the job is still queued, cancelled, caused by it. A
// started event that lands first wins the guarded append, and the job runs on with its
// cancellation requested.
//
// The package's API:
//
//   - [Service], built by [New] over a Journal with [Options]: [Service.Load],
//     [Service.Append], [Service.Get], and [Service.Cancel].
//   - [Routes]: the route group the composition root mounts, each refusal a problem typed by
//     one of client's Problem constants.
//   - [Job]: the folded job, a [client.Job] and the state the fold keeps beside it, with
//     [Job.Ended] and [Job.Results].
//   - [Decide] and [Appended]: the caller's decision, and the appended event with its sequence
//     and the job folded with it.
//   - TypeQueued and the other Type constants: the job's 13 event types; ExtCausation and the
//     other Ext constants: its extension attributes; the Layer constants and [Source].
//   - [TypeWork] and [Work]: the event that puts a job on the work queue, and its data.
//   - [ErrNotFound], [ErrInvalidID], [ErrUnknownType], and [ErrEnded].
package jobs
