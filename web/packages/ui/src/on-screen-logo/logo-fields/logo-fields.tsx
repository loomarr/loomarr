import {
  Field,
  FieldHint,
  nativeFontFace,
  RadioGroup,
  RadioItem,
  SelectControl,
  Surface,
  Text,
  Toggle,
  typography,
} from "@loomarr/design-system";
import { Image, Platform, View } from "react-native";
import { COLORS, CORNERS, LOOKS, MARGINS, OPACITIES, SIZES } from "../choices";
import { Row } from "../field-row";
import { HolidayTouches } from "../holiday-touches";
import type { LogoColor, LogoCorner, OnScreenLogoSectionProps } from "../on-screen-logo.type";
import { Pick } from "../pick";
import { SmallButton } from "../preview-parts";
import { group, row, thumb } from "../styles";
import type { LogoView } from "../use-logo-view";

type LogoFieldsProps = Omit<OnScreenLogoSectionProps, "drawable" | "today" | "defaults"> & { view: LogoView };

// The section's controls: one Row per policy field, each with its Reset to default.
const LogoFields = ({
  autoCallsign,
  channelNumber,
  eraLabel,
  everyHoliday,
  holidays,
  imageSrc,
  onChange,
  onOpenProgramming,
  onRemoveImage,
  onUploadImage,
  policy: p,
  previewSrc,
  view,
}: LogoFieldsProps) => {
  const {
    callsign,
    colorOn,
    corner,
    enabled,
    img,
    look,
    lookValue,
    margin,
    open,
    opacity,
    phone,
    reset,
    size,
  } = view;
  return (
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
        onReset={reset("look")}
        overridden={p.look !== undefined}
      >
        <Pick
          columns={phone ? 2 : 3}
          disabled={!enabled || img}
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
        <RadioGroup
          label="Colour"
          onValueChange={(v) => onChange({ color: v as LogoColor })}
          value={p.color ?? "era"}
          values={COLORS.map((c) => c.value)}
          {...group}
        >
          {COLORS.map((c) => (
            <RadioItem disabled={!colorOn} key={c.value} label={c.label} ringRadius={16} value={c.value}>
              {({ checked }) => (
                <Surface
                  backgroundColor={c.swatch}
                  borderColor={checked ? "contentPrimary" : "borderControl"}
                  borderRadius="round"
                  borderWidth={checked ? 2 : 1}
                  height={32}
                  width={32}
                />
              )}
            </RadioItem>
          ))}
        </RadioGroup>
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
            onChangeText={view.setCallsign}
            placeholder={autoCallsign ? `Auto: ${autoCallsign}` : "Auto"}
            // The mock's .input is the body face; Field defaults to the data (mono) face.
            style={{
              fontFamily:
                Platform.OS === "web"
                  ? typography.family.body.web
                  : nativeFontFace("body", Platform.OS === "android" ? "android" : "ios"),
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
        onReset={view.ignoresHolidays ? undefined : reset("holiday")}
        overridden={p.holiday !== undefined}
      >
        <HolidayTouches
          everyHoliday={everyHoliday}
          holidays={holidays}
          onChange={onChange}
          onOpenProgramming={onOpenProgramming}
          policy={p}
          previewSrc={previewSrc}
          view={view}
        />
      </Row>
      <Row
        field="corner"
        help="Where the logo sits on the picture."
        hint="The logo sits inside the picture, so a letterboxed film keeps it off the black bars."
        label="Position"
        onReset={reset("corner")}
        overridden={p.corner !== undefined}
      >
        <RadioGroup
          label="Position"
          onValueChange={(v) => onChange({ corner: v as LogoCorner })}
          value={corner}
          values={CORNERS.map((c) => c.value)}
          {...group}
          gap={6}
          width={118}
        >
          {CORNERS.map((c) => (
            <RadioItem key={c.value} label={c.label} value={c.value}>
              {({ checked }) => (
                // The mock's .cell: 56 × 40, the chosen one tinted with a 14 px foreground marker inside.
                <Surface
                  alignItems="center"
                  backgroundColor={checked ? "surfaceFocus" : "transparent"}
                  borderColor={checked ? "contentPrimary" : "borderControl"}
                  borderRadius="control"
                  borderWidth={checked ? 2 : 1}
                  height={40}
                  justifyContent="center"
                  width={56}
                >
                  {checked ? (
                    <Surface backgroundColor="contentPrimary" borderRadius={3} height={14} width={14} />
                  ) : null}
                </Surface>
              )}
            </RadioItem>
          ))}
        </RadioGroup>
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
          onOpenChange={(o) => view.setOpen(o ? "size" : null)}
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
          onOpenChange={(o) => view.setOpen(o ? "margin" : null)}
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
          onOpenChange={(o) => view.setOpen(o ? "opacity" : null)}
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
};

export { LogoFields };
