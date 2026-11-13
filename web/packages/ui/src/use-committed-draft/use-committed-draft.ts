import { useState } from "react";

// useCommittedDraft — the draft of one input that is not a form: a single value the viewer edits
// beside the value that is stored, which only changes when the draft is committed (on blur or
// Enter) or discarded. A box that saves on each keystroke, a path-map row, a From/To pair that
// waits for its partner: each keeps what is being typed here, and hands it over itself.
//
// It is the one sanctioned draft buffer for these (scripts/check-forms.mjs names it). Inputs that
// are submitted together, with validation, are a form and use `useUiForm`.
//
// The draft starts as `committed` and is not reseeded when `committed` changes: a caller that
// wants the stored value back remounts the box with a `key`, or calls `editDraft` itself.
const useCommittedDraft = <T>(committed: T) => {
  const [draft, editDraft] = useState<T>(committed);
  // Drop the edit and show the stored value again.
  const discard = () => editDraft(committed);
  return { discard, draft, editDraft };
};

export { useCommittedDraft };
