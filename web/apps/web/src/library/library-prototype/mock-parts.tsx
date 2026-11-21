// PROTOTYPE — throwaway (#4476 A1). Local stand-ins for the parts the design system does not have
// yet (each is a design-system gap named in the report): a 2:3 poster with stand-in art busy enough
// to test the wall against real posters; tabs that read as tabs; a letter rail; a file name that
// shortens in the middle; the PROPOSAL frame. Colours are tokens only.
import { Badge, SelectControl, Text } from "@loomarr/design-system";
import { type CSSProperties, type ReactNode, useState } from "react";
import type { MockTitle } from "./mock-data";

// Artwork hues. Real posters are loud, so the stand-ins are too: two hues per poster, mixed hard.
const HUES = [
  "var(--color-tune)",
  "var(--color-lock)",
  "var(--color-onair)",
  "var(--color-suggest)",
  "var(--color-caution)",
];
const hue = (i: number) => HUES[((i % HUES.length) + HUES.length) % HUES.length] ?? "var(--color-static-400)";
const mix = (c: string, pct: number, base = "var(--color-static-950)") => `color-mix(in srgb, ${c} ${pct}%, ${base})`;
const fade = (c: string, pct: number) => `color-mix(in srgb, ${c} ${pct}%, transparent)`;
const INK = "var(--color-static-950)";
const PAPER = "var(--color-static-100)";

type Art = Pick<MockTitle, "name" | "year" | "hue" | "hue2" | "layout">;

// Eight compositions, as different as real posters are: a horizon band, a lit face, a diagonal
// split, a framed still, a light paper poster, giant type, a cast collage, a silhouette at dusk.
const background = (t: Art): string => {
  const a = hue(t.hue);
  const b = hue(t.hue2);
  switch (t.layout % 8) {
    case 0:
      return `radial-gradient(130% 18% at 50% 64%, ${fade(a, 90)}, transparent 70%), linear-gradient(180deg, ${mix(b, 35)} 0%, ${mix(a, 12)} 58%, ${INK} 100%)`;
    case 1:
      return `radial-gradient(22% 16% at 55% 34%, ${PAPER}, transparent 75%), radial-gradient(55% 40% at 55% 36%, ${fade(a, 85)}, transparent 75%), linear-gradient(200deg, ${mix(b, 40)}, ${INK} 85%)`;
    case 2:
      return `linear-gradient(115deg, ${mix(a, 75)} 0 47%, ${INK} 47% 49%, ${mix(b, 55)} 49%)`;
    case 3:
      return `radial-gradient(80% 55% at 35% 35%, ${fade(a, 70)}, transparent 70%), linear-gradient(0deg, ${INK}, ${mix(b, 45, "var(--color-static-900)")})`;
    case 4:
      return `radial-gradient(45% 30% at 70% 68%, ${fade(a, 75)}, transparent 72%), linear-gradient(180deg, ${mix(PAPER, 92, a)}, ${mix(PAPER, 70, b)})`;
    case 5:
      return `linear-gradient(160deg, ${mix(a, 85)}, ${mix(b, 60)})`;
    case 6:
      return `linear-gradient(90deg, ${mix(a, 70)} 0 50%, ${mix(b, 65)} 50%)`;
    default:
      return `radial-gradient(140% 40% at 50% 108%, ${INK} 60%, transparent 61%), linear-gradient(180deg, ${mix(a, 80)} 0%, ${mix(b, 70)} 70%)`;
  }
};

const titleClass = (layout: number): string =>
  [
    "text-[11px] font-semibold tracking-[0.14em] uppercase text-static-0",
    "text-[13px] font-medium text-static-0",
    "text-sm font-bold uppercase leading-none text-static-0",
    "text-[13px] font-medium text-static-100",
    "text-base font-bold leading-tight text-static-950",
    "text-[22px] font-black uppercase leading-[0.95] tracking-tight break-words text-static-950",
    "text-xs font-bold uppercase tracking-[0.2em] text-static-0",
    "text-[13px] font-semibold tracking-wide text-static-0",
  ][layout % 8] ?? "";

const placeClass = (layout: number): string =>
  [
    "justify-start text-left",
    "justify-end items-center text-center",
    "justify-end text-left",
    "justify-start items-center text-center",
    "justify-start text-left",
    "justify-end text-left",
    "justify-end items-center text-center",
    "justify-start items-center text-center",
  ][layout % 8] ?? "";

