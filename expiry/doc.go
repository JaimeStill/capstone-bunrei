// Package expiry reclaims the objects under one prefix of the object store
// once they have gone unmodified for an age: the bounded passes the
// service's sweeper drives over the content and input stores, whose puts
// rewrite an object in use once it is older than their renew age, so an
// object past the expiry age is one no job has put since.
//
// [New] builds a [Reclaimer] over a go-storage Client, the prefix it
// sweeps, and the age, with [Options] carrying the clock. Its
// [Reclaimer.Pass] satisfies go-database's sweep.Reclaimer structurally:
// each pass lists at most budget objects under the prefix, continuing
// from where the previous pass stopped, deletes those whose ModifiedAt is
// more than the age before the clock's now, and reports more while the
// listing has objects left, so a cycle walks the prefix once and the next
// cycle starts it again. The package imports no provider and nothing of
// the service's composition root.
package expiry
