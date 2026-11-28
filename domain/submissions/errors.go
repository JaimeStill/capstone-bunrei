package submissions

import "errors"

var (
	// ErrInvalid reports a Submit missing its ID or profile, with an ID past 255 bytes, or with
	// a media type that is not type/subtype: a 400.
	ErrInvalid = errors.New("invalid submit")

	// ErrContentTooLarge reports a document that decodes past the small-document limit: a 413.
	ErrContentTooLarge = errors.New("the document exceeds the small-document limit; send it through the upload")

	// ErrExtractionTooLarge reports extracts that marshal past [ExtractionLimit] together: a 413.
	ErrExtractionTooLarge = errors.New("the extracts exceed the extraction limit")

	// ErrConflict reports a Submit repeating an ID with a different request: a 409.
	ErrConflict = errors.New("the ID was submitted with a different request")
)