// The stand-in art. Decorative: whatever holds it names the title once.
const PosterArt = ({ title, small = false, dim = false }: { title: Art; small?: boolean; dim?: boolean }) => {
  const layout = title.layout % 8;
  const style: CSSProperties = { background: background(title) };
  return (
    <div
      aria-hidden
      className={`relative flex aspect-[2/3] w-full flex-col overflow-hidden rounded-md ${placeClass(layout)} ${small ? "p-1.5" : "p-3"} ${dim ? "opacity-40" : ""}`}
      style={style}
    >
      {layout === 3 ? <div className="border-static-100/30 absolute inset-2 rounded-sm border" /> : null}
      {layout === 6 ? (
        <div className="absolute inset-0 grid grid-cols-2 grid-rows-2">
          <div style={{ background: `radial-gradient(40% 35% at 50% 45%, ${fade(PAPER, 55)}, transparent 70%)` }} />
          <div style={{ background: `linear-gradient(0deg, ${fade(INK, 60)}, transparent)` }} />
          <div style={{ background: `linear-gradient(0deg, ${fade(INK, 70)}, ${fade(hue(title.hue2 + 1), 50)})` }} />
          <div style={{ background: `radial-gradient(40% 35% at 50% 45%, ${fade(PAPER, 40)}, ${fade(INK, 40)})` }} />
        </div>
      ) : null}
      {small ? null : <span className={`relative ${titleClass(layout)}`}>{title.name}</span>}
      {small || (layout !== 2 && layout !== 5) ? null : (
        <span className={`relative mt-1 text-[11px] ${layout === 5 ? "text-static-950/70" : "text-static-0/70"}`}>{title.year}</span>
      )}
      {small || ![0, 3, 4].includes(layout) ? null : (
        <div className={`absolute inset-x-4 bottom-3 flex flex-col items-center gap-1 ${layout === 4 ? "opacity-40" : "opacity-50"}`}>
          <div className={`h-px w-full ${layout === 4 ? "bg-static-950" : "bg-static-100"}`} />
          <div className={`h-px w-4/5 ${layout === 4 ? "bg-static-950" : "bg-static-100"}`} />
        </div>
      )}
    </div>
  );
};

// Tabs that read as tabs: words on a shared rule, the current one underlined in the foreground
// (selection is neutral; amber is only the focus ring). GAP: the design system's Tabs draws each
// tab as an outlined Action.
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
          {o.count ? <span className="text-static-400 text-xs font-normal">{o.count}</span> : null}
        </button>
      );
    })}
  </div>
);

// Jump by letter (A–Z) or by when it arrived (Recently added). Desktop only: 24px targets.
// GAP: no index rail in the design system.
const LETTERS = "#ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("");
const EMPTY = new Set(["#", "Q", "U", "X"]);
const JumpRail = ({ sort, current }: { sort: "az" | "recent"; current: string }) => {
  const stops = sort === "az" ? LETTERS : ["This week", "This month", "Summer", "Spring", "2025", "2024", "Earlier"];
  return (
    <nav
      aria-label={sort === "az" ? "Jump to a letter" : "Jump to when it arrived"}
      className="sticky top-4 flex shrink-0 flex-col self-start"
    >
      {stops.map((s) => {
        const empty = sort === "az" && EMPTY.has(s);
        const on = s === current;
        return (
          <button
            aria-current={on ? "true" : undefined}
            className={`focus-visible:outline-signal flex h-6 min-w-7 items-center justify-center rounded-sm px-1.5 text-xs focus-visible:outline-2 ${on ? "bg-static-800 text-static-0 font-semibold" : empty ? "text-static-500" : "text-static-400 hover:bg-static-900 hover:text-static-100"}`}
            disabled={empty}
            key={s}
            type="button"
          >
            {s}
          </button>
        );
      })}
    </nav>
  );
};

// A file name that never breaks mid-word: the middle gives way with an ellipsis, and the end of
// the name and its extension always show. The only text set in the data (mono) face.
const FileName = ({ name, strong = true }: { name: string; strong?: boolean }) => {
  const dot = name.lastIndexOf(".");
  const tailFrom = Math.max(0, (dot > 0 ? dot : name.length) - 8);
  return (
    <span className={`flex min-w-0 font-mono text-[13px] ${strong ? "text-static-0 font-semibold" : "text-static-100"}`} title={name}>
      <span className="truncate whitespace-pre">{name.slice(0, tailFrom)}</span>
      <span className="shrink-0 whitespace-pre">{name.slice(tailFrom)}</span>
    </span>
  );
};

// A picker that matches the search field: the design system's SelectControl in its field look.
const Picker = <V extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: V;
  options: readonly { value: V; label: string }[];
  onChange: (v: V) => void;
}) => {
  const [open, setOpen] = useState(false);
  return (
    <SelectControl
      appearance="field"
      label={label}
      onOpenChange={setOpen}
      onValueChange={(v) => {
        onChange(v);
        setOpen(false);
      }}
      open={open}
      options={options}
      value={value}
    />
  );
};

// A drawn-but-undecided part of the mock, marked so nobody mistakes it for the design.
const Proposal = ({ question, children, className = "" }: { question: string; children: ReactNode; className?: string }) => (
  <section aria-label={`Proposal: ${question}`} className={`border-tune/60 rounded-lg border border-dashed p-3 ${className}`}>
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <Badge tone="tune">Proposal</Badge>
      <Text textRole="compact" tone="info">
        {question}
      </Text>
    </div>
    {children}
  </section>
);

export { FileName, JumpRail, Picker, PosterArt, Proposal, UnderlineTabs };
