// Package uploads is Bunrei's store of resumable uploads in progress:
// go-web-sdk's upload.Store over PostgreSQL and blob storage, so every
// replica serving one upload shares it. An upload's metadata, offset,
// length, hash state, and expiry are one row; each committed chunk is an
// object under the upload prefix at <upload>/<offset>/<append id>, whose
// key a chunk row records.
//
// go-storage has no conditional write, so the row is the authority and
// each append writes under a key of its own: an append writes its object,
// then commits by a conditional update of the upload's offset from the
// offset it expects, in one transaction with its chunk row. Of two appends
// racing at one offset, whichever replicas run them, exactly one commits;
// the other removes its own object, best effort, and returns the winner's
// offset. An object no chunk row names never reads back, and one a failed
// removal leaves is reclaimed by the upload prefix's expiry. An upload
// past its expiry, by the database's clock, is missing to every
// operation; a create reclaims an expired upload under its id, and the
// expired uploads' rows stay until they are reclaimed, by such a create or
// by the [Reclaimer] go-database's sweeper runs.
//
// The package's API:
//
//   - [Service], built by [New]: the upload.Store, each method returning
//     an error that matches fs.ErrNotExist for a missing upload.
//   - [Reclaimer], from [Service.Reclaimer]: the expired uploads'
//     go-database sweep.Reclaimer, which deletes their rows and leaves
//     their objects to the upload prefix's expiry.
package uploads
