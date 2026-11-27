# goal · capstone.data-intelligence.bunrei

- **State:** idle
- **Task:** none
- **Branch:** none

## Tasks

1. [x] metadata
2. [x] metadata-text
3. [ ] text-layer
4. [ ] vision
5. [ ] messaging-append
6. [ ] messaging-work-queue
7. [ ] state
8. [ ] api
9. [ ] jobs
10. [ ] event-stream
11. [ ] tool-packaging

## Decisions

- capstone: contracts (`chunks`, `protocol`, `events`) are designed in `.foundation` at capstone-data-intelligence; Bunrei and Mikoshi build against them, and a task that reads one opens its summary with "Waits on".
- capstone: Bunrei's `client` module is released by tag (`client/v0.1.0` at `api`); Mikoshi imports the published version, never a replace directive.
- capstone: items join contexts by reference and a context covers its descendants; chunks and embeddings stay keyed by content and carry no context.
- metadata: PDF metadata through pdfinfo; rejected qpdf (its JSON reaches the dictionary and XMP only by object id, and leaves PDF `D:` date parsing to docproc).
- metadata: EXIF through magick; rejected exiftool (a new Perl tool in Bunrei's image) and Go EXIF
  libraries (none ecosystem-standard).
- metadata: docinfer widens the embedded metadata text (task `metadata-text`): `MetadataText(p
  dispatch.Part)` builds the file lines, and an unexported record renderer beside it, which only
  `Pipeline.Run` calls, renders records; rejected a docproc text rendering (chunked as content),
  Bunrei (`cmd/docinfer` would lack it), and widening `MetadataText` to take the record (every host
  would handle an error a part can't have); never defer a layer's change to avoid its release.
- metadata: capstone-bunrei requires docproc and docinfer only from the first task that imports them.
- metadata-text: `Pipeline.Run` reads each metadata record from content and renders it; rejected
  having each host render records into `Input.Metadata`.
- metadata-text: Skype conversation records are not rendered (backlog).
- metadata-text: validated by unit tests and a scratch run over docproc's real stages; no live
  acceptance run, so the next one records the per-record metadata counts.

## Pending edits

- coordinator · `docs/data-intelligence.md`, "Tool packaging": the image holds poppler-utils 22.02
  or later, whose pdfinfo docproc v0.10.0's document stage requires beside pdftotext and pdfdetach,
  and ImageMagick 7.1 or later, whose magick its vision stage now requires, as the picture and
  office stages do. "Open items": `metadata` resolved by docproc v0.10.0.
- coordinator · roadmap: a backlog entry for picture metadata beyond EXIF (IPTC, maker notes,
  reverse geocoding).
- coordinator · roadmap: a backlog entry for rendering Skype conversation records
  (`MediaMailConversation`: topic, start, end, missed) as metadata text.
- capstone-data-intelligence · `context/chunks.md`, "Fields" and "Source occurrence": a source's
  text SHA-256 is its `sha256` when present, else its `output`'s; a metadata source may carry the
  record output it renders, whose `sha256` is the record's, not the text's; links move to docinfer
  v0.4.0 and docproc v0.10.0. The change reaches `.mikoshi`'s `ingest`, `contexts` and `lineage`
  tasks and `.bunrei`'s `api` task.
