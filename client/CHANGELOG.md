# Changelog

All notable changes to Bunrei's client module (`github.com/JaimeStill/capstone-bunrei/client`)
are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the module adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). This changelog covers this
sub-module only; the service at the repository root is not released as a module.

## [Unreleased]

## [v0.1.0] - 2026-11-28

### Added

- `Client`, built by `New` over the base URL the API's routes hang from, such as
  `http://bunrei:8080/api`, and the caller's `http.Client`, with one method per Command and
  Query: `Submit` (`POST /jobs`), `CompleteUpload` (`POST /uploads/{upload}/complete`),
  `CancelJob` (`POST /jobs/{id}/cancel`), `GetJob` (`GET /jobs/{id}`), `GetOutput`
  (`GET /outputs/{sha256}`, streamed), and `ListProfiles` (`GET /profiles`).
- `Client.Upload` sends a large document as a resumable upload under `/uploads`, resuming a send
  cut off partway from the offset the server holds within its `UploadOptions`, and returns the ID
  `CompleteUpload` names.
- The types: `Job`, `State` and its five states, `Manifest`, `Attempt`, `Profile`, the Commands
  `Submit` and `CompleteUpload`, the completed `Upload`, and the job's event data `Queued`,
  `Started`, `AttemptFailed`, and `Failed`, each at the wire version `Version`, `1`, which
  Bunrei's events name as `urn:standards-lab:bunrei:client:1`.
- `Problem`, the RFC 9457 refusal every method returns, its type constants under `ProblemPrefix`,
  and one sentinel per type for `errors.Is`: `ErrInvalidRequest`, `ErrContentTooLarge`,
  `ErrExtractionTooLarge`, `ErrSubmitConflict`, `ErrJobNotFound`, `ErrJobEnded`,
  `ErrOutputNotFound`, and `ErrUploadNotFound`.
- Requires `github.com/standards-lab/docinfer` v0.6.0, `github.com/standards-lab/docproc`
  v0.10.0, and `github.com/standards-lab/go-web-sdk` v0.20.0.

[Unreleased]: https://github.com/JaimeStill/capstone-bunrei/compare/client/v0.1.0...HEAD
[v0.1.0]: https://github.com/JaimeStill/capstone-bunrei/releases/tag/client/v0.1.0
