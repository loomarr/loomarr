// PROTOTYPE — throwaway (#4476 A1). Local stand-ins for the parts the design system does not have
// yet (each is a design-system gap named in the report): a 2:3 poster with dark, film-like
// stand-in art; tabs that read as tabs; a letter rail; the PROPOSAL frame. Colours are tokens only.
import { Badge, Text } from "@loomarr/design-system";
import type { CSSProperties, ReactNode } from "react";
import type { MockTitle } from "./mock-data";

// Artwork tints: the broadcast hues at a few percent over the darkest static, so a wall reads as
// posters without any one of them shouting. Amber (focus, primary) is left out on purpose.
const HUES = [
  "var(--color-tune)",
  "var(--color-lock)",
  "var(--color-onair)",
  "var(--color-suggest)",
  "var(--color-static-400)",
  "var(--color-caution)",
];

const mix = (hue: string, pct: number, base = "var(--color-static-950)") =>
  `color-mix(in srgb, ${hue} ${pct}%, ${base})`;
const glow = (hue: string, pct: number) => `color-mix(in srgb, ${hue} ${pct}%, transparent)`;

// Four poster compositions, so the wall has the rhythm of real, busy artwork: a horizon band, a
// lit figure, a diagonal split, a framed still. Each is gradients over the darkest static.
const artStyle = (t: Pick<MockTitle, "hue" | "layout">): CSSProperties => {
  const h = HUES[t.hue % HUES.length] ?? "var(--color-static-400)";
  const shade = glow("var(--color-static-950)", 70);
  switch (t.layout % 4) {
    case 0:
      return {
        background: [
          `radial-gradient(120% 22% at 50% 62%, ${glow(h, 38)}, transparent 70%)`,
          `linear-gradient(180deg, ${mix(h, 10)} 0%, ${mix(h, 4)} 55%, var(--color-static-950) 100%)`,
        ].join(","),
      };
    case 1:
      return {
        background: [
          `radial-gradient(28% 20% at 58% 34%, ${glow("var(--color-static-100)", 22)}, transparent 70%)`,
          `radial-gradient(60% 45% at 58% 36%, ${glow(h, 30)}, transparent 75%)`,
          `linear-gradient(200deg, ${mix(h, 16)}, var(--color-static-950) 80%)`,
        ].join(","),
      };
    case 2:
      return {
        background: [
          `linear-gradient(115deg, transparent 48%, ${shade} 48.5%)`,
          `linear-gradient(160deg, ${mix(h, 22)} 0%, ${mix(h, 6)} 60%, var(--color-static-950))`,
        ].join(","),
      };
    default:
      return {
        background: [
          `radial-gradient(90% 60% at 30% 30%, ${glow(h, 24)}, transparent 70%)`,
          `linear-gradient(0deg, var(--color-static-950), ${mix(h, 12, "var(--color-static-900)")})`,
        ].join(","),
      };
  }
};

// The stand-in art. Decorative: the tile's caption names the title once.
const PosterArt = ({
  title,
  small = false,
  dim = false,
}: {
  title: Pick<MockTitle, "name" | "year" | "hue" | "layout">;
  small?: boolean;
  dim?: boolean;
}) => {
  const layout = title.layout % 4;
  const place =
    layout === 0
      ? "justify-start text-left"
      : layout === 1
        ? "justify-end text-center items-center"
        : layout === 2
          ? "justify-end text-left"
          : "justify-start text-center items-center";
  return (
    <div
      aria-hidden
      className={`relative flex aspect-[2/3] w-full flex-col overflow-hidden rounded-md ${place} ${small ? "p-1.5" : "p-3"} ${dim ? "opacity-50" : ""}`}
      style={artStyle(title)}
    >
      {layout === 3 ? (
        <div className="border-static-100/15 pointer-events-none absolute inset-2 rounded-sm border" />
      ) : null}
      {small ? null : (
        <span
          className={`text-static-100/80 relative leading-tight ${layout % 2 === 0 ? "text-[11px] font-semibold tracking-[0.14em] uppercase" : "text-[13px] font-medium"}`}
        >
          {title.name}
        </span>
      )}
      {small || layout !== 2 ? null : (
        <span className="text-static-100/50 relative mt-1 font-mono text-[11px]">{title.year}</span>
      )}
      {small || layout === 1 || layout === 2 ? null : (
        <div className="absolute inset-x-4 bottom-3 flex flex-col items-center gap-1 opacity-30">
          <div className="bg-static-100 h-px w-full" />
          <div className="bg-static-100 h-px w-4/5" />
          <div className="bg-static-100 h-px w-3/5" />
        </div>
      )}
    </div>
  );
};

