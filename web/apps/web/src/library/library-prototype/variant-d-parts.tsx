// PROTOTYPE — throwaway (#2151 step 2). Variant D ("Appliance") shared bits: the lit artwork
// stand-in, station bug, colour bars, lock-in moment, accordion and plain-words helpers.
// Design-system primitives only; Tailwind is layout glue. Motion is <=300ms and `motion-safe`.
import { ArtworkFrame, Surface, Text } from "@loomarr/design-system";
import { ArrowLeft, Search } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

// One page frame for the Library and for title pages, so content starts at the same x.
const FRAME = "mx-auto flex w-full max-w-6xl flex-col gap-6 p-4 md:p-8";

// A hue-lit stand-in for artwork: never a remote image. Depth is light (a gradient and a soft
// highlight), never size. Inside a `group` it lights a white ring on hover and focus.
const Poster = ({
  hue,
  title,
  big = false,
  dim = false,
  children,
}: {
  hue: number;
  title: string;
  big?: boolean;
  dim?: boolean;
  children?: ReactNode;
}) => (
  <div
    className="relative aspect-[2/3] w-full overflow-hidden rounded-lg ring-white/0 group-hover:ring-2 group-hover:ring-white group-focus-visible:ring-2 group-focus-visible:ring-white"
    style={dim ? { filter: "brightness(0.72) saturate(0.7)" } : undefined}
  >
    <div
      className="absolute inset-0"
      style={{
        backgroundImage: `radial-gradient(circle at 30% 18%, hsl(${hue} 70% 68% / 0.32), transparent 62%), linear-gradient(165deg, hsl(${hue} 42% 34%), hsl(${hue} 40% 14%))`,
      }}
    />
    <div className="absolute inset-0">
      <ArtworkFrame
        alignItems="center"
        height="100%"
        justifyContent="center"
        state="ready"
        style={{ backgroundColor: "transparent" }}
        width="100%"
      >
        <Text style={{ color: `hsl(${hue} 60% 82%)` }} textRole={big ? "display" : "title"}>
          {title.replace(/^The /, "").slice(0, 1)}
        </Text>
      </ArtworkFrame>
    </div>
    {children}
  </div>
);

// A fixed-width poster for rows and lists.
const Thumb = ({ hue, title, width = 40 }: { hue: number; title: string; width?: number }) => (
  <div className="shrink-0" style={{ width }}>
    <Poster hue={hue} title={title} />
  </div>
);

// The app's wayfinding idiom (as Settings sub-pages): "← Parent / Current".
const BackLink = ({ label, current, onBack }: { label: string; current?: string; onBack: () => void }) => (
  <nav aria-label="Where you are">
    <ol className="flex min-w-0 items-center gap-2 text-sm">
      <li>
        <button
          className="text-muted-foreground hover:text-foreground focus-visible:ring-ring inline-flex cursor-pointer items-center gap-1.5 rounded-sm focus-visible:ring-2 focus-visible:outline-none"
          onClick={onBack}
          type="button"
        >
          <ArrowLeft aria-hidden className="size-3.5" />
          {label}
        </button>
      </li>
      {current ? (
        <>
          <li aria-hidden="true" className="text-muted-foreground">
            /
          </li>
          <li className="text-foreground truncate">{current}</li>
        </>
      ) : null}
    </ol>
  </nav>
);

// A search box in the body font, the same height as the tabs beside it.
const SearchBox = ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
  <label className="border-border focus-within:border-foreground flex h-10 items-center gap-2 rounded-lg border px-3 md:w-72">
    <Search aria-hidden className="text-muted-foreground size-4" />
    <input
      aria-label="Search your library"
      className="placeholder:text-muted-foreground min-w-0 flex-1 bg-transparent text-sm outline-none"
      onChange={(e) => onChange(e.target.value)}
      placeholder="Search your library"
      type="search"
      value={value}
    />
  </label>
);

