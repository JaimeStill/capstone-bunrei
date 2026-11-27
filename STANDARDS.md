# capstone-bunrei standards

The judgement calls the standards-reviewer applies to capstone-bunrei, beyond what `mise run check`
enforces. Each is one line; the architecture pages hold the reasoning.

- `docproc/context/bunrei.md`: the boundary the service implements over docproc and docinfer; a change to it lands in docproc first.
- `architecture/standards/go-elemental/principles/topology-and-naming.md`: `cmd/server`, `internal/app`'s layer files, and the domain packages.
- `architecture/principles/composition-root.md`: `internal/app` builds each domain service from the infrastructure nodes it uses.
- `architecture/principles/comments.md`: every comment.
