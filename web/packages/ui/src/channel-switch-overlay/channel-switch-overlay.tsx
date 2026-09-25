import { Surface } from "@loomarr/design-system";
import { useEffect, useRef, useState } from "react";
import { Animated, Image } from "react-native";
import { SurfChannelCard } from "../surf-rail/surf-channel-card";
import type { ChannelSwitchOverlayProps } from "./channel-switch-overlay.type";

const FADE_MS = 200;
/** The still is a backdrop for the card, not the picture: dimmed so the card stays the focus. */
const STILL_OPACITY = 0.45;
/** The surf rail's focused card width (420 rail − its 48 left inset − its right padding), kept so the card is the same picture. */
const CARD_WIDTH = 360;
const CARD_LEFT_INSET = 48;

/**
 * Covers the screen the instant a channel switch begins: the channel's latest still (dimmed) with the
 * TV surf card over it, then fades out once the first frame decodes. Renders only from data the
 * client already holds, so it needs no request at the switch itself.
 */
const ChannelSwitchOverlay = ({
  bottomInset = 48,
  channel,
  stillUri,
  visible,
}: ChannelSwitchOverlayProps) => {
  const [mounted, setMounted] = useState(visible);
  const [failedUri, setFailedUri] = useState<string>();
  const opacity = useRef(new Animated.Value(visible ? 1 : 0)).current;

  useEffect(() => {
    if (visible) {
      setMounted(true);
      opacity.setValue(1);
      return undefined;
    }
    Animated.timing(opacity, { duration: FADE_MS, toValue: 0, useNativeDriver: true }).start();
    const timeout = setTimeout(() => setMounted(false), FADE_MS);
    return () => clearTimeout(timeout);
  }, [opacity, visible]);

  if (!mounted && !visible) return null;

  return (
    <Animated.View
      accessibilityLabel="Channel switch"
      pointerEvents="none"
      style={{ backgroundColor: "#000", bottom: 0, left: 0, opacity, position: "absolute", right: 0, top: 0 }}
    >
      {stillUri && failedUri !== stillUri ? (
        <Image
          onError={() => setFailedUri(stillUri)}
          resizeMode="cover"
          source={{ uri: stillUri }}
          style={{ bottom: 0, left: 0, opacity: STILL_OPACITY, position: "absolute", right: 0, top: 0 }}
        />
      ) : null}
      <Surface
        backgroundColor="$transparent"
        borderWidth={0}
        bottom={bottomInset}
        left={CARD_LEFT_INSET}
        position="absolute"
        width={CARD_WIDTH}
      >
        <SurfChannelCard channel={channel} current selected />
      </Surface>
    </Animated.View>
  );
};

export { ChannelSwitchOverlay };
