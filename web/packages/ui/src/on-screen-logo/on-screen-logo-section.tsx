import {
  Action,
  Badge,
  brandChroma,
  Field,
  FieldHint,
  LogoScene,
  SelectControl,
  Surface,
  Text,
  Toggle,
  typography,
} from "@loomarr/design-system";
import { type ReactNode, useEffect, useState } from "react";
import { Image, Platform, Pressable, useWindowDimensions, View } from "react-native";

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

const row = { alignItems: "center", flexDirection: "row", gap: 8 } as const;
// A radio group laid out as a wrapping row; Surface carries the role (Text drops it on web).
const thumb = {
  alignItems: "center",
  borderRadius: 4,
  height: 44,
  justifyContent: "center",
  overflow: "hidden",
  // The mock's #2a3140 → #14171d scene backdrop; the bugs are drawn to be read on a dark frame.
  ...(Platform.OS === "web"
    ? ({ backgroundImage: "linear-gradient(135deg, #2a3140, #14171d)" } as object)
    : { backgroundColor: "#1c212a" }),
} as const;
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
          {/* The mock's .help: an 18 px outlined circle beside the label, so the header stays one row. */}
          <Pressable
            accessibilityLabel={`About ${label}`}
            accessibilityRole="button"
            aria-expanded={showHelp}
            hitSlop={13}
            onPress={() => setShowHelp((v) => !v)}
          >
            <Surface
              alignItems="center"
              backgroundColor="transparent"
              borderColor="$borderControl"
              borderRadius="$round"
              borderWidth={1}
              height={18}
              justifyContent="center"
              width={18}
            >
              <Text textRole="guideMeta" tone="muted">
                ?
              </Text>
            </Surface>
          </Pressable>
        </View>
        {onReset ? (
          <View style={row}>
            <Badge shape="square" tone={overridden ? "signal" : "neutral"}>
              {overridden ? "Channel override" : "Default"}
            </Badge>
            {/* The mock's .link: 12 px underlined text, no button box, so the header stays one row. */}
            <Pressable
              accessibilityLabel={`Reset to default (${label})`}
              accessibilityRole="button"
              disabled={!overridden}
              hitSlop={12}
              onPress={onReset}
              style={{ opacity: overridden ? 1 : 0.5 }}
            >
              <Text textDecorationLine="underline" textRole="hint" tone="primary">
                Reset to default
              </Text>
            </Pressable>
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
  columns?: number;
  label: string;
  /** Shown first in the row, before the cards (the mock's inline "Show as"). */
  lead?: ReactNode;
  onPick: (v: V) => void;
  options: readonly { label: string; value: V; visual?: ReactNode }[];
  value: V | undefined;
}

// A row of selectable cards (style thumbnails, scene buttons, "Show as"): one radio group.
// With `columns`, the cards sit in a fixed grid of equal tiles (the mock's .looks grid).
const Pick = <V extends string>({ columns, label, lead, onPick, options, value }: PickProps<V>) => {
  const card = (o: PickProps<V>["options"][number]) => {
    const checked = o.value === value;
    return (
      <Pressable
        accessibilityLabel={o.label}
        accessibilityRole="radio"
        aria-checked={checked}
        key={o.value}
        onPress={() => onPick(o.value)}
        style={columns ? { flex: 1, minWidth: 0 } : undefined}
      >
        <Surface
          alignItems={columns ? "stretch" : "center"}
          backgroundColor={checked ? "$surfaceFocus" : "$surfaceRaised"}
          borderColor={checked ? "$actionPrimary" : "$borderControl"}
          borderRadius="$control"
          borderWidth={checked ? 2 : 1}
          flexGrow={columns ? 1 : undefined}
          gap={6}
          paddingHorizontal={columns ? 8 : 12}
          paddingVertical={8}
        >
          {columns ? (
            // The mock's .thumb: a dark gradient box that holds the bug, name left-aligned below.
            <View style={thumb}>{o.visual}</View>
          ) : (
            o.visual
          )}
          <Text textRole="cardLabel">{o.label}</Text>
        </Surface>
      </Pressable>
    );
  };
  if (!columns) {
    return (
      <Surface alignItems="center" aria-label={label} role="radiogroup" {...group}>
        {lead}
        {options.map(card)}
      </Surface>
    );
  }
  // Rows of `columns` equal cells; the last row keeps empty cells so its tiles stay the same width.
  const rows = Array.from({ length: Math.ceil(options.length / columns) }, (_, i) =>
    options.slice(i * columns, (i + 1) * columns),
  );
  return (
    <Surface aria-label={label} borderWidth={0} gap={8} role="radiogroup">
      {rows.map((r) => (
        <View key={r[0]?.value} style={{ flexDirection: "row", gap: 8 }}>
          {r.map(card)}
          {Array.from({ length: columns - r.length }, (_, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: an empty cell's only identity is its column.
            <View key={`empty-${i}`} style={{ flex: 1 }} />
          ))}
        </View>
      ))}
    </Surface>
  );
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

// One 16:9 scene with the aired PNG anchored inside the active picture (decision 0036): the PNG
// is drawn for a 1080-line frame, so its own height over 1080 is its share of the picture height,
// and the margin is a share of that height on both axes (the mock's cqh rule), in px.
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
  return (
    <LogoScene
      label={`${SCENES.find((s) => s.value === scene)?.label} frame with the logo ${enabled ? corner.replace("-", " ") : "off"}`}
      scene={scene}
    >
      {(picture) =>
        enabled && dims ? (
          <Image
            accessibilityIgnoresInvertColors
            source={{ uri: src }}
            style={{
              height: share * picture.height,
              opacity,
              position: "absolute",
              width: (share * picture.height * dims.width) / dims.height,
              [v === "top" ? "top" : "bottom"]: margin * picture.height,
              [h === "left" ? "left" : "right"]: margin * picture.height,
            }}
          />
        ) : null
      }
    </LogoScene>
  );
};

// One consequence line under the preview: a bold lead phrase, then the plain sentence.
const Consequence = ({ children, lead }: { children: ReactNode; lead: string }) => (
  <Text textRole="compact" tone="muted">
    <Text textRole="cardLabel" tone="primary">
      {lead}
    </Text>{" "}
    {children}
  </Text>
);

// The mock's .btn.sm.outline: the holiday chips' box with a 13 px label, not a bold Action.
const SmallButton = ({ label, onPress }: { label: string; onPress?: () => void }) => (
  <Pressable accessibilityRole="button" onPress={onPress}>
    <Surface
      alignItems="center"
      backgroundColor="transparent"
      borderColor="$borderControl"
      borderRadius="$control"
      borderWidth={1}
      justifyContent="center"
      minHeight={32}
      paddingHorizontal={10}
    >
      <Text textRole="cardLabel">{label}</Text>
    </Surface>
  </Pressable>
);

// A holiday picture at chip size: the backend's picture-only PNG (the mock's 24 px icon).
const MotifIcon = ({ src }: { src: string }) => (
  <Image
    accessibilityIgnoresInvertColors
    resizeMode="contain"
    source={{ uri: src }}
    style={{ height: 24, width: 24 }}
  />
);

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
  imageSrc,
  onRemoveImage,
  onUploadImage,
  policy: p,
  previewSrc,
}: OnScreenLogoSectionProps) => {
  const { width } = useWindowDimensions();
  const phone = width < 720;
  const [scene, setScene] = useState<Scene>("bright");
  const [zoom, setZoom] = useState(true);
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
    <View style={{ flex: phone ? undefined : 1, gap: 20, minWidth: 0 }}>
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
          columns={phone ? 2 : 3}
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
            // The mock's .input is the body face; Field defaults to the data (mono) face.
            style={{
              fontFamily: Platform.OS === "web" ? typography.family.body.web : typography.family.body.native,
            }}
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
                  // The mock's .hrow: one bordered row per holiday, its pictures as small chips.
                  <Surface
                    alignItems="center"
                    backgroundColor="transparent"
                    borderColor="$borderSubtle"
                    borderRadius="$control"
                    borderWidth={1}
                    columnGap={14}
                    flexDirection="row"
                    flexWrap="wrap"
                    key={h.id}
                    paddingHorizontal={12}
                    paddingVertical={10}
                    rowGap={8}
                  >
                    <View style={{ minWidth: 108 }}>
                      <Text textRole="label">{h.name}</Text>
                    </View>
                    <FieldHint>{`${day(h.start)} – ${day(h.end)}`}</FieldHint>
                    {h.active ? (
                      <Badge shape="square" tone="lock">
                        In effect now
                      </Badge>
                    ) : null}
                    {h.pictures.length > 1 ? (
                      <Surface aria-label={`Picture for ${h.name}`} role="radiogroup" {...group} gap={6}>
                        {h.pictures.map((m) => {
                          const checked = m.id === h.picture;
                          return (
                            <Pressable
                              accessibilityLabel={`${m.name} for ${h.name}`}
                              accessibilityRole="radio"
                              aria-checked={checked}
                              key={m.id}
                              onPress={() => onChange({ motifs: { ...p.motifs, [h.id]: m.id } })}
                            >
                              <Surface
                                alignItems="center"
                                backgroundColor={checked ? "$surfaceFocus" : "transparent"}
                                borderColor={checked ? "$actionPrimary" : "$borderControl"}
                                borderRadius="$control"
                                borderWidth={1}
                                flexDirection="row"
                                gap={6}
                                minHeight={36}
                                paddingLeft={6}
                                paddingRight={10}
                                paddingVertical={4}
                              >
                                <MotifIcon
                                  src={previewSrc({ holiday: h.id, motifOnly: true, picture: m.id })}
                                />
                                <Text textRole="cardLabel">{m.name}</Text>
                              </Surface>
                            </Pressable>
                          );
                        })}
                      </Surface>
                    ) : (
                      <View style={{ ...row, gap: 6 }}>
                        <MotifIcon src={previewSrc({ holiday: h.id, motifOnly: true })} />
                        <Text textRole="cardLabel">{h.pictures[0]?.name ?? ""}</Text>
                      </View>
                    )}
                  </Surface>
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
        <Surface aria-label="Position" role="radiogroup" {...group} gap={6} width={118}>
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
                {/* The mock's .cell: 56 × 40, the chosen one tinted with a 16 px marker inside. */}
                <Surface
                  alignItems="center"
                  backgroundColor={checked ? "$surfaceFocus" : "transparent"}
                  borderColor={checked ? "$actionPrimary" : "$borderControl"}
                  borderRadius="$control"
                  borderWidth={1}
                  height={40}
                  justifyContent="center"
                  width={56}
                >
                  {checked ? (
                    <Surface backgroundColor="$actionPrimary" borderRadius={3} height={14} width={14} />
                  ) : null}
                </Surface>
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
          appearance="field"
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
          appearance="field"
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
          appearance="field"
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
              <View style={{ ...thumb, width: 96 }}>
                {imageSrc ? (
                  <Image
                    accessibilityIgnoresInvertColors
                    accessibilityLabel="Your own image"
                    resizeMode="contain"
                    source={{ uri: imageSrc }}
                    style={{ height: 30, width: 90 }}
                  />
                ) : null}
              </View>
              <View style={{ flexGrow: 1, flexShrink: 1 }}>
                <Text textRole="compact">Your own image</Text>
              </View>
              <SmallButton label="Remove" onPress={onRemoveImage} />
            </>
          ) : (
            <>
              <SmallButton label="Upload an image" onPress={onUploadImage} />
              <FieldHint>PNG or WebP. Transparent background works best.</FieldHint>
            </>
          )}
        </View>
      </Row>
    </View>
  );

  const what = enabled
    ? `${img ? "Your image" : holShown ? `${look.label} with a ${holShown.name} ${(holShown.pictures.find((m) => m.id === holShown.picture)?.name ?? "").toLowerCase()}` : look.label}, ${corner.replace("-", " ")}, ${labelOf(OPACITIES, opacity).toLowerCase().replace(" — ", " (")}).`
    : "No logo on this channel.";
  const preview = (
    <View style={{ flex: phone ? undefined : 1.3, gap: 10, minWidth: 0 }} testID="logo-preview">
      <Pick label="Preview picture" onPick={setScene} options={SCENES} value={scene} />
      <View style={{ ...row, gap: 6 }}>
        <Toggle
          checked={zoom}
          kind="checkbox"
          label="Zoom the logo ×2 to inspect it (real size is the Size setting)"
          labelHidden
          onCheckedChange={setZoom}
        />
        {/* The mock's .zoom label: 12 px muted, and clicking it toggles the box. */}
        <View style={{ flex: 1 }}>
          <Text onPress={() => setZoom(!zoom)} textRole="hint" tone="muted">
            Zoom the logo ×2 to inspect it (real size is the Size setting)
          </Text>
        </View>
      </View>
      {showAs ? (
        <View>
          <Pick
            label="Show the logo as"
            lead={
              <Text textRole="hint" tone="muted">
                Show as
              </Text>
            }
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
            <View style={{ ...row, gap: 10 }}>
              {SCENES.filter((s) => s.value !== scene).map((s) => (
                <View key={s.value} style={{ flex: 1 }}>
                  <Frame {...frame} scene={s.value} />
                </View>
              ))}
            </View>
          </>
        );
      })()}
      {/* The mock's .consq: muted lines, each led by a bold phrase. */}
      <View style={{ gap: 4 }} testID="logo-consequence">
        <Consequence lead="What you will see.">{what}</Consequence>
        <Consequence lead="Where it shows.">
          Over movies and episodes. Hidden during commercials, bumpers and station IDs.
        </Consequence>
        <Consequence lead="When.">
          From the next programme on this channel. What is airing now does not change.
        </Consequence>
        {showAs ? (
          <Consequence lead="Holidays.">
            {`${holSentence} ${active ? `Today (${todayLabel}) that is the ${active.name} one.` : `Today (${todayLabel}) it shows the everyday logo.`}`}
          </Consequence>
        ) : null}
      </View>
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
        // The mock's minmax(0, 1fr) minmax(0, 1.3fr) split with a 28 px gap.
        <View style={{ alignItems: "flex-start", flexDirection: "row", gap: 28 }}>
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
