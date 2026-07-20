<!-- Thanks for contributing! Keep PRs focused; see CONTRIBUTING.md. -->

## What & why

<!-- What does this change and why? Link any issue: Closes #123 -->

## Checklist

- [ ] `make check` passes (and any other gate this PR touches — see the table in CONTRIBUTING.md)
- [ ] Docs updated **first** if behavior deviates from `docs/design.md` (doc-first rule)
- [ ] No new dependency, or one added with a §14 rationale in the same PR
- [ ] Generated files regenerated, not hand-edited (`make openapi` / orval / migrations forward-only)
- [ ] Tests added/updated and they don't touch the network (mocked via `internal/testkit`)

## Notes for reviewers

<!-- Anything non-obvious: trade-offs, follow-ups, things you're unsure about. -->
