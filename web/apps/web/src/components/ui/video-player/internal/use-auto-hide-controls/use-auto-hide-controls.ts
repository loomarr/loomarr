import { useCallback, useEffect, useMemo, useRef, useState } from "react";

// How long the controls linger before hiding. IDLE is the "mouse present but still" timeout
// (Emby-style). GRACE is the shorter linger after the pointer LEAVES the frame (or a menu closes with
// the pointer elsewhere) — long enough that a quick over-and-back doesn't flicker.
const IDLE_MS = 2800;
const GRACE_MS = 600;

// useAutoHideControls owns the overlay's show/hide behaviour — the Emby/Jellyfin auto-hide that
// native `<video controls>` gives free but custom controls must build. Extracted from VideoPlayer so
// the timer/ref/race logic (the fiddliest part of the player) is one testable unit.
//
// Hiding is a decision over two FACTS re-checked at the timer's fire time, not a race between events:
//  - `held`: a bar control (an open menu) is interacting; while > 0 the controls never hide, else the
//    auto-hide would pull the bar out from under the open menu.
//  - `hovering`: the pointer is over the frame. "Hover off" and "menu closed while the pointer is
//    elsewhere" both hide because the pointer is not here — not because a timer happened to fire.
// Each is mirrored into a ref so the fire-time callback sees the live value; the state copies drive
// the re-render + effect. `holdControls` is handed to HoldControlsContext so caller-supplied bar
// controls (which the player never sees at author time) can raise/lower `held`.
const useAutoHideControls = (playing: boolean) => {
  const [controlsShown, setControlsShown] = useState(true);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [hovering, setHovering] = useState(false);
  const hoveringRef = useRef(false);
  const [held, setHeld] = useState(0);
  const heldRef = useRef(0);

  const holdControls = useMemo(
    () => ({
      hold: (on: boolean) =>
        setHeld((n) => {
          const next = Math.max(0, n + (on ? 1 : -1));
          heldRef.current = next;
          return next;
        }),
    }),
    [],
  );

  // armHide schedules a hide, re-checking the live facts at fire time so a countdown started before a
  // menu opened or the pointer returned still cancels itself. `after` is IDLE while the pointer rests
  // on the frame, GRACE once it has left.
  const armHide = useCallback((after: number) => {
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => {
      if (heldRef.current === 0 && !hoveringRef.current) setControlsShown(false);
    }, after);
  }, []);

  // Pointer entered / moved over the frame: mark hovering (ref in sync for the leave-grace timer's
  // fire-time check) and reveal, re-arming the idle window.
  const onPointerActive = useCallback(() => {
    hoveringRef.current = true;
    setHovering(true);
    setControlsShown(true);
    // While playing, a still mouse over the frame hides after the idle window; paused stays shown.
    if (playing) armHide(IDLE_MS);
    else if (hideTimer.current) clearTimeout(hideTimer.current);
  }, [playing, armHide]);

  // Pointer left the frame: drop hovering. The effect below (dep: hovering) then arms the short GRACE
  // hide — we don't hide inline so open menus (which portal OUTSIDE the frame, firing leave) still win
  // via `held`.
  const onPointerLeave = useCallback(() => {
    hoveringRef.current = false;
    setHovering(false);
  }, []);

  // Focus reveals but must not claim "hovering" (a still pointer elsewhere shouldn't be pinned shown by
  // a focus). It just shows and, while playing, arms the idle window like a pointer would.
  const revealControls = useCallback(() => {
    setControlsShown(true);
    if (playing) armHide(IDLE_MS);
    else if (hideTimer.current) clearTimeout(hideTimer.current);
  }, [playing, armHide]);

  useEffect(() => {
    // Paused, or a control is holding (open menu), or the pointer is over the frame ⇒ keep controls
    // shown. When none of those hold and we're playing, the pointer is off the frame (or a menu just
    // closed with the pointer elsewhere) — hide after the short GRACE rather than lingering.
    if (!playing || held > 0 || hovering) {
      setControlsShown(true);
      if (hideTimer.current) clearTimeout(hideTimer.current);
      return;
    }
    armHide(GRACE_MS);
    return () => {
      if (hideTimer.current) clearTimeout(hideTimer.current);
    };
  }, [playing, held, hovering, armHide]);

  return { controlsShown, holdControls, onPointerActive, onPointerLeave, revealControls };
};

export { useAutoHideControls };
