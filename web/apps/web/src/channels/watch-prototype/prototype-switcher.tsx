// PROTOTYPE — throwaway (#2088 step 0). The floating bar that flips variants (← →), states and device
// frames. Deliberately un-styled-like-the-app (a white pill) so it never reads as part of a design.
import { useEffect } from "react";
import { DEVICES, type Device, WATCH_STATES, type WatchState } from "./prototype-state";

interface SwitcherProps {
  variants: readonly { key: string; name: string }[];
  variant: string;
  state: WatchState;
  device: Device;
  onChange: (next: { variant?: string; state?: WatchState; device?: Device }) => void;
}

const PrototypeSwitcher = ({ variants, variant, state, device, onChange }: SwitcherProps) => {
  const index = Math.max(
    0,
    variants.findIndex((v) => v.key === variant),
  );
  const cycle = (step: number) =>
    onChange({ variant: variants[(index + step + variants.length) % variants.length]?.key });
  const stepState = (step: number) => {
    const i = WATCH_STATES.indexOf(state);
    onChange({ state: WATCH_STATES[(i + step + WATCH_STATES.length) % WATCH_STATES.length] });
  };

  // [ / ] cycle variants and , / . cycle states — not the arrows, which the player itself uses.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.closest("input, textarea, select, [contenteditable]")) return;
      if (e.key === "[") cycle(-1);
      else if (e.key === "]") cycle(1);
      else if (e.key === ",") stepState(-1);
      else if (e.key === ".") stepState(1);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  if (import.meta.env.PROD) return null;
  const btn = "rounded-full px-2 py-1 hover:bg-black/10";
  return (
    <div className="fixed top-2 left-1/2 z-[100] flex -translate-x-1/2 items-center gap-2 rounded-full bg-white px-3 py-1.5 font-sans text-xs text-black shadow-2xl ring-1 ring-black/20">
      <button type="button" className={btn} onClick={() => cycle(-1)} aria-label="Previous variant">
        ◀
      </button>
      <span className="min-w-44 text-center font-semibold">
        {variants[index]?.key} — {variants[index]?.name}
      </span>
      <button type="button" className={btn} onClick={() => cycle(1)} aria-label="Next variant">
        ▶
      </button>
      <span className="mx-1 h-4 w-px bg-black/20" />
      <select
        className="rounded border border-black/20 bg-white px-1 py-0.5"
        value={state}
        onChange={(e) => onChange({ state: e.target.value as WatchState })}
        aria-label="State"
      >
        {WATCH_STATES.map((s) => (
          <option key={s}>{s}</option>
        ))}
      </select>
      <select
        className="rounded border border-black/20 bg-white px-1 py-0.5"
        value={device}
        onChange={(e) => onChange({ device: e.target.value as Device })}
        aria-label="Device"
      >
        {DEVICES.map((d) => (
          <option key={d}>{d}</option>
        ))}
      </select>
      <span className="text-black/50">[ ] variant · , . state</span>
    </div>
  );
};

export { PrototypeSwitcher };
