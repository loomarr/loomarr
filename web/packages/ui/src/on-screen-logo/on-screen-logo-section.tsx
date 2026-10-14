import {
  Action,
  Badge,
  brandChroma,
  Field,
  FieldHint,
  SelectControl,
  Surface,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { type ReactNode, useEffect, useState } from "react";
import { Image, Pressable, useWindowDimensions, View } from "react-native";

// ON-SCREEN LOGO — the channel page's per-channel logo section (#1659; approved mock
// project/prototypes/watermark.html, "Channel page → Channel info"). Presentational: the app
// owns the policy, the autosave PATCH and the preview URL. The preview is the backend's PNG, the
// exact image that airs, laid on three frames; nothing here redraws the logo.

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
  onRemoveImage: () => void;
  onUploadImage: () => void;
  policy: OnScreenLogoPolicy;
  /** The URL of the PNG the backend would air for the request. */
  previewSrc: (request: OnScreenLogoPreviewRequest) => string;
}

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
  { value: "era", label: "Match the era", swatch: "$surfaceElevated" },
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
  { value: "0.4", label: "Subtle — 40%" },
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

const row = { alignItems: "center", flexDirection: "row", gap: 8 } as const;
// A radio group laid out as a wrapping row; Surface carries the role (Text drops it on web).
const group = { borderWidth: 0, flexDirection: "row", flexWrap: "wrap", gap: 8 } as const;

interface RowProps {
  children: ReactNode;
  field?: OnScreenLogoKey;
  help: string;
  hint?: string;
  label: string;
  off?: boolean;
  onReset?: () => void;
  overridden?: boolean;
}

// One field row (the Programming tab's PolicyField shape): label + "?" help, Default/Channel
// override with Reset to default, the control, then a plain-language hint.
const Row = ({ children, field, help, hint, label, off, onReset, overridden }: RowProps) => {
  const [showHelp, setShowHelp] = useState(false);
  return (
    <View style={{ gap: 8, opacity: off ? 0.55 : 1 }} testID={field ? `logo-field-${field}` : undefined}>
      <View style={{ ...row, flexWrap: "wrap", justifyContent: "space-between" }}>
        <View style={{ ...row, gap: 6 }}>
          <Text textRole="label">{label}</Text>
          <Action
            accessibilityLabel={`About ${label}`}
            onPress={() => setShowHelp((v) => !v)}
            tone="secondary"
            variant="ghost"
          >
            ?
          </Action>
        </View>
        {onReset ? (
          <View style={row}>
            <Badge tone={overridden ? "warning" : "neutral"}>
              {overridden ? "Channel override" : "Default"}
            </Badge>
            <Action
              accessibilityLabel={`Reset to default (${label})`}
              disabled={!overridden}
              onPress={onReset}
              tone="secondary"
              variant="link"
            >
              Reset to default
            </Action>
          </View>
        ) : null}
      </View>
      {showHelp ? <FieldHint>{help}</FieldHint> : null}
      {children}
      {hint ? <FieldHint>{hint}</FieldHint> : null}
    </View>
  );
};

interface PickProps<V extends string> {
  label: string;
  onPick: (v: V) => void;
  options: readonly { label: string; value: V; visual?: ReactNode }[];
  value: V | undefined;
}

// A row of selectable cards (style thumbnails, scene buttons, "Show as"): one radio group.
const Pick = <V extends string>({ label, onPick, options, value }: PickProps<V>) => (
  <Surface aria-label={label} role="radiogroup" {...group}>
    {options.map((o) => {
      const checked = o.value === value;
      return (
        <Pressable
          accessibilityLabel={o.label}
          accessibilityRole="radio"
          aria-checked={checked}
          key={o.value}
          onPress={() => onPick(o.value)}
        >
          <Surface
            alignItems="center"
            backgroundColor={checked ? "$surfaceFocus" : "$surfaceRaised"}
            borderColor={checked ? "$actionPrimary" : "$borderControl"}
            borderRadius="$control"
            borderWidth={checked ? 2 : 1}
            gap={6}
            minWidth={o.visual ? 104 : undefined}
            paddingHorizontal={12}
            paddingVertical={8}
          >
            {o.visual}
            <Text textRole="metadata">{o.label}</Text>
          </Surface>
        </Pressable>
      );
    })}
  </Surface>
);

