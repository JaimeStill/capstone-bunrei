# goal · capstone.data-intelligence.bunrei

- **State:** idle
- **Task:** none
- **Branch:** none

## Tasks

1. [x] metadata
2. [ ] metadata-text
3. [ ] text-layer
4. [ ] vision
5. [ ] messaging-append
6. [ ] messaging-work-queue
7. [ ] sse-codec
8. [ ] sse-writer
9. [ ] go-ai-sse
10. [ ] upload
11. [ ] state
12. [ ] api
13. [ ] jobs
14. [ ] event-stream
15. [ ] tool-packaging

## Decisions

- capstone: contracts (`chunks`, `protocol`, `events`) are designed in `.foundation` at capstone-data-intelligence; Bunrei and Mikoshi build against them, and a task that reads one opens its summary with "Waits on".
- capstone: Bunrei's `client` module is released by tag (`client/v0.1.0` at `api`); Mikoshi imports the published version, never a replace directive.
- capstone: only `.bunrei` touches the exclusive group (docproc, docinfer, go-messaging); `.mikoshi` stays out of it, so its SSE relay and context tree live in Mikoshi, shaped to graduate.
- capstone: items join contexts by reference and a context covers its descendants; chunks and embeddings stay keyed by content and carry no context.
- metadata: PDF metadata through pdfinfo; rejected qpdf (its JSON reaches the dictionary and XMP only by object id, and leaves PDF `D:` date parsing to docproc).
- metadata: EXIF through magick; rejected exiftool (a new Perl tool in Bunrei's image) and Go EXIF
  libraries (none ecosystem-standard).
- metadata: the embedded metadata text widens in docinfer, `MetadataText`'s one builder, in task
  `metadata-text`; rejected a docproc text rendering (chunked as content) and Bunrei (`cmd/docinfer`
  would lack it); never defer a layer's change to avoid its release.
- metadata: capstone-bunrei requires docproc and docinfer only from the first task that imports them.

## Pending edits

- coordinator · `docs/data-intelligence.md`, "Tool packaging": the image holds poppler-utils 22.02
  or later, whose pdfinfo docproc v0.10.0's document stage requires beside pdftotext and pdfdetach,
  and ImageMagick 7.1 or later, whose magick its vision stage now requires, as the picture and
  office stages do. "Open items": `metadata` resolved by docproc v0.10.0.
- coordinator · roadmap: a backlog entry for picture metadata beyond EXIF (IPTC, maker notes,
  reverse geocoding).
