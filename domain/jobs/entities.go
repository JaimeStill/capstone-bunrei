package jobs

import (
	"crypto/sha256"
	"encoding"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"slices"
	"strconv"

	"github.com/standards-lab/docinfer/job"
	"github.com/standards-lab/go-core/event"

	"github.com/JaimeStill/capstone-bunrei/client"
)

// Job is a job as the fold of its events reads it: the [client.Job] GetJob returns, and the state
// the fold keeps that client.Job has no field for.
type Job struct {
	client.Job
	// CancelRequested reports whether a cancel_requested event has been folded.
	CancelRequested bool
	// Checkpoint is the After of the last checkpoint event, 0 before the first.
	Checkpoint uint64
	// Batches counts the batch events, every attempt's.
	Batches int
	// Chunks sums the batch events' batchchunks.
	Chunks int
	// Attempts holds one entry per started event, in order, each without its Done.
	Attempts []client.Attempt

	digest []byte
}

// Ended reports whether the job has ended: completed, failed, or cancelled.
func (j Job) Ended() bool {
	switch j.State {
	case client.StateCompleted, client.StateFailed, client.StateCancelled:
		return true
	}
	return false
}

// Results returns the manifest of the batches folded so far, over every attempt, as the events
// note's manifest rule gives it; the caller adds each attempt's Done.
func (j Job) Results() client.Manifest {
	h := resume(j.digest)
	return client.Manifest{
		Batches:  j.Batches,
		Chunks:   j.Chunks,
		Digest:   hex.EncodeToString(h.Sum(nil)),
		Attempts: slices.Clone(j.Attempts),
	}
}

// Decide decides the next event of a job from the job as folded, or refuses with an error. The
// event needs only its type, its data, and the extensions the caller sets: causationid, and on a
// batch batchdigest and batchchunks. It may carry its own id; the [Service] stamps the rest.
type Decide func(Job) (event.Event, error)

// Appended is an appended event as the [Service] stamped it, its stream sequence, and the job
// with it folded.
type Appended struct {
	Event    event.Event
	Sequence uint64
	Job      Job
}

func (j Job) apply(e event.Event) (Job, error) {
	j.Attempts = slices.Clip(j.Attempts)
	j.Sequence++
	switch e.Type {
	case TypeQueued:
		var d client.Queued
		if err := decode(e, &d); err != nil {
			return j, err
		}
		j.Profile, j.Name, j.Size, j.SHA256 = d.Profile, d.Name, d.Size, d.SHA256
		j.State = client.StateQueued
	case TypeStarted:
		var d client.Started
		if err := decode(e, &d); err != nil {
			return j, err
		}
		j.State, j.Attempt = client.StateRunning, d.Attempt
		j.Attempts = append(j.Attempts, client.Attempt{})
	case TypeAttemptFailed:
		var d client.AttemptFailed
		if err := decode(e, &d); err != nil {
			return j, err
		}
		j.State, j.Error = client.StateQueued, d.Error
	case TypeCancelRequested:
		j.CancelRequested = true
	case TypeCompleted:
		var d client.Manifest
		if err := decode(e, &d); err != nil {
			return j, err
		}
		j.State, j.Manifest = client.StateCompleted, &d
	case TypeFailed:
		var d client.Failed
		if err := decode(e, &d); err != nil {
			return j, err
		}
		j.State, j.Error = client.StateFailed, d.Error
	case TypeCancelled:
		j.State = client.StateCancelled
	case TypeCheckpoint:
		var d job.Checkpoint
		if err := decode(e, &d); err != nil {
			return j, err
		}
		j.Checkpoint = d.After
	case TypeBatch:
		return j.batch(e)
	case TypePart, TypeOutput, TypeWarning, TypeInference:
	default:
		return j, fmt.Errorf("%w: %q", ErrUnknownType, e.Type)
	}
	return j, nil
}

func (j Job) batch(e event.Event) (Job, error) {
	digest, ok := e.Extensions[ExtBatchDigest]
	if !ok {
		return j, fmt.Errorf("%s at jobsequence %d: no %s", e.Type, j.Sequence, ExtBatchDigest)
	}
	chunks, err := strconv.Atoi(e.Extensions[ExtBatchChunks])
	if err != nil || chunks < 0 {
		return j, fmt.Errorf("%s at jobsequence %d: %s %q is not a count", e.Type, j.Sequence, ExtBatchChunks, e.Extensions[ExtBatchChunks])
	}
	h := resume(j.digest)
	h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(digest))))
	h.Write([]byte(digest))
	state, err := h.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return j, fmt.Errorf("digest state: %w", err)
	}
	j.Batches++
	j.Chunks += chunks
	j.digest = state
	return j, nil
}

func resume(state []byte) hash.Hash {
	h := sha256.New()
	if state != nil {
		if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary(state); err != nil {
			panic(fmt.Sprintf("jobs: digest state: %v", err))
		}
	}
	return h
}

func decode(e event.Event, v any) error {
	if err := json.Unmarshal(e.Data, v); err != nil {
		return fmt.Errorf("%s: data: %w", e.Type, err)
	}
	return nil
}
