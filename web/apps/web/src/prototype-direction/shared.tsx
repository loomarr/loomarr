// PROTOTYPE — throwaway (Loomarr Demo Review, 2026-11-08, "Direction" mocks). Shared scaffold for
// the /prototype/{home,guide,cards,player} routes: the device frame, the floating switcher, the
// era tint toggle and token-drawn stand-in pictures. Lives on prototype/design-direction only; it
// never reaches main.
import { brandChroma } from "@loomarr/design-system";
import { type ReactNode, useEffect } from "react";

// ---------------------------------------------------------------------------------------------
// Search params every prototype route shares.

const DEVICES = ["desktop", "phone"] as const;
type Device = (typeof DEVICES)[number];

// Direction move 1, as a toggle on every surface: neutral chrome (today's rule, #1909 Q3) or the
// channel's era colour on its chip, row, card rule and player band.
const TINTS = ["neutral", "era"] as const;
type Tint = (typeof TINTS)[number];

interface ProtoSearch {
  variant: string;
  state: string;
  device: Device;
  tint: Tint;
}

const parseSearch =
  (variants: readonly { key: string }[], states: readonly string[]) =>
  (raw: Record<string, unknown>): ProtoSearch => ({
    variant:
      typeof raw.variant === "string" && variants.some((v) => v.key === raw.variant)
        ? raw.variant
        : (variants[0]?.key ?? "A"),
    state: typeof raw.state === "string" && states.includes(raw.state) ? raw.state : (states[0] ?? ""),
    device: DEVICES.includes(raw.device as Device) ? (raw.device as Device) : "desktop",
    tint: TINTS.includes(raw.tint as Tint) ? (raw.tint as Tint) : "neutral",
  });

// ---------------------------------------------------------------------------------------------
// The era tint. The one table is internal/playout/watermark/design.go eraPalettes; a prototype
// takes each decade's first colour from the brand chroma (amber, yellow, green, cyan, magenta,
// red, grey). A channel with no era is amber, as NoEraPalette is.

const ERA_TINT: Record<string, string> = {
  "1970s": brandChroma[1],
  "1980s": brandChroma[5],
  "1990s": brandChroma[3],
};

const eraTint = (era: string | undefined, tint: Tint): string | undefined =>
  tint === "era" ? (era ? (ERA_TINT[era] ?? brandChroma[0]) : brandChroma[0]) : undefined;

// ---------------------------------------------------------------------------------------------
// Stand-in pictures, drawn from tokens (no photos, no generated art, no remote images). Film-like
// per the maintainer's Library feedback: dark, barely tinted, the title set small — never toy
// blocks, saturated gradients or a giant initial.

