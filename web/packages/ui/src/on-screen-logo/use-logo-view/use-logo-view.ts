import { useWindowWidth } from "@loomarr/design-system";
import { useEffect, useState } from "react";
import { day, LOOKS, MARGINS, nearest, OPACITIES, type Scene, SIZES, TEXT_LOOK } from "../choices";
import type { OnScreenLogoKey, OnScreenLogoSectionProps } from "../on-screen-logo.type";

// What the section derives from the saved policy and its own view state: the effective look, size
// and corner, which rows are paused, which holiday the preview shows, and the sentences about it.
const useLogoView = ({
  defaults,
  holidays,
  onChange,
  policy: p,
  previewSrc,
  today = new Date(),
}: Pick<
  OnScreenLogoSectionProps,
  "defaults" | "holidays" | "onChange" | "policy" | "previewSrc" | "today"
>) => {
  // Not useWindowDimensions() from react-native: a component body that calls a react-native import
  // makes Storybook's docgen parse the package's Flow source and fail the build.
  const width = useWindowWidth();
  const phone = width < 720;
  const [scene, setScene] = useState<Scene>("bright");
  const [zoom, setZoom] = useState(true);
  // undefined follows today: the holiday in effect, else the everyday logo.
  const [view, setView] = useState<string | null | undefined>(undefined);
  const [open, setOpen] = useState<"size" | "margin" | "opacity" | null>(null);
  const [callsign, setCallsign] = useState(p.callsign ?? "");
  // The box is a draft of the saved name: follow it when it changes underneath (Reset to default,
  // a save that was reverted), or the next blur would write the stale draft back.
  useEffect(() => setCallsign(p.callsign ?? ""), [p.callsign]);

  const enabled = p.enabled !== false;
  const lookValue = p.look ?? defaults.look;
  const look = LOOKS.find((l) => l.value === lookValue) ?? TEXT_LOOK;
  const img = !!p.image;
  const ignoresHolidays = holidays.length === 0;
  const holidayOn = p.holiday !== false && !ignoresHolidays;
  const colorOn = look.color && !img && enabled;
  const corner = p.corner ?? "top-right";
  const size = nearest(SIZES, p.size ?? 0.06);
  const margin = nearest(MARGINS, p.margin ?? 0.05);
  const opacity = nearest(OPACITIES, p.opacity ?? defaults.opacity);
  const showAs = holidayOn && enabled && !img;
  const active = holidays.find((h) => h.active);
  const shown = view === undefined ? (active?.id ?? null) : view;
  const hol = showAs && shown && holidays.some((h) => h.id === shown) ? shown : null;
  const holShown = holidays.find((h) => h.id === hol);
  // The consequence names the holiday in effect, or the first one the channel honours.
  const named = active ?? holidays[0];
  const holSentence = named
    ? `From ${day(named.start)} to ${day(named.end)} this channel shows the ${named.name} logo.`
    : "";
  const todayLabel = `${today.getMonth() + 1}/${today.getDate()}`;
  const reset = (k: OnScreenLogoKey) => () => onChange({ [k]: undefined });
  const src = previewSrc({ color: p.color, holiday: hol, look: p.look, size: Number(size) });

  return {
    active,
    callsign,
    colorOn,
    corner,
    enabled,
    hol,
    holShown,
    holSentence,
    holidayOn,
    ignoresHolidays,
    img,
    look,
    lookValue,
    margin,
    open,
    opacity,
    phone,
    reset,
    scene,
    setCallsign,
    setOpen,
    setScene,
    setView,
    setZoom,
    showAs,
    size,
    src,
    todayLabel,
    zoom,
  };
};

type LogoView = ReturnType<typeof useLogoView>;

export type { LogoView };
export { useLogoView };
