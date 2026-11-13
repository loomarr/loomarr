import { Surface, Text } from "@loomarr/design-system";
import { View } from "react-native";
import { LogoFields } from "./logo-fields";
import { LogoPreview } from "./logo-preview";
import type { OnScreenLogoSectionProps } from "./on-screen-logo.type";
import { useLogoView } from "./use-logo-view";

// ON-SCREEN LOGO — the channel page's per-channel logo section (#1659; approved mock
// project/prototypes/watermark.html, "Channel page → Channel info"). Presentational: the app
// owns the policy, the autosave PATCH and the preview URL. The preview is the backend's PNG, the
// exact image that airs, laid on three frames; nothing here redraws the logo.

const OnScreenLogoSection = (props: OnScreenLogoSectionProps) => {
  const { defaults, drawable, holidays, policy, previewSrc, onChange, today, ...fieldProps } = props;
  const view = useLogoView({ defaults, holidays, onChange, policy, previewSrc, today });
  const fields = (
    <LogoFields
      {...fieldProps}
      holidays={holidays}
      onChange={onChange}
      policy={policy}
      previewSrc={previewSrc}
      view={view}
    />
  );
  const preview = <LogoPreview drawable={drawable} holidays={holidays} view={view} />;

  return (
    <Surface aria-label="On-screen logo" backgroundColor="surfaceRaised" gap={20} padding={20} role="region">
      <View style={{ gap: 4 }}>
        <Text textRole="title">On-screen logo</Text>
        <Text textRole="hint" tone="muted">
          The small logo in the corner while a movie or episode plays. Pick how it looks and where it sits.
          Every choice is for this channel only.
        </Text>
      </View>
      {view.phone ? (
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

export { OnScreenLogoSection };
