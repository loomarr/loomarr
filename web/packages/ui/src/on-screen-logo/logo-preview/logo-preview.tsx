import { Surface, Text, Toggle } from "@loomarr/design-system";
import { View } from "react-native";
import { labelOf, OPACITIES, SCENES } from "../choices";
import type { OnScreenLogoSectionProps } from "../on-screen-logo.type";
import { Pick as Cards } from "../pick";
import { Consequence, Frame } from "../preview-parts";
import { row } from "../styles";
import type { LogoView } from "../use-logo-view";

type LogoPreviewProps = Pick<OnScreenLogoSectionProps, "drawable" | "holidays"> & { view: LogoView };

// The aired PNG laid on three frames, the "show as" holiday switch, the plain-language
// consequences, and the warning when this server cannot draw the logo.
const LogoPreview = ({ drawable, holidays, view }: LogoPreviewProps) => {
  const {
    active,
    corner,
    enabled,
    hol,
    holShown,
    holSentence,
    img,
    look,
    margin,
    opacity,
    phone,
    scene,
    showAs,
    src,
    todayLabel,
    zoom,
  } = view;
  const what = enabled
    ? `${img ? "Your image" : holShown ? `${look.label} with a ${holShown.name} ${(holShown.pictures.find((m) => m.id === holShown.picture)?.name ?? "").toLowerCase()}` : look.label}, ${corner.replace("-", " ")}, ${labelOf(OPACITIES, opacity).toLowerCase().replace(" — ", " (")}).`
    : "No logo on this channel.";
  const frame = { corner, enabled, margin: Number(margin), opacity: Number(opacity), src, zoom };
  return (
    <View style={{ flex: phone ? undefined : 1.3, gap: 10, minWidth: 0 }} testID="logo-preview">
      <Cards label="Preview picture" onPick={view.setScene} options={SCENES} value={scene} />
      <View style={{ ...row, gap: 6 }}>
        <Toggle
          checked={zoom}
          kind="checkbox"
          label="Zoom the logo ×2 to inspect it (real size is the Size setting)"
          labelHidden
          onCheckedChange={view.setZoom}
        />
        {/* The mock's .zoom label: 12 px muted, and clicking it toggles the box. */}
        <View style={{ flex: 1 }}>
          <Text onPress={() => view.setZoom(!zoom)} textRole="hint" tone="muted">
            Zoom the logo ×2 to inspect it (real size is the Size setting)
          </Text>
        </View>
      </View>
      {showAs ? (
        <View>
          <Cards
            label="Show the logo as"
            lead={
              <Text textRole="hint" tone="muted">
                Show as
              </Text>
            }
            onPick={(v) => view.setView(v === "none" ? null : v)}
            options={[
              { value: "none", label: "Everyday" },
              ...holidays.map((h) => ({ value: h.id, label: h.name })),
            ]}
            value={hol ?? "none"}
          />
        </View>
      ) : null}
      <Frame {...frame} scene={scene} />
      <View style={{ ...row, gap: 10 }}>
        {SCENES.filter((s) => s.value !== scene).map((s) => (
          <View key={s.value} style={{ flex: 1 }}>
            <Frame {...frame} scene={s.value} />
          </View>
        ))}
      </View>
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
          backgroundColor="stateWarningSurface"
          borderColor="stateWarning"
          borderRadius="control"
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
};

export { LogoPreview };
