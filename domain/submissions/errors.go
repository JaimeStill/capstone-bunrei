package submissions

import "errors"

var (
	// ErrInvalid reports a Submit missing its ID or profile, with an ID past 255 bytes, or with
	// a media type that is not type/subtype, or a CompleteUpload missing its profile or whose
	// upload is not a UUID: a 400.
	ErrInvalid = errors.New("invalid submit")

	// ErrContentTooLarge reports a document that decodes past the small-document limit: a 413.
	ErrContentTooLarge = errors.New("the document exceeds the small-document limit; send it through the upload")

	// ErrExtractionTooLarge reports extracts that marshal past [ExtractionLimit] together: a 413.
	ErrExtractionTooLarge = errors.New("the extracts exceed the extraction limit")

	// ErrConflict reports a Submit repeating an ID, or a CompleteUpload repeating an upload,
	// with a different request: a 409.
	ErrConflict = errors.New("the ID was submitted with a different request")

	// ErrUploadNotFound reports a CompleteUpload of an upload with no completed record: never
	// completed, or completed longer than the upload lifetime ago. A 404.
	ErrUploadNotFound = errors.New("no completed upload is recorded under the ID")
)