// Tabs that read as tabs: words on a shared rule, the current one underlined in the foreground
// (selection is neutral; amber is only the focus ring). Counts ride along in the data face.
// GAP: the design system's Tabs draws each tab as an outlined Action.
const UnderlineTabs = <V extends string>({
  label,
  options,
  value,
  onChange,
  bare = false,
}: {
  label: string;
  options: readonly { value: V; label: string; count?: string }[];
  value: V;
  onChange: (v: V) => void;
  bare?: boolean;
}) => (
  <div aria-label={label} className={`flex gap-6 ${bare ? "" : "border-static-700 border-b"}`} role="tablist">
    {options.map((o) => {
      const on = o.value === value;
      return (
        <button
          aria-selected={on}
          className={`focus-visible:outline-signal -mb-px flex items-baseline gap-1.5 border-b-2 pt-1 pb-2.5 text-sm font-semibold focus-visible:outline-2 focus-visible:outline-offset-2 ${on ? "border-static-100 text-static-0" : "text-static-400 hover:text-static-100 border-transparent"}`}
          key={o.value}
          onClick={() => onChange(o.value)}
          role="tab"
          tabIndex={on ? 0 : -1}
          type="button"
        >
          {o.label}
          {o.count ? <span className="text-static-400 font-mono text-xs font-normal">{o.count}</span> : null}
        </button>
      );
    })}
  </div>
);

// Jump by letter (A–Z order) or by when it arrived (Recently added): the same rail, its stops
// following the sort. GAP: no index rail in the design system.
const LETTERS = "#ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("");
const EMPTY = new Set(["#", "Q", "U", "X"]);
const JumpRail = ({ sort, current, compact = false }: { sort: "az" | "recent"; current: string; compact?: boolean }) => {
  const stops = sort === "az" ? LETTERS : ["This week", "This month", "Summer", "Spring", "2025", "2024", "Earlier"];
  return (
    <nav
      aria-label={sort === "az" ? "Jump to a letter" : "Jump to when it arrived"}
      className={`sticky top-4 flex shrink-0 flex-col items-center self-start ${compact ? "gap-0" : "gap-0.5"}`}
    >
      {stops.map((s) => {
        const empty = sort === "az" && EMPTY.has(s);
        const on = s === current;
        return (
          <button
            aria-current={on ? "true" : undefined}
            className={`focus-visible:outline-signal rounded-sm font-mono leading-none focus-visible:outline-2 ${compact ? "px-1 py-[3px] text-[11px]" : "px-1.5 py-[3px] text-xs"} ${sort === "recent" ? "w-full text-right [writing-mode:horizontal-tb]" : ""} ${on ? "bg-static-800 text-static-0 font-semibold" : empty ? "text-static-500" : "text-static-400 hover:text-static-100"}`}
            disabled={empty}
            key={s}
            type="button"
          >
            {sort === "recent" && compact ? s.replace("This ", "").slice(0, 4) : s}
          </button>
        );
      })}
    </nav>
  );
};

// A drawn-but-undecided part of the mock, marked so nobody mistakes it for the design.
const Proposal = ({ question, children, className = "" }: { question: string; children: ReactNode; className?: string }) => (
  <section
    aria-label={`Proposal: ${question}`}
    className={`border-tune/60 rounded-lg border border-dashed p-3 ${className}`}
  >
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <Badge tone="tune">Proposal</Badge>
      <Text textRole="compact" tone="info">
        {question}
      </Text>
    </div>
    {children}
  </section>
);

export { JumpRail, PosterArt, Proposal, UnderlineTabs };