const FRAME_BG: Record<Scene, string> = {
  bright: "$contentSecondary",
  dark: "$surfaceCanvas",
  box: "$surfaceCanvas",
};

interface FrameProps {
  corner: LogoCorner;
  enabled: boolean;
  margin: number;
  opacity: number;
  scene: Scene;
  src: string;
  zoom: boolean;
}

// One 16:9 frame with the aired PNG anchored inside the active picture (decision 0036): the PNG
// is drawn for a 1080-line frame, so its own height over 1080 is its share of the picture.
const Frame = ({ corner, enabled, margin, opacity, scene, src, zoom }: FrameProps) => {
  const [dims, setDims] = useState<{ height: number; width: number } | null>(null);
  const [v, h] = corner.split("-");
  // Image.getSize works on web and native; the Image onLoad event carries no source size on web.
  useEffect(() => {
    if (!enabled) return;
    let current = true;
    Image.getSize(
      src,
      (width, height) => current && setDims({ height, width }),
      () => current && setDims(null),
    );
    return () => {
      current = false;
    };
  }, [enabled, src]);
  const share = dims ? (dims.height / 1080) * (zoom ? 2 : 1) : 0;
  const inset = `${margin * 100}%`;
  return (
    <Surface
      aria-label={`${SCENES.find((s) => s.value === scene)?.label} frame with the logo ${enabled ? corner.replace("-", " ") : "off"}`}
      role="img"
      aspectRatio={16 / 9}
      backgroundColor="$surfaceCanvas"
      borderRadius="$control"
      overflow="hidden"
      width="100%"
      alignItems="center"
    >
      <Surface
        backgroundColor={FRAME_BG[scene]}
        borderRadius={0}
        borderWidth={0}
        height="100%"
        position="relative"
        width={scene === "box" ? "75%" : "100%"}
      >
        {enabled ? (
          <Image
            accessibilityIgnoresInvertColors
            source={{ uri: src }}
            style={{
              aspectRatio: dims ? dims.width / dims.height : 3,
              height: `${share * 100}%`,
              opacity,
              position: "absolute",
              [v === "top" ? "top" : "bottom"]: inset,
              [h === "left" ? "left" : "right"]: inset,
            }}
          />
        ) : null}
      </Surface>
    </Surface>
  );
};

// "2026-10-01" → "Oct 1": a calendar day, so read and printed in UTC.
const day = (iso: string) =>
  new Date(`${iso}T00:00:00Z`).toLocaleDateString("en-US", {
    day: "numeric",
    month: "short",
    timeZone: "UTC",
  });

