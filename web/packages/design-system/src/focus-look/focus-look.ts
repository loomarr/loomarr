import { useState } from "react";
import { useBehindModal } from "../modal-layer";

type FocusHandlers<E> = { onBlur?: ((event: E) => void) | null; onFocus?: ((event: E) => void) | null };

// Whether a control draws its focus look (the one amber border, §9), and the handlers that track
// it, chained to the control's own. A native Modal never blurs its opener, so a control behind an
// open modal drops the look rather than showing a second focus over the dialog's.
const useFocusLook = <E = unknown>(own: FocusHandlers<E> = {}) => {
  const [hasFocus, setHasFocus] = useState(false);
  const behindModal = useBehindModal();
  return {
    focused: hasFocus && !behindModal,
    onBlur: (event: E) => {
      setHasFocus(false);
      own.onBlur?.(event);
    },
    onFocus: (event: E) => {
      setHasFocus(true);
      own.onFocus?.(event);
    },
  };
};

export { useFocusLook };
