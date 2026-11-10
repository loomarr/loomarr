import { brandChroma } from "@loomarr/design-system";
import type { LogoColor, LogoCorner, LogoLook } from "../on-screen-logo.type";

type LookInfo = { color: boolean; help: string; label: string; number: boolean; value: LogoLook };
const TEXT_LOOK: LookInfo = {
  value: "text",
  label: "Text",
  help: "The name alone, with a soft shadow.",
  color: false,
  number: false,
};
const LOOKS: readonly LookInfo[] = [
  {
    value: "plate",
    label: "Plate",
    help: "The name cut out of a white rounded badge.",
    color: false,
    number: false,
  },
  TEXT_LOOK,
  { value: "outline", label: "Outline", help: "The name as hollow letters.", color: false, number: false },
  {
    value: "small-plate",
    label: "Small plate",
    help: "A smaller badge with no shadow.",
    color: false,
    number: false,
  },
  {
    value: "tint",
    label: "Colour plate",
    help: "The name cut out of a coloured badge.",
    color: true,
    number: false,
  },
  {
    value: "twotone",
    label: "Number + name",
    help: "Channel number in a coloured tab beside a white name plate.",
    color: true,
    number: true,
  },
  {
    value: "badge",
    label: "Number badge",
    help: "A rounded square with just the channel number.",
    color: true,
    number: true,
  },
  {
    value: "lockup",
    label: "Number | name",
    help: "Number, thin divider, name. White text only, like a network ID.",
    color: false,
    number: true,
  },
  {
    value: "glass",
    label: "Glass",
    help: "A frosted, see-through plate with a bright edge.",
    color: false,
    number: false,
  },
  {
    value: "stripe",
    label: "Era stripe",
    help: "The name with a three-colour stripe in the channel's era colours.",
    color: true,
    number: false,
  },
];
const COLORS: readonly { label: string; swatch: string; value: LogoColor }[] = [
  { value: "era", label: "Match the era", swatch: "surfaceElevated" },
  { value: "amber", label: "Amber", swatch: brandChroma[0] },
  { value: "cyan", label: "Cyan", swatch: brandChroma[3] },
  { value: "green", label: "Green", swatch: brandChroma[2] },
  { value: "red", label: "Red", swatch: brandChroma[5] },
  { value: "pink", label: "Pink", swatch: brandChroma[4] },
  { value: "yellow", label: "Yellow", swatch: brandChroma[1] },
];
const CORNERS: readonly { label: string; value: LogoCorner }[] = [
  { value: "top-left", label: "Top left" },
  { value: "top-right", label: "Top right" },
  { value: "bottom-left", label: "Bottom left" },
  { value: "bottom-right", label: "Bottom right" },
];
// Named steps (decided): the stored number is the step's value.
const SIZES = [
  { value: "0.045", label: "Small" },
  { value: "0.06", label: "Standard (default)" },
  { value: "0.08", label: "Large" },
  { value: "0.1", label: "Extra large" },
] as const;
const MARGINS = [
  { value: "0.03", label: "Tight" },
  { value: "0.05", label: "Standard (default)" },
  { value: "0.08", label: "Roomy" },
] as const;
const OPACITIES = [
  { value: "0.25", label: "Faint — 25%" },
  { value: "0.4", label: "Subtle — 40% (default)" },
  { value: "0.7", label: "Clear — 70%" },
  { value: "1", label: "Solid — 100%" },
] as const;
const SCENES = [
  { value: "bright", label: "Bright" },
  { value: "dark", label: "Dark" },
  { value: "box", label: "Old 4:3 film" },
] as const;
type Scene = (typeof SCENES)[number]["value"];

// The step nearest a stored number, so a value set through the API still shows a choice.
const nearest = (steps: readonly { value: string }[], v: number) =>
  steps.reduce((a, b) => (Math.abs(Number(b.value) - v) < Math.abs(Number(a.value) - v) ? b : a)).value;
const labelOf = (steps: readonly { label: string; value: string }[], v: string) =>
  (steps.find((s) => s.value === v)?.label ?? "").replace(" (default)", "");

// "2026-10-01" → "Oct 1": a calendar day, so read and printed in UTC.
const day = (iso: string) =>
  new Date(`${iso}T00:00:00Z`).toLocaleDateString("en-US", {
    day: "numeric",
    month: "short",
    timeZone: "UTC",
  });

export type { LookInfo, Scene };
export { COLORS, CORNERS, day, LOOKS, labelOf, MARGINS, nearest, OPACITIES, SCENES, SIZES, TEXT_LOOK };
