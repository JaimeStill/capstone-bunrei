# goal · capstone.data-intelligence.bunrei

- **State:** building
- **Task:** text-layer
- **Branch:** text-layer

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

## Task brief · text-layer

**Problem**

docinfer sends every page image to vision, even when the page's text layer already holds its
text. On docs-corpus v0.2.0, 1,234 of the 1,329 page images with a layer have at least 100 layer
words. They take about 84% of vision's request time, and for them vision mostly repeats the
layer. Vision helps on pages whose layer is thin or missing. So a skip rule has to tell those
pages from a page whose layer is whole, and decide from the layer alone before any request is
sent. 100 words is a starting value, and the live run has to record enough to tune it.

**Behaviors**
1. `Vision` takes a layer-word threshold. 0 never skips, `New` refuses a negative value
   (`ErrRequest`), and docinfer ships no default.
2. A word is a run of Unicode letters or digits. A page's layer words are the words in its text
   output.
3. A page image is matched to its page's text output when both come from the document stage
   with the same input (Origin), part ID, stage, and `MetaPage`, and the text comes before the
   image in pull order.
4. When the threshold is above 0 and the matched layer holds at least that many words, `Run`
   sends no vision request for the image. It yields no `Inference` and holds nothing in the
   cache. The image is echoed and settled with no work, and counted in a new
   `VisionCounts.Skipped`.
5. These page images go to vision exactly as before:
   - an image with no matched text output
   - an image whose layer has fewer words than the threshold
   - an image from another stage: the picture stage's TIFF pages, OneNote pictures, and video
     frames
6. A page's text output is still chunked as an output source, whether or not its image is
   skipped.
7. `Profile` reports the threshold beside vision's `Identity`. The threshold isn't part of the
   `Identity`, so changing it never invalidates a held description.
8. When inputs arrive in another order, such as an image before its page's text, that image
   is never skipped and the run never fails.
9. Skipping changes nothing for transcription, extraction, metadata texts, or embeddings.
   Extraction runs on the page's text output as before.
10. cmd/docinfer's configuration takes the threshold, with a starting value of 100. Its Build
    refuses a negative value before reading the docproc run, and its report shows `Skipped`.
11. A cached rerun of cmd/docinfer over the docs-corpus v0.2.0 baseline, through gb10-2's
    gateway, is recorded in docinfer's acceptance notes. The record holds:
    - the run's counts, including `Skipped`, compared with the 1,234 measured at PLAN
    - the per-record metadata counts
    - from the baseline's held vision texts, the thin pages the threshold skipped and the
      vision words lost, using the finding's thin rule (vision has at least twice the layer's
      words and at least 60 more)
    - the skipped and thin-skipped counts at thresholds 50, 100, 150, and 200, so the threshold
      can be tuned

**Test seams**
- `Pipeline.Run` against go-ai's `aitest` fake endpoint, counting requests and asserting on
  `Result`s, `Counts`, and `Profile`.
- cmd/docinfer's configuration Build, and its built-binary tier over `processtest`.

**Slices**
1. The threshold on `Vision`, `New`'s refusal, `Profile`'s report, and the word count
   (behaviors 1, 2, 7).
2. The match and the skip in `Run`, and `VisionCounts.Skipped` (behaviors 3–6, 8, 9). Tests
   cover:
   - a layer at, below, and above the threshold
   - a page with no layer
   - a TIFF page from the picture stage
   - a page's text and image from different parts
   - an image before its text
   - a threshold of 0
   - an extraction alongside a skip
   - a redelivery with `Final`
3. docinfer v0.5.0's changelog. postgres, cmd/docinfer, and acceptance require v0.5.0.
   cmd/docinfer takes the threshold at 100, and its report and changelog show `Skipped`
   (behavior 10).
4. The cached rerun over docs-corpus v0.2.0, recorded in the acceptance notes (behavior 11).
   It reuses the baseline's cache so the held vision texts stay available.

**Out of scope**

This task does not cover:
- the vision prompt, sampling, and pages described instead of transcribed (task `vision`)
- OneNote pictures, which are images and use vision
- a bitmap-coverage measure
- any change to docproc
- tuning the threshold past its starting value
- Bunrei's profile, which takes the threshold at `jobs`

**Door**

One-way: once pushed, the docinfer v0.5.0, postgres, and cmd/docinfer tags reach consumers. The
behavior stays two-way until SHIP, and a threshold of 0 turns it off.

**Release**

- docinfer: `v0.5.0`, `postgres/v0.1.3`, `cmd/docinfer/v0.4.0`

## Progress

slices 0/4 committed · standards — · spec — · editor —

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
  layer-word threshold, starting at 100, to be tuned from the docs-corpus rerun.
