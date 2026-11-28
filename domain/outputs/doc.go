// Package outputs is the GetOutput Query: the content a job's output event names by its SHA-256,
// streamed from docproc's content store, served as GET /api/outputs/{sha256}.
//
// An event carries an output's SHA-256, never its content; docproc keeps the content in its
// content store under that SHA-256, and GetOutput reads it from there. The store keeps no media
// type, so the content is answered as application/octet-stream, the output event's media_type
// naming what it is; its SHA-256, a strong validator, is its ETag.
//
// The package's API:
//
//   - [Service], built by [New]: [Service.Open].
//   - [Routes]: the route group the composition root mounts, each refusal a problem typed by
//     one of client's Problem constants.
//   - [ErrInvalid] and [ErrNotFound].
package outputs
