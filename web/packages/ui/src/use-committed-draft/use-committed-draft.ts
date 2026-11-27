import { type Dispatch, type SetStateAction, useCallback, useState } from "react";

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
//
// `validate` says why a draft cannot be handed over. `commit` then keeps it, with that reason as
// `problem` for the box to show, until the next edit.
const useCommittedDraft = <T>(committed: T, validate?: (draft: T) => string | undefined) => {
  const [draft, setDraft] = useState<T>(committed);
  const [problem, setProblem] = useState<string>();
  // Stable, as the state setter it wraps is: callers list it in effect dependencies.
  const editDraft: Dispatch<SetStateAction<T>> = useCallback((next) => {
    setProblem(undefined);
    setDraft(next);
  }, []);
  // Drop the edit and show the stored value again.
  const discard = () => editDraft(committed);
  // Hand the draft over, unless `validate` refuses it.
  const commit = (handOver: (draft: T) => void) => {
    const refused = validate?.(draft);
    setProblem(refused);
    if (refused === undefined) handOver(draft);
  };
  return { commit, discard, draft, editDraft, problem };
};

export { useCommittedDraft };
