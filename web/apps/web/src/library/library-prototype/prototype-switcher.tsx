// PROTOTYPE — throwaway (#2151 step 2). The floating bar that flips variants, screens, scan states
// and device frames. Deliberately a white pill so it never reads as part of the design.
import { useEffect } from "react";
import { DEVICES, type Device, SCAN_STATES, type ScanState, SCREENS, type Screen } from "./prototype-state";

interface SwitcherProps {
  variants: readonly { key: string; name: string }[];
  variant: string;
  screen: Screen;
  scan: ScanState;
  device: Device;
  onChange: (next: { variant?: string; screen?: Screen; scan?: ScanState; device?: Device }) => void;
}

const step = <T,>(list: readonly T[], current: T, by: number): T =>
  list[(list.indexOf(current) + by + list.length) % list.length] as T;

const PrototypeSwitcher = ({ variants, variant, screen, scan, device, onChange }: SwitcherProps) => {
  const keys = variants.map((v) => v.key);
  const current = variants.find((v) => v.key === variant) ?? variants[0];

  // ← → cycle variants, [ ] cycle screens, , . cycle scan states — never while typing.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.closest("input, textarea, select, [contenteditable]")) return;
      if (e.key === "ArrowLeft") onChange({ variant: step(keys, variant, -1) });
      else if (e.key === "ArrowRight") onChange({ variant: step(keys, variant, 1) });
      else if (e.key === "[") onChange({ screen: step(SCREENS, screen, -1) });
      else if (e.key === "]") onChange({ screen: step(SCREENS, screen, 1) });
      else if (e.key === ",") onChange({ scan: step(SCAN_STATES, scan, -1) });
      else if (e.key === ".") onChange({ scan: step(SCAN_STATES, scan, 1) });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  if (import.meta.env.PROD) return null;
  const btn = "rounded-full px-2 py-1 hover:bg-black/10";
  const select = "rounded border border-black/20 bg-white px-1 py-0.5";
  return (
    <div className="fixed bottom-3 left-1/2 z-[100] flex -translate-x-1/2 items-center gap-2 rounded-full bg-white px-3 py-1.5 font-sans text-xs text-black shadow-2xl ring-1 ring-black/20">
      <button type="button" className={btn} onClick={() => onChange({ variant: step(keys, variant, -1) })} aria-label="Previous variant">
        ◀
      </button>
      <span className="min-w-48 text-center font-semibold">
        {current?.key} — {current?.name}
      </span>
      <button type="button" className={btn} onClick={() => onChange({ variant: step(keys, variant, 1) })} aria-label="Next variant">
        ▶
      </button>
      <span className="mx-1 h-4 w-px bg-black/20" />
      <select className={select} value={screen} onChange={(e) => onChange({ screen: e.target.value as Screen })} aria-label="Screen">
        {SCREENS.map((s) => (
          <option key={s}>{s}</option>
        ))}
      </select>
      <select className={select} value={scan} onChange={(e) => onChange({ scan: e.target.value as ScanState })} aria-label="Scan state">
        {SCAN_STATES.map((s) => (
          <option key={s}>{s}</option>
        ))}
      </select>
      <select className={select} value={device} onChange={(e) => onChange({ device: e.target.value as Device })} aria-label="Device">
        {DEVICES.map((d) => (
          <option key={d}>{d}</option>
        ))}
      </select>
      <span className="text-black/50">← → variant · [ ] screen · , . scan</span>
    </div>
  );
};

export { PrototypeSwitcher };
