# Spoken-safety cascade prototype

This is a throwaway prototype for one question: can a pinned native-audio LLM
correctly adjudicate the local keyword detector's proposed intervals without a
person reviewing every clip? The pilot sends all eight labeled clean controls
and one deterministic positive from each of the nine positive challenge slices.
It reveals labels only after each response so scoring cannot influence the
request. It does not grant production, ingestion, or scheduling authority.

Run it from this throwaway branch with:

```bash
make prototype-spoken-safety-cascade
```

The command reads the private policy and development controls outside the
repository, routes serially through the snapshot-proven Google Vertex ZDR
endpoint with fallback disabled, prints the complete aggregate state after
each case, and creates a mode-0600 private report. Pass
`PROTOTYPE_ARGS='--mode all --out /new/private/path.json'` only after the pilot
has established that the transport and decision shape work.

The same sample can be run against a different snapshot-proven native-audio
model by supplying its concrete model, canonical model, provider identity,
provider slug, snapshot, and reasoning mode. This exists only to compare an
independent model family; it is not a general-purpose model runner.

## Result

The prototype answered the narrow question: a native-audio LLM can materially
reduce the keyword detector's review queue, but no single model tested can
authorize a clean or suitable classification.

- Gemini 3.8 Flash with minimal reasoning detected 3 of 9 labeled positive
  controls and held 1 of 8 labeled clean controls. Medium reasoning improved
  positive detection to 4 of 9 but retained the same clean false hold. Its
  negative decisions are therefore not safe overrides for keyword candidates.
- Voxtral Small 24B detected all 9 labeled positive controls and held 6 of 8
  labeled clean controls. This is too sensitive as a standalone classifier,
  but the desired failure direction for a candidate adjudicator.
- On 88 real keyword candidates, Voxtral retained the known prohibited source,
  retained 49 of 87 unlabelled candidates, and rejected 38. All 88 requests
  completed without an unclear response or transport failure. The submitted
  audio windows totalled 410.28 seconds, with a median of 2.88 seconds.
- Twelve candidates also had prior direct-video evidence. Voxtral and Gemini
  agreed on four no-signal candidates, disagreed on six, agreed on the known
  prohibited source, and shared one case whose video assessment failed. This
  is corroboration evidence only; none of the four becomes certified clean.
- Gemini 3.8 then screened the complete source video and audio for all 38
  Voxtral-negative candidates. All 38 returned complete modality coverage with
  no prohibited flag or operational failure, so all 38 survive as two-model
  candidate rejections. They still are not clean or admission-authorized.
- The exact Gemini 3.8 video lane was challenged with the three prohibited-
  signal anchors from the prior 48-case video result. It recovered both spoken
  anchors with valid audio intervals. It also returned eleven explicit-nudity
  flags for the visual anchor, but one interval was inverted, so the stricter
  contract correctly converted that response to an operational hold. A
  presence signal can quarantine; malformed timing cannot be projected.

The retained design is a conservative cascade: a local detector proposes
small intervals, an independent native-audio model adjudicates them, and a
second independent lane reviews only negative decisions. Any positive,
disagreement, insufficient coverage, or operational failure remains held. The
real-corpus run now has 50 held candidates and 38 two-model candidate
rejections. A negative decision means only `candidate_rejected`; it does not
mean clean, suitable, ingestible, or schedulable.

Private reproducibility artifacts:

- Voxtral control report SHA-256:
  `23e882eea55b76e7f2d33aeb9065367231aa6b6ba3642f01bb1f63258e9d9852`
- Voxtral real-candidate report SHA-256:
  `935eb06c1f068bcb6b9c94474751cd11aa498361d6834946c36e63c4933ecf1a`
- Gemini complete-video corroboration report SHA-256:
  `2b26fb046d9875dee9cf9773ad6a4ba3bfb685579879a994d8b3985b46e95e3b`
- Gemini positive-anchor challenge report SHA-256:
  `1d34998b32f50dd8e4a8f310541e3c71505d74925271bec264129f25d68b43dc`

The complete-video probe, 38-case run, and two three-anchor attempts charged
$0.188965125 in total. Including the native-audio work, this prototype phase
charged $0.266244985.

The prototype does not settle the production local-detector dependency. The
sherpa-onnx model was useful for this measurement, but its distributable
license evidence must be resolved or the proposer must be replaced before a
production implementation is selected.
