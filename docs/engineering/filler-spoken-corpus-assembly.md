# Spoken-safety corpus assembly

`cmd/filler-spoken-corpus-assemble` will turn separately prepared private
candidate cohorts into the one complete draft that independent reviewers and
the authority locker share. It is the only supported place to join VCTK
target-locale controls, consented real-speaker positives, and the remaining
clean slices. Reviewing or hand-splicing a cohort before this step binds the
wrong draft and is not valid evidence.

The exported interface is one deep operation:
`fillersafetycorpus.AssembleReviewDraft(ctx, config)`. Its config names only a
private `0600` assembly plan, one private input root, and a new output
directory. Cohort decoding, byte verification, collision detection, source
snapshotting, #929 draft construction, review-worklist construction, resource
limits, permissions, and atomic publication stay inside the module.

The private plan binds:

- the exact policy path and digest;
- certification challenge, proposer, implementation, and audio/video route
  identities required by the #929 draft;
- a sorted list of cohort document paths, source roots, document digests,
  candidate kinds, datasets, and exact case counts;
- the exact combined case count; and
- aggregate verified-input, published-output, and wall-time ceilings.

Every prepared case supplies one opaque case and source-family identity, a
complete audiovisual source authority, an optional transcript path and digest,
rights and truth-provenance paths and digests, locale, sorted slices, a proposed
`clean` or `positive` claim, and sorted positive intervals when applicable.
The assembler reopens and hashes all referenced bytes. A reused path with a
different authority, a duplicate case/family/source across cohorts, a symlink,
policy or implementation drift, an unknown slice, or a malformed interval
fails before publication.

A certification assembly must reach the #929 corpus floor before review: at
least 59 independent positive families, at least 100 independent clean
families, and complete coverage of every required positive and clean slice.
Meeting that floor does not establish truth; it only makes the proposed draft
complete enough to review.

The output is one atomic private `0700` directory containing `0600` files:

- `draft.json`, the canonical path-bearing #929 authority draft;
- `policy.json`, the exact policy bytes reviewers use;
- `primary-review-one.json` and `primary-review-two.json`, byte-identical
  worklists that bind the draft and policy digests and contain every case in
  canonical order; and
- `cases/<opaque-case>/source.mp4`, optional `transcript.txt`,
  `provenance.json`, and `rights.json` snapshots.

The worklists expose the proposed claim and intervals because reviewers must
verify known-script evidence, but they include no evaluation output, other
review, reviewer identity, or completed decision. Reviewers independently emit
the #929 review schema. The authority locker still requires two complete
agreements or exact disagreement-only adjudication.

```bash
go run ./cmd/filler-spoken-corpus-assemble \
  --plan /private/spoken/assembly-plan.json \
  --input-root /private/spoken \
  --output /private/spoken/review-draft
```

Assembly performs no download, model call, review, certification run, training,
ingestion, scheduling, or spend.