const standInStill = (hue: number, title?: string) =>
  `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360" viewBox="0 0 640 360"><defs><linearGradient id="g" x1="0" y1="0" x2="0.6" y2="1"><stop offset="0" stop-color="hsl(${hue} 22% 26%)"/><stop offset="0.55" stop-color="hsl(${hue + 20} 16% 14%)"/><stop offset="1" stop-color="hsl(${hue + 30} 12% 7%)"/></linearGradient><radialGradient id="l" cx="0.68" cy="0.32" r="0.45"><stop offset="0" stop-color="hsl(${hue + 10} 40% 70%)" stop-opacity="0.35"/><stop offset="1" stop-color="hsl(${hue} 30% 40%)" stop-opacity="0"/></radialGradient></defs><rect width="640" height="360" fill="url(#g)"/><rect width="640" height="360" fill="url(#l)"/><rect y="250" width="640" height="110" fill="black" opacity="0.25"/>${
      title
        ? `<text x="24" y="336" font-family="Geist, sans-serif" font-size="15" font-weight="500" fill="white" fill-opacity="0.7">${title.replace(/[<&>]/g, "")}</text>`
        : ""
    }</svg>`,
  )}`;

// The ImageDTO shape @loomarr/ui components take (thumbImage, posters), around a stand-in still.
const standInImage = (hue: number, title?: string, width = 640, height = 360) => ({
  animated: false,
  dominantHex: "#222222",
  hash: `stand-in-${hue}-${title ?? ""}`,
  height,
  placeholder: "",
  role: "thumb",
  src: standInStill(hue, title),
  srcSetAvif: "",
  srcSetWebp: "",
  width,
});

// A live "frame of video" with real light and dark areas (the #2088 prototype's dusk seascape), so
// text over a picture is judged against something other than a flat field.
const LivePicture = () => (
  <div aria-hidden className="absolute inset-0 overflow-hidden">
    <div className="absolute inset-0 bg-[linear-gradient(180deg,#1b2a4a_0%,#4a3b6b_38%,#c8743f_58%,#f2b36b_62%,#20304a_63%,#0d1626_100%)]" />
    <div className="absolute top-[30%] left-[62%] aspect-square w-[7%] rounded-full bg-[#ffd9a0] opacity-80 blur-[2px]" />
    <div className="absolute top-[36%] left-[18%] h-[27%] w-[2.2%] bg-[linear-gradient(180deg,#e9e2d0,#8a8478)]" />
    <div className="absolute top-[33%] left-[17.4%] h-[4%] w-[3.4%] rounded-t-sm bg-[#3a3f4a]" />
    <div className="absolute top-[34%] left-[19.1%] h-[1.5%] w-[24%] origin-left -rotate-6 bg-[linear-gradient(90deg,rgba(255,236,170,.75),transparent)] blur-[3px]" />
    <div className="absolute top-[63%] right-0 left-0 h-[37%] bg-[repeating-linear-gradient(180deg,rgba(255,255,255,.05)_0_2px,transparent_2px_9px)]" />
  </div>
);

// ---------------------------------------------------------------------------------------------
// The device frame. Desktop is a 1440 × 900 window; phone is 390 × 844. `landscape` exists for
// the player route only (844 × 390).

const frameClass = (device: string) =>
  device === "phone" || device === "portrait"
    ? "h-[844px] w-[390px] rounded-[36px] ring-8 ring-static-800"
    : device === "landscape"
      ? "h-[390px] w-[844px] rounded-[36px] ring-8 ring-static-800"
      : device === "tv"
        ? // A television: 960 × 540 layout units (what both TV apps lay out at), drawn at 1.5×.
          "h-[540px] w-[960px] [zoom:1.5] ring-[10px] ring-static-800"
        : "h-[900px] w-[1440px] ring-1 ring-static-800";

const ProtoFrame = ({ device, children }: { device: string; children: ReactNode }) => (
  <div className="bg-static-950 flex min-h-dvh items-center justify-center p-12">
    <div
      data-proto-frame
      className={`bg-background text-foreground relative overflow-hidden ${frameClass(device)}`}
    >
      {children}
    </div>
  </div>
);

// ---------------------------------------------------------------------------------------------
// The floating switcher: [ ] variant, , . state, D device, T tint. A white pill, deliberately unlike
// the app, so it never reads as part of a design.

interface SwitcherProps {
  variants: readonly { key: string; name: string }[];
  states: readonly string[];
  devices?: readonly string[];
  search: { variant: string; state: string; device: string; tint: Tint };
  // The player route's portrait/landscape ride through this as a cast.
  onChange: (next: Partial<{ variant: string; state: string; device: Device; tint: Tint }>) => void;
}

const ProtoSwitcher = ({ variants, states, devices = DEVICES, search, onChange }: SwitcherProps) => {
  const vi = Math.max(
    0,
    variants.findIndex((v) => v.key === search.variant),
  );
  const step = <T,>(list: readonly T[], current: T, by: number) =>
    list[(list.indexOf(current) + by + list.length) % list.length];

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.closest("input, textarea, select, [contenteditable]")) return;
      if (e.key === "[") onChange({ variant: step(variants, variants[vi], -1)?.key });
      else if (e.key === "]") onChange({ variant: step(variants, variants[vi], 1)?.key });
      else if (e.key === ",") onChange({ state: step(states, search.state, -1) });
      else if (e.key === ".") onChange({ state: step(states, search.state, 1) });
      else if (e.key === "D") onChange({ device: step(devices, search.device, 1) as Device });
      else if (e.key === "T") onChange({ tint: search.tint === "era" ? "neutral" : "era" });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  if (import.meta.env.PROD) return null;
  const btn = "rounded-full px-2 py-1 hover:bg-black/10";
  const select = "rounded border border-black/20 bg-white px-1 py-0.5";
  return (
    <div className="fixed top-2 left-1/2 z-[100] flex -translate-x-1/2 items-center gap-2 rounded-full bg-white px-3 py-1.5 font-sans text-xs text-black shadow-2xl ring-1 ring-black/20">
      <button
        type="button"
        className={btn}
        onClick={() => onChange({ variant: step(variants, variants[vi], -1)?.key })}
      >
        ◀
      </button>
      <span className="min-w-44 text-center font-semibold">
        {variants[vi]?.key} — {variants[vi]?.name}
      </span>
      <button
        type="button"
        className={btn}
        onClick={() => onChange({ variant: step(variants, variants[vi], 1)?.key })}
      >
        ▶
      </button>
      {states.length > 1 ? (
        <select
          aria-label="State"
          className={select}
          value={search.state}
          onChange={(e) => onChange({ state: e.target.value })}
        >
          {states.map((s) => (
            <option key={s}>{s}</option>
          ))}
        </select>
      ) : null}
      <select
        aria-label="Device"
        className={select}
        value={search.device}
        onChange={(e) => onChange({ device: e.target.value as Device })}
      >
        {devices.map((d) => (
          <option key={d}>{d}</option>
        ))}
      </select>
      <select
        aria-label="Tint"
        className={select}
        value={search.tint}
        onChange={(e) => onChange({ tint: e.target.value as Tint })}
      >
        {TINTS.map((t) => (
          <option key={t}>{t}</option>
        ))}
      </select>
      <span className="text-black/50">[ ] variant · , . state · D device · T tint</span>
    </div>
  );
};

export type { Device, ProtoSearch, Tint };
export {
  DEVICES,
  eraTint,
  LivePicture,
  ProtoFrame,
  ProtoSwitcher,
  parseSearch,
  standInImage,
  standInStill,
  TINTS,
};
