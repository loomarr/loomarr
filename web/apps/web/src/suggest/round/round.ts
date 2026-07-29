// Reads the tool-loop round off a `suggestion` SSE frame.
//
// It rides the wire as a STRING ("0" outside the loop) because the BE payload is a flat
// map of strings. 0 and anything unparseable both become undefined, i.e. "no round to
// show" rather than a literal "round 0" or a NaN leaking into the UI.
//
// Shared by the two hooks that follow a generation (suggest and refine): they track the
// same frames, and a second copy of this rule is a second place for it to drift.
const roundOf = (raw: string | undefined): number | undefined => {
  const n = Number(raw);
  return Number.isFinite(n) && n > 0 ? n : undefined;
};

export { roundOf };
