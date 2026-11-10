type LogoLook =
  | "plate"
  | "text"
  | "outline"
  | "small-plate"
  | "tint"
  | "twotone"
  | "badge"
  | "lockup"
  | "glass"
  | "stripe";
type LogoColor = "era" | "amber" | "cyan" | "green" | "red" | "pink" | "yellow";
type LogoCorner = "top-left" | "top-right" | "bottom-left" | "bottom-right";

/** The channel's saved `policy.watermark`: an omitted field inherits its default. */
interface OnScreenLogoPolicy {
  callsign?: string;
  color?: LogoColor;
  corner?: LogoCorner;
  enabled?: boolean;
  holiday?: boolean;
  image?: string;
  look?: LogoLook;
  margin?: number;
  /** The picture per holiday, where there is a choice; an omitted holiday shows its first. */
  motifs?: Record<string, string>;
  opacity?: number;
  size?: number;
}
type OnScreenLogoKey = keyof OnScreenLogoPolicy;

/** A holiday picture, as the backend names it. */
interface OnScreenLogoPicture {
  id: string;
  name: string;
}

/** A holiday this channel honours, from the backend calendar. */
interface OnScreenLogoHoliday {
  /** The window that is on today or comes next: first and last day, YYYY-MM-DD. */
  start: string;
  end: string;
  /** In effect today for this channel: the logo airs its holiday version. */
  active: boolean;
  id: string;
  name: string;
  /** The picture this channel shows, and the holiday's pictures (default first). */
  picture: string;
  pictures: readonly OnScreenLogoPicture[];
}

/** What the preview should draw: the backend renders it; `holiday` null is the everyday logo. */
interface OnScreenLogoPreviewRequest {
  color?: LogoColor;
  holiday: string | null;
  look?: LogoLook;
  /** Draw only the holiday's picture, as a small square (the picture chips). */
  motifOnly?: boolean;
  /** The holiday's picture to draw instead of the channel's. */
  picture?: string;
  size?: number;
}

interface OnScreenLogoSectionProps {
  /** Text the logo shows when the name is left blank, as the backend derives it; omitted shows "Auto". */
  autoCallsign?: string;
  /** The channel number the numbered styles show. */
  channelNumber: number;
  /** The install-wide defaults an omitted look/opacity inherits (Settings → Playback). */
  defaults: { look: LogoLook; opacity: number };
  /** False while this server cannot overlay a logo (the preview's drawable header). */
  drawable: boolean;
  /** The decade "Match the era" uses, from the channel's Programming era; omitted has none. */
  eraLabel?: string;
  /** True when the channel uses every built-in holiday rather than a chosen few. */
  everyHoliday: boolean;
  /** The holidays the channel honours; empty when its seasonal mode is "Ignore holidays". */
  holidays: readonly OnScreenLogoHoliday[];
  /** Today, for the "Today (10/13)" sentence; omitted is the device clock. */
  today?: Date;
  onChange: (
    patch: Partial<Record<OnScreenLogoKey, OnScreenLogoPolicy[OnScreenLogoKey] | undefined>>,
  ) => void;
  /** Opens the Programming tab, where Holidays are set. */
  onOpenProgramming: () => void;
  /** The image service URL of the uploaded image (from policy.image's hash); shown as its thumbnail. */
  imageSrc?: string;
  onRemoveImage: () => void;
  onUploadImage: () => void;
  policy: OnScreenLogoPolicy;
  /** The URL of the PNG the backend would air for the request. */
  previewSrc: (request: OnScreenLogoPreviewRequest) => string;
}

export type {
  LogoColor,
  LogoCorner,
  LogoLook,
  OnScreenLogoHoliday,
  OnScreenLogoKey,
  OnScreenLogoPicture,
  OnScreenLogoPolicy,
  OnScreenLogoPreviewRequest,
  OnScreenLogoSectionProps,
};
