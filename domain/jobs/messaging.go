package jobs

import (
	"github.com/standards-lab/docinfer"
	"github.com/standards-lab/docproc/dispatch"

	"github.com/JaimeStill/capstone-bunrei/client"
)

// The job's 13 event types, each bunrei.job. followed by its name. The data each carries, and the
// layer that produces it, are the events note's: client's [client.Queued], [client.Started],
// [client.Manifest], [client.AttemptFailed], and [client.Failed]; docproc's dispatch.Event for a
// part, an output, and a warning; docinfer's Inference and Batch; and docinfer job's Checkpoint.
// TypeCancelRequested and TypeCancelled carry no data.
const (
	TypeQueued          = "bunrei.job.queued"
	TypeStarted         = "bunrei.job.started"
	TypePart            = "bunrei.job.part"
	TypeOutput          = "bunrei.job.output"
	TypeWarning         = "bunrei.job.warning"
	TypeInference       = "bunrei.job.inference"
	TypeBatch           = "bunrei.job.batch"
	TypeCheckpoint      = "bunrei.job.checkpoint"
	TypeCancelRequested = "bunrei.job.cancel_requested"
	TypeCompleted       = "bunrei.job.completed"
	TypeAttemptFailed   = "bunrei.job.attempt.failed"
	TypeFailed          = "bunrei.job.failed"
	TypeCancelled       = "bunrei.job.cancelled"
)

// The extension attributes of a job's events. The caller sets ExtCausation, and on a batch
// ExtBatchDigest and ExtBatchChunks; the [Service] stamps the rest.
const (
	ExtCorrelation = "correlationid"
	ExtCausation   = "causationid"
	ExtLayer       = "layer"
	ExtSequence    = "jobsequence"
	ExtBatchDigest = "batchdigest"
	ExtBatchChunks = "batchchunks"
)

// The layers that produce a job's events, the values of [ExtLayer].
const (
	LayerBunrei   = "bunrei"
	LayerDocproc  = "docproc"
	LayerDocinfer = "docinfer"
)

// Source is the source of every event Bunrei appends: the service, never a replica.
const Source = "bunrei"

var layers = map[string]string{
	TypeQueued:          LayerBunrei,
	TypeStarted:         LayerBunrei,
	TypePart:            LayerDocproc,
	TypeOutput:          LayerDocproc,
	TypeWarning:         LayerDocproc,
	TypeInference:       LayerDocinfer,
	TypeBatch:           LayerDocinfer,
	TypeCheckpoint:      LayerDocinfer,
	TypeCancelRequested: LayerBunrei,
	TypeCompleted:       LayerBunrei,
	TypeAttemptFailed:   LayerBunrei,
	TypeFailed:          LayerBunrei,
	TypeCancelled:       LayerBunrei,
}

var schemas = map[string]string{
	LayerBunrei:   "urn:standards-lab:bunrei:client:" + client.Version,
	LayerDocproc:  "urn:standards-lab:docproc:dispatch:" + dispatch.Version,
	LayerDocinfer: "urn:standards-lab:docinfer:" + docinfer.Version,
}

func (s *Service) subject(job string) string { return s.prefix + "." + job }