// A labelled text input in the body font (the design-system Field sets its text in monospace).
const TextBox = ({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) => (
  <label className="flex flex-col gap-1.5">
    <Text textRole="label">{label}</Text>
    <input
      className="border-border focus:border-foreground placeholder:text-muted-foreground h-11 w-full rounded-lg border bg-transparent px-3 text-sm outline-none"
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      type="text"
      value={value}
    />
  </label>
);

// A calm accordion row: label and a quiet hint in the body font, Show/Hide on the right.
const Collapse = ({
  label,
  hint,
  open,
  onToggle,
  children,
}: {
  label: string;
  hint?: string;
  open: boolean;
  onToggle: (open: boolean) => void;
  children: ReactNode;
}) => (
  <Surface gap="inline" padding={0}>
    <button
      aria-expanded={open}
      className="focus-visible:ring-ring flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-lg px-4 py-2 text-left focus-visible:ring-2 focus-visible:outline-none"
      onClick={() => onToggle(!open)}
      type="button"
    >
      <span className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-3">
        <Text textRole="label" tone="primary">
          {label}
        </Text>
        {hint ? (
          <Text textRole="compact" tone="muted">
            {hint}
          </Text>
        ) : null}
      </span>
      <Text textRole="compact" tone="secondary">
        {open ? "Hide" : "Show"}
      </Text>
    </button>
    {open ? <div className="border-border border-t">{children}</div> : null}
  </Surface>
);

// A rating chip: the same pill whatever the length.
const RatingPill = ({ children }: { children: string }) => (
  <span className="border-border inline-flex h-6 min-w-10 items-center justify-center rounded-full border px-3">
    <Text textRole="compact" tone="secondary">
      {children}
    </Text>
  </span>
);

// "CH 12 · Holiday" -> "On Holiday (channel 12)".
const channelPhrase = (raw: string): string => {
  const m = /^CH (\d+) · (.+)$/.exec(raw);
  return m ? `On ${m[2]} (channel ${m[1]})` : raw;
};

// The station bug: the channel number and its name, one baseline, one scale.
const ChannelBug = ({ raw }: { raw: string }) => {
  const m = /^CH (\d+) · (.+)$/.exec(raw);
  return (
    <span className="border-border inline-flex items-baseline gap-2 rounded-md border px-3 py-1.5">
      <Text textRole="label" tone="primary">
        {m ? `CH ${m[1]}` : "CH"}
      </Text>
      <Text textRole="label" tone="primary">
        {m ? m[2] : raw}
      </Text>
    </span>
  );
};

// Static colour bars for idle surfaces only (empty states). Never on the grid or forms.
const BARS = [0, 55, 120, 180, 215, 275, 330];
const ColorBars = () => (
  <div aria-hidden="true" className="flex h-2 w-full max-w-xs overflow-hidden rounded-full opacity-80">
    {BARS.map((h) => (
      <div className="flex-1" key={h} style={{ backgroundColor: `hsl(${h} 55% 52%)` }} />
    ))}
  </div>
);

// Signature moment: static -> clear (200ms), then the card leaves. Instant under reduced motion.
const LockIn = ({
  children,
  onDone,
  hold = 1400,
}: {
  children: ReactNode;
  onDone: () => void;
  hold?: number;
}) => {
  const [clear, setClear] = useState(false);
  useEffect(() => {
    const a = window.setTimeout(() => setClear(true), 30);
    const b = window.setTimeout(onDone, hold);
    return () => {
      window.clearTimeout(a);
      window.clearTimeout(b);
    };
  }, [onDone, hold]);
  return (
    <div
      className="motion-safe:transition-[filter,opacity] motion-safe:duration-200"
      style={clear ? undefined : { filter: "grayscale(1) blur(2px)", opacity: 0.6 }}
    >
      {children}
    </div>
  );
};

// "44:02" -> "44 min"; "1:38:01" -> "1 h 38 min".
const minutesPhrase = (clock: string): string => {
  const parts = clock.split(":").map((p) => Number.parseInt(p, 10));
  const [h, m] = parts.length === 3 ? [parts[0] ?? 0, parts[1] ?? 0] : [0, parts[0] ?? 0];
  return h > 0 ? `${h} h ${m} min` : `${m} min`;
};

const Heading = ({ children }: { children: ReactNode }) => <Text textRole="headline">{children}</Text>;

export {
  BackLink,
  ChannelBug,
  Collapse,
  ColorBars,
  channelPhrase,
  FRAME,
  Heading,
  LockIn,
  minutesPhrase,
  Poster,
  RatingPill,
  SearchBox,
  TextBox,
  Thumb,
};
