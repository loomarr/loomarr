import { LogoScene, Surface, Text } from "@loomarr/design-system";
import { type ReactNode, useEffect, useState } from "react";
import { Image, Pressable } from "react-native";
import { SCENES, type Scene } from "../choices";
import type { LogoCorner } from "../on-screen-logo.type";

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
      borderColor="borderControl"
      borderRadius="control"
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

export { Consequence, Frame, MotifIcon, SmallButton };
