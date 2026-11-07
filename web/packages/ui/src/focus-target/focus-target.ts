import { useState } from "react";

import type { FocusableTargetHandle, FocusTargetRegistry } from "./focus-target.type";

// The props that make a control one of a surface's focus targets: its handle is registered, so a
// journey can send focus to it, and the registry hears where the focus engine lands before the
// control's own `onFocus` runs.
const focusTargetProps = <TTarget>(
  registry: FocusTargetRegistry<TTarget> | undefined,
  target: TTarget,
  onFocus?: () => void,
) => ({
  onFocus: () => {
    registry?.focused?.(target);
    onFocus?.();
  },
  ref: (handle: FocusableTargetHandle | null) => registry?.register(target, handle),
});

// The selection outlives focus (#1887): a control outside a list (a filter, a rail action) takes
// the remote while the list keeps the item OK returns to. Only the list's own focus lights that
// item, so the focused control is the one amber. A TV surface opens with focus in its list.
const useSelectionOutlivesFocus = () => {
  const [focusInList, setFocusInList] = useState(true);
  return {
    focusInList,
    focusEnteredList: () => setFocusInList(true),
    focusLeftList: () => setFocusInList(false),
  };
};

export { focusTargetProps, useSelectionOutlivesFocus };
