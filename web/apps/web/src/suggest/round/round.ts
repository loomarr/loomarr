// Reads the tool-loop round off a `suggestion` SSE frame.
//
// ⚠ It arrives as a NUMBER now. It used to ride the wire as a string ("0" outside the loop)
// because the backend payload was a flat map of strings, and the frontend type carried a
// warning that declaring it a number "would typecheck and then compare wrong at runtime".
// The frame is a typed DTO now (internal/api/events.go SuggestionEvent), so the parse is gone.
//
// The rest of the rule stays, because it was never about parsing: 0 means "outside the tool
// loop", and the UI wants "no round to show" rather than a literal "round 0". Non-finite is
// folded into the same answer so a malformed frame cannot leak NaN into the display.
//
// Shared by the two hooks that follow a generation (suggest and refine): they track the same
// frames, and a second copy of this rule is a second place for it to drift.
const roundOf = (raw: number | undefined): number | undefined =>
  raw !== undefined && Number.isFinite(raw) && raw > 0 ? raw : undefined;

export { roundOf };
