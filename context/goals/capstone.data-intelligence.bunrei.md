# goal · capstone.data-intelligence.bunrei

- **State:** idle
- **Task:** none
- **Branch:** none

## Tasks

1. [x] metadata
2. [x] metadata-text
3. [x] text-layer
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
- metadata: docinfer renders the metadata text; rejected a docproc text rendering (chunked as
  content), Bunrei (`cmd/docinfer` would lack it), and widening `MetadataText` to take the record
  (every host would handle an error a part can't have); never defer a layer's change to avoid its
  release.
- metadata: capstone-bunrei requires docproc and docinfer only from the first task that imports them.
- metadata-text: Skype conversation records are not rendered (backlog).
- text-layer: a page's text layer stands in for its vision text when it holds at least the
  threshold's words, a word being a run of letters or digits. A page with no layer always goes
  to vision. 100 is a starting value, to be tuned from the live run's measurements of thin
  pages skipped and vision words lost. Rejected OCRmyPDF's any-text rule, which skips every thin
  page, and bitmap coverage, which needs a Python layout pass or a new docproc adapter.
- text-layer: docinfer's `Pipeline.Run` decides the skip by matching a page image to the text
  output of its page that comes just before it. docproc keeps emitting every page image.
  Rejected dropping the image in docproc's document stage, which would put inference policy
  into preparation.
- text-layer: a skipped page image yields no `Inference` and counts in `VisionCounts.Skipped`.
  `Profile` reports the threshold. `Outcome` gains no constant.
- text-layer: Bunrei's profile carries the threshold, set at `jobs`.
- text-layer: a OneNote picture is an image and uses vision. Its OCR text reaches chunks only
  through its page's Markdown, and there is no backlog entry for it.
- text-layer: a later acceptance rerun over docs-corpus v0.2.0 reads docproc v0.10.0's run,
  `~/runs/docs-corpus/baseline/v0.2.0/docproc-v0.10.0`, and seeds its cache from
  `docinfer/text-layer/out/`; the v0.9.0 run in `docproc/` lacks the metadata records.

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
- coordinator · `docs/data-intelligence.md`, "Open items": `text-layer` resolved. A OneNote
  picture is an image and uses vision.
- coordinator · `docs/data-intelligence.md`, "Text layer and vision": resolved by docinfer
  v0.5.0's layer-word skip, starting at 100 words. Correct the claim that two-column layers
  interleave their columns: the finding shows pdftotext reads column by column, and the misses
  are words split across columns. Cite docs-corpus v0.2.0 rather than the retired ~/docs pool.
- coordinator · `docs/data-intelligence.md`, "docinfer's configuration": add the vision
  layer-word threshold, cmd/docinfer's `vision.layer_words`, starting at 100, to be tuned from
  the thresholds docinfer's `docs/acceptance.md` measures ("The layer-word threshold").
- docs-corpus · `docs/baseline.md`, "v0.2.0": the run directory also holds docproc v0.10.0's run,
  `docproc-v0.10.0/`, with its binaries in `bin-v0.10.0/`, and docinfer's text-layer run in
  `docinfer/text-layer/`; link docinfer's
  `docs/acceptance.md#docs-corpus-v020-with-the-text-layer-skip`.