const OnScreenLogoSection = ({
  autoCallsign,
  channelNumber,
  defaults,
  drawable,
  eraLabel,
  everyHoliday,
  holidays,
  today = new Date(),
  onChange,
  onOpenProgramming,
  onRemoveImage,
  onUploadImage,
  policy: p,
  previewSrc,
}: OnScreenLogoSectionProps) => {
  const { width } = useWindowDimensions();
  const phone = width < 720;
  const [scene, setScene] = useState<Scene>("bright");
  const [zoom, setZoom] = useState(false);
  // undefined follows today: the holiday in effect, else the everyday logo.
  const [view, setView] = useState<string | null | undefined>(undefined);
  const [open, setOpen] = useState<"size" | "margin" | "opacity" | null>(null);
  const [callsign, setCallsign] = useState(p.callsign ?? "");

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

  const fields = (
    <View style={{ flex: phone ? undefined : 1, gap: 24, minWidth: 0 }}>
      <Row
        field="enabled"
        help="A small logo in the corner of movies and episodes. It never appears on commercials, bumpers or station IDs."
        label="Show the channel logo"
        onReset={reset("enabled")}
        overridden={p.enabled !== undefined}
      >
        <Toggle
          checked={enabled}
          kind="switch"
          label={enabled ? "On" : "Off"}
          onCheckedChange={(on) => onChange({ enabled: on })}
        />
      </Row>
      <Row
        field="look"
        help="How the logo Loomarr draws from this channel looks. Pick one and watch the preview. A logo you upload keeps its own shape."
        hint={img ? "Your own image is in use, so styles are paused." : `${look.label}: ${look.help}`}
        label="Style"
        off={!enabled || img}
        onReset={reset("look")}
        overridden={p.look !== undefined}
      >
        <Pick
          label="Style"
          onPick={(v) => onChange({ look: v })}
          options={LOOKS.map((l) => ({
            label: l.label,
            value: l.value,
            visual: (
              <Image
                accessibilityIgnoresInvertColors
                resizeMode="contain"
                source={{ uri: previewSrc({ color: p.color, holiday: null, look: l.value }) }}
                style={{ height: 26, width: 80 }}
              />
            ),
          }))}
          value={img ? undefined : lookValue}
        />
      </Row>
      <Row
        field="color"
        help="Used by the coloured styles. “Match the era” picks colours from the channel’s decade."
        hint={
          !colorOn && !img && enabled
            ? "This style is white, so colour does not apply."
            : colorOn && (p.color ?? "era") === "era"
              ? eraLabel
                ? `${eraLabel} palette for this channel.`
                : "This channel has no era on its Programming tab, so it uses amber."
              : undefined
        }
        label="Colour"
        off={!colorOn}
        onReset={reset("color")}
        overridden={p.color !== undefined}
      >
        <Surface aria-label="Colour" role="radiogroup" {...group}>
          {COLORS.map((c) => {
            const checked = (p.color ?? "era") === c.value;
            return (
              <Pressable
                accessibilityLabel={c.label}
                accessibilityRole="radio"
                aria-checked={checked}
                disabled={!colorOn}
                key={c.value}
                onPress={() => onChange({ color: c.value })}
              >
                <Surface
                  backgroundColor={c.swatch}
                  borderColor={checked ? "$actionPrimary" : "$borderControl"}
                  borderRadius="$round"
                  borderWidth={checked ? 3 : 1}
                  height={32}
                  width={32}
                />
              </Pressable>
            );
          })}
        </Surface>
      </Row>
      <Row
        field="callsign"
        help="Up to 12 characters. Leave blank and Loomarr makes one from the channel name."
        hint={
          look.number && !img ? `Styles with a number use the channel number, ${channelNumber}.` : undefined
        }
        label="Name on the logo"
        off={!enabled || img}
        onReset={reset("callsign")}
        overridden={p.callsign !== undefined}
      >
        <View style={{ maxWidth: 176 }}>
          <Field
            accessibilityLabel="Name on the logo"
            maxLength={12}
            onBlur={() => {
              const next = callsign.trim();
              if (next !== (p.callsign ?? "")) onChange({ callsign: next || undefined });
            }}
            onChangeText={setCallsign}
            placeholder={autoCallsign ? `Auto: ${autoCallsign}` : "Auto"}
            value={callsign}
          />
        </View>
      </Row>
      <Row
        field="holiday"
        help="Adds a small picture beside the logo while a holiday is on, such as a pumpkin in October. The picture and colours are built in. It stops when the holiday ends."
        hint={img ? "Your own logo airs unchanged, so holiday touches are paused." : undefined}
        label="Holiday touches"
        off={img}
        onReset={ignoresHolidays ? undefined : reset("holiday")}
        overridden={p.holiday !== undefined}
      >
        {ignoresHolidays ? (
          <View style={{ gap: 6 }}>
            <Text textRole="hint" tone="muted">
              This channel ignores holidays, so its logo stays the same all year. To change that, set Holidays
              on its Programming tab.
            </Text>
            <Action onPress={onOpenProgramming} tone="secondary" variant="link">
              Open Programming
            </Action>
          </View>
        ) : (
          <View style={{ gap: 6 }}>
            <Toggle
              checked={holidayOn}
              kind="switch"
              label={holidayOn ? "On" : "Off"}
              onCheckedChange={(on) => onChange({ holiday: on })}
            />
            {holidayOn ? (
              <>
                {holidays.map((h) => (
                  <View
                    key={h.id}
                    style={{
                      alignItems: "center",
                      columnGap: 12,
                      flexDirection: "row",
                      flexWrap: "wrap",
                      rowGap: 6,
                    }}
                  >
                    <Text textRole="metadata">{h.name}</Text>
                    <FieldHint>{`${day(h.start)} – ${day(h.end)}`}</FieldHint>
                    {h.active ? <Badge tone="lock">In effect now</Badge> : null}
                    {h.pictures.length > 1 ? (
                      <Pick
                        label={`Picture for ${h.name}`}
                        onPick={(m) => onChange({ motifs: { ...p.motifs, [h.id]: m } })}
                        options={h.pictures.map((m) => ({
                          label: m.name,
                          value: m.id,
                          visual: (
                            <Image
                              accessibilityIgnoresInvertColors
                              resizeMode="contain"
                              source={{
                                uri: previewSrc({
                                  color: p.color,
                                  holiday: h.id,
                                  look: p.look,
                                  picture: m.id,
                                }),
                              }}
                              style={{ height: 26, width: 80 }}
                            />
                          ),
                        }))}
                        value={h.picture}
                      />
                    ) : (
                      <View style={{ alignItems: "center", flexDirection: "row", gap: 6 }}>
                        <Image
                          accessibilityIgnoresInvertColors
                          resizeMode="contain"
                          source={{ uri: previewSrc({ color: p.color, holiday: h.id, look: p.look }) }}
                          style={{ height: 26, width: 80 }}
                        />
                        <Text textRole="metadata">{h.pictures[0]?.name ?? ""}</Text>
                      </View>
                    )}
                  </View>
                ))}
                <FieldHint>
                  {`Holidays come from this channel’s Programming tab (${everyHoliday ? "it uses every built-in holiday" : `it uses ${holidays.map((h) => h.name).join(", ")}`}). ${holSentence} ${holidays.length > 1 ? "The other holidays follow the dates above; on all other days it shows the everyday logo." : "On every other day it shows the everyday logo."}`}
                </FieldHint>
                <FieldHint>
                  The change happens at the next programme that starts after the date. A programme already
                  airing keeps its logo until it ends.
                </FieldHint>
              </>
            ) : (
              <FieldHint>Off: the everyday logo airs all year.</FieldHint>
            )}
          </View>
        )}
      </Row>
      <Row
        field="corner"
        help="Where the logo sits on the picture."
        hint="The logo sits inside the picture, so a letterboxed film keeps it off the black bars."
        label="Position"
        onReset={reset("corner")}
        overridden={p.corner !== undefined}
      >
        <Surface aria-label="Position" role="radiogroup" {...group} gap={6} width={92}>
          {CORNERS.map((c) => {
            const checked = corner === c.value;
            return (
              <Pressable
                accessibilityLabel={c.label}
                accessibilityRole="radio"
                aria-checked={checked}
                key={c.value}
                onPress={() => onChange({ corner: c.value })}
              >
                <Surface
                  backgroundColor={checked ? "$actionPrimary" : "$surfaceRaised"}
                  borderColor={checked ? "$actionPrimary" : "$borderControl"}
                  borderRadius="$control"
                  borderWidth={1}
                  height={28}
                  width={40}
                />
              </Pressable>
            );
          })}
        </Surface>
      </Row>
      <Row
        field="size"
        help="How big the logo is. Wide names are made shorter so every logo covers about the same area."
        label="Size"
        onReset={reset("size")}
        overridden={p.size !== undefined}
      >
        <SelectControl
          label="Size"
          onOpenChange={(o) => setOpen(o ? "size" : null)}
          onValueChange={(v) => onChange({ size: Number(v) })}
          open={open === "size"}
          options={SIZES}
          value={size}
        />
      </Row>
      <Row
        field="margin"
        help="Keeps the logo off the very edge, where some TVs crop. Standard is the broadcast safe area."
        label="Distance from the edge"
        onReset={reset("margin")}
        overridden={p.margin !== undefined}
      >
        <SelectControl
          label="Distance from the edge"
          onOpenChange={(o) => setOpen(o ? "margin" : null)}
          onValueChange={(v) => onChange({ margin: Number(v) })}
          open={open === "margin"}
          options={MARGINS}
          value={margin}
        />
      </Row>
      <Row
        field="opacity"
        help="Faint blends in; Solid is easiest to read. Settings → Playback holds the default for every channel."
        label="How see-through"
        onReset={reset("opacity")}
        overridden={p.opacity !== undefined}
      >
        <SelectControl
          label="How see-through"
          onOpenChange={(o) => setOpen(o ? "opacity" : null)}
          onValueChange={(v) => onChange({ opacity: Number(v) })}
          open={open === "opacity"}
          options={OPACITIES}
          value={opacity}
        />
      </Row>
      <Row
        help="Upload a PNG or WebP. It keeps its own colours and shape, and replaces the styles above."
        label="Your own logo"
      >
        <View style={{ ...row, flexWrap: "wrap" }}>
          {img ? (
            <>
              <Text textRole="metadata">Your image is in use.</Text>
              <Action onPress={onRemoveImage} tone="secondary">
                Remove
              </Action>
            </>
          ) : (
            <>
              <Action onPress={onUploadImage} tone="secondary">
                Upload an image
              </Action>
              <FieldHint>PNG or WebP. Transparent background works best.</FieldHint>
            </>
          )}
        </View>
      </Row>
    </View>
  );

  const what = enabled
    ? `${img ? "Your image" : holShown ? `${look.label} with a ${holShown.name} ${(holShown.pictures.find((m) => m.id === holShown.picture)?.name ?? "").toLowerCase()}` : look.label}, ${corner.replace("-", " ")}, ${labelOf(OPACITIES, opacity).toLowerCase()}.`
    : "No logo on this channel.";
  const preview = (
    <View style={{ gap: 12, width: phone ? "100%" : 420 }} testID="logo-preview">
      <Pick label="Preview picture" onPick={setScene} options={SCENES} value={scene} />
      <Toggle
        checked={zoom}
        kind="checkbox"
        label="Zoom the logo ×2 to inspect it (real size is the Size setting)"
        onCheckedChange={setZoom}
      />
      {showAs ? (
        <View style={{ gap: 6 }}>
          <Text textRole="hint" tone="muted">
            Show as
          </Text>
          <Pick
            label="Show the logo as"
            onPick={(v) => setView(v === "none" ? null : v)}
            options={[
              { value: "none", label: "Everyday" },
              ...holidays.map((h) => ({ value: h.id, label: h.name })),
            ]}
            value={hol ?? "none"}
          />
        </View>
      ) : null}
      {(() => {
        const frame = { corner, enabled, margin: Number(margin), opacity: Number(opacity), src, zoom };
        return (
          <>
            <Frame {...frame} scene={scene} />
            <View style={row}>
              {SCENES.filter((s) => s.value !== scene).map((s) => (
                <View key={s.value} style={{ flex: 1 }}>
                  <Frame {...frame} scene={s.value} />
                </View>
              ))}
            </View>
          </>
        );
      })()}
      <Surface backgroundColor="$surfaceRaised" borderRadius="$control" gap={6} padding={12}>
        <Text textRole="hint">{`What you will see. ${what}`}</Text>
        <Text textRole="hint">
          Where it shows. Over movies and episodes. Hidden during commercials, bumpers and station IDs.
        </Text>
        <Text textRole="hint">
          When. From the next programme on this channel. What is airing now does not change.
        </Text>
        {showAs ? (
          <Text textRole="hint">
            {`Holidays. ${holSentence} ${active ? `Today (${todayLabel}) that is the ${active.name} one.` : `Today (${todayLabel}) it shows the everyday logo.`}`}
          </Text>
        ) : null}
      </Surface>
      {drawable ? null : (
        <Surface
          backgroundColor="$stateWarningSurface"
          borderColor="$stateWarning"
          borderRadius="$control"
          borderWidth={1}
          padding={12}
          role="alert"
        >
          <Text textRole="hint">
            This server can't draw the logo right now. It is encoding without a graphics card overlay, so
            programmes air without a logo. Your choices are saved and will show once it can.
          </Text>
        </Surface>
      )}
    </View>
  );

  return (
    <Surface aria-label="On-screen logo" backgroundColor="$surfaceRaised" gap={20} padding={20} role="region">
      <View style={{ gap: 4 }}>
        <Text textRole="title">On-screen logo</Text>
        <Text textRole="hint" tone="muted">
          The small logo in the corner while a movie or episode plays. Pick how it looks and where it sits.
          Every choice is for this channel only.
        </Text>
      </View>
      {phone ? (
        <View style={{ gap: 24 }}>
          {preview}
          {fields}
        </View>
      ) : (
        <View style={{ alignItems: "flex-start", flexDirection: "row", gap: 32 }}>
          {fields}
          {preview}
        </View>
      )}
    </Surface>
  );
};

export type {
  LogoColor,
  LogoCorner,
  LogoLook,
  OnScreenLogoHoliday,
  OnScreenLogoKey,
  OnScreenLogoPolicy,
  OnScreenLogoPreviewRequest,
  OnScreenLogoSectionProps,
};
export { OnScreenLogoSection };
