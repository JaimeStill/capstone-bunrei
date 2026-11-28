package client

// ProblemPrefix begins the type of every problem Bunrei answers a refusal with; the problem's
// name follows it.
const ProblemPrefix = "urn:standards-lab:bunrei:problem:"

// The types of the problems Bunrei refuses a request with, each [ProblemPrefix] followed by the
// problem's name.
const (
	// ProblemInvalidRequest is a 400: a body that is not the Command's JSON, or a Command missing
	// its ID or profile, or with a media type that is not type/subtype.
	ProblemInvalidRequest = ProblemPrefix + "invalid-request"
	// ProblemContentTooLarge is a 413: a document past the small-document limit, which goes
	// through the upload instead, or a body past the limit's encoded size.
	ProblemContentTooLarge = ProblemPrefix + "content-too-large"
	// ProblemExtractionTooLarge is a 413: extracts that marshal past 256 KiB together.
	ProblemExtractionTooLarge = ProblemPrefix + "extraction-too-large"
	// ProblemSubmitConflict is a 409: a Submit repeating an ID with a different request.
	ProblemSubmitConflict = ProblemPrefix + "submit-conflict"
)
