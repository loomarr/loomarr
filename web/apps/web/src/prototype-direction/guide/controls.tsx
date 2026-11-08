// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
import type { GuideLayout } from "@loomarr/core/guide";
import type { Device, Tint } from "../shared";
import type { GuideState, Zoom } from "./data";

// What the route hands every variant.
interface VariantProps {
  device: Device;
  eras: Record<string, "1970s" | "1980s" | "1990s" | undefined>;
  layout: GuideLayout;
  onZoom: (zoom: Zoom) => void;
  state: GuideState;
  tint: Tint;
  zoom: Zoom;
}

interface SegmentedProps<T extends string | number> {
  label: string;
  onChange: (value: T) => void;
  options: readonly { label: string; value: T }[];
  value: T;
}

// A two-or-three way choice, drawn as one control (radio group semantics).
const Segmented = <T extends string | number>({ label, onChange, options, value }: SegmentedProps<T>) => (
  <fieldset
    aria-label={label}
    className="bg-card border-static-700 m-0 flex min-w-0 shrink-0 rounded-lg border p-0.5"
  >
    {options.map((option) => {
      const on = option.value === value;
      return (
        <button
          key={option.value}
          type="button"
          aria-pressed={on}
          onClick={() => onChange(option.value)}
          className={`focus-visible:ring-signal h-8 rounded-md px-3 text-sm font-semibold focus-visible:ring-2 focus-visible:outline-none ${
            on ? "bg-static-700 text-static-100" : "text-static-400 hover:text-static-100"
          }`}
        >
          {option.label}
        </button>
      );
    })}
  </fieldset>
);

const ZOOM_OPTIONS = [
  { label: "2 hours", value: 120 },
  { label: "4 hours", value: 240 },
] as const;

export type { VariantProps };
export { Segmented, ZOOM_OPTIONS };
