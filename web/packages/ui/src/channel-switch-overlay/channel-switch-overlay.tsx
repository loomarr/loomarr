import { AnalogSnow, SignalLoader, Text, useReducedMotionPreference } from "@loomarr/design-system";
import { useEffect, useRef, useState } from "react";
import { Animated, Easing, Image, View } from "react-native";
import type { ChannelSwitchOverlayProps } from "./channel-switch-overlay.type";

const FADE_MS = 200;
// The held frame drains as on web (#1620 B4): to 55% by 256 ms and 12% by 448 ms, greyed and
// flattened. Reduced motion skips the drain and rests at the drained frame.
const DRAIN = { mid: { at: 256, opacity: 0.55 }, end: { at: 448, opacity: 0.12 } } as const;
// The snow thickens in over the draining frame, from a quarter to full, in 260 ms.
const THICKEN = { from: 0.25, ms: 260 } as const;
const CURSOR_BLINK_MS = 500;
// Greyed and flattened like the web's backdrop filter. React Native applies grayscale and
// contrast on Android only; elsewhere the still simply drains.
const GREYED = [{ grayscale: 1 }, { contrast: 0.6 }] as const;
const fill = { bottom: 0, left: 0, position: "absolute", right: 0, top: 0 } as const;

/**
 * ChannelSwitchOverlay — the channel-switch readout ("B4", #1627, the web tuner's #1620 look):
 * the outgoing still drains into analog snow while the bars lock, over a centred channel line
 * ("CH 7" tracked amber mono, then the name) and "TUNING IN_". It covers the screen the instant a
 * switch begins and fades once the first frame decodes. It renders only from data the client
 * already holds, so it needs no request at the switch itself.
 *
 * Reduced motion: no drain, still snow, locked bars, a steady cursor.
 */
const ChannelSwitchOverlay = ({ channel, reducedMotion, stillUri, visible }: ChannelSwitchOverlayProps) => {
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const still = prefersReducedMotion !== false;
  const [mounted, setMounted] = useState(visible);
  const [failedUri, setFailedUri] = useState<string>();
  const opacity = useRef(new Animated.Value(visible ? 1 : 0)).current;
  const drain = useRef(new Animated.Value(still ? DRAIN.end.opacity : 1)).current;
  const wash = useRef(new Animated.Value(still ? 1 : THICKEN.from)).current;
  const cursor = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    if (visible) {
      setMounted(true);
      opacity.setValue(1);
      if (still) {
        drain.setValue(DRAIN.end.opacity);
        wash.setValue(1);
        return undefined;
      }
      drain.setValue(1);
      wash.setValue(THICKEN.from);
      const switching = Animated.parallel([
        Animated.sequence([
          Animated.timing(drain, {
            duration: DRAIN.mid.at,
            easing: Easing.in(Easing.quad),
            toValue: DRAIN.mid.opacity,
            useNativeDriver: true,
          }),
          Animated.timing(drain, {
            duration: DRAIN.end.at - DRAIN.mid.at,
            easing: Easing.in(Easing.quad),
            toValue: DRAIN.end.opacity,
            useNativeDriver: true,
          }),
        ]),
        Animated.timing(wash, {
          duration: THICKEN.ms,
          easing: Easing.in(Easing.quad),
          toValue: 1,
          useNativeDriver: true,
        }),
      ]);
      switching.start();
      return () => switching.stop();
    }
    Animated.timing(opacity, { duration: FADE_MS, toValue: 0, useNativeDriver: true }).start();
    const timeout = setTimeout(() => setMounted(false), FADE_MS);
    return () => clearTimeout(timeout);
  }, [drain, opacity, still, visible, wash]);

  useEffect(() => {
    if (still || !visible) {
      cursor.setValue(1);
      return undefined;
    }
    const blink = Animated.loop(
      Animated.sequence([
        Animated.timing(cursor, { duration: 0, toValue: 1, useNativeDriver: true }),
        Animated.delay(CURSOR_BLINK_MS),
        Animated.timing(cursor, { duration: 0, toValue: 0, useNativeDriver: true }),
        Animated.delay(CURSOR_BLINK_MS),
      ]),
    );
    blink.start();
    return () => blink.stop();
  }, [cursor, still, visible]);

  if (!mounted && !visible) return null;

  return (
    <Animated.View
      accessibilityLabel={`Tuning in to channel ${channel.channelNumber}, ${channel.channelName}`}
      pointerEvents="none"
      style={[fill, { backgroundColor: "#0B0C0E", opacity }]}
    >
      {stillUri && failedUri !== stillUri ? (
        <Animated.View style={[fill, { filter: GREYED, opacity: drain }]}>
          <Image
            onError={() => setFailedUri(stillUri)}
            resizeMode="cover"
            source={{ uri: stillUri }}
            style={fill}
          />
        </Animated.View>
      ) : null}
      <Animated.View style={[fill, { opacity: wash }]}>
        <AnalogSnow reducedMotion={reducedMotion} />
      </Animated.View>
      <View
        style={[fill, { alignItems: "center", gap: 12, justifyContent: "center", paddingHorizontal: 48 }]}
      >
        <SignalLoader
          accessibilityLabel="Tuning in"
          density="tv"
          label={null}
          reducedMotion={reducedMotion}
        />
        <View style={{ alignItems: "baseline", flexDirection: "row", gap: 12 }}>
          <Text density="tv" halo textRole="cardChannelNumber" tracking={0.16}>
            CH {channel.channelNumber}
          </Text>
          <Text density="tv" halo numberOfLines={1} textRole="title">
            {channel.channelName}
          </Text>
        </View>
        <View style={{ flexDirection: "row" }}>
          <Text density="tv" halo textRole="metadata" tone="signal" tracking={0.24}>
            TUNING IN
          </Text>
          <Animated.View style={{ opacity: cursor }}>
            <Text density="tv" halo textRole="metadata" tone="signal">
              _
            </Text>
          </Animated.View>
        </View>
      </View>
    </Animated.View>
  );
};

export { ChannelSwitchOverlay };
