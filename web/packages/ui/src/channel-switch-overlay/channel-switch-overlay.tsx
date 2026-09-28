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
 * It stays mounted between switches, hidden, so a key only flips its opacity (#1781). Mounting the
 * readout at the key (the snow alone is ~600 native views) held its first paint 312 ms on a Shield.
 * The shown opacity is a plain prop committed with the key's own render, never a value an effect or
 * the native animation driver sets afterwards. The fade-out runs on an inner layer, and the next
 * switch's starting values are set once the overlay is hidden, so every switch opens on the same frame.
 *
 * Reduced motion: no drain, still snow, locked bars, a steady cursor.
 */
const ChannelSwitchOverlay = ({
  channel,
  onShown,
  reducedMotion,
  stillUri,
  visible,
}: ChannelSwitchOverlayProps) => {
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const still = prefersReducedMotion !== false;
  // Derived in render, so the frame that hides the readout already fades rather than popping out.
  const [wasVisible, setWasVisible] = useState(visible);
  const [fading, setFading] = useState(false);
  if (visible !== wasVisible) {
    setWasVisible(visible);
    setFading(!visible);
  }
  const shown = visible || fading;
  const [failedUri, setFailedUri] = useState<string>();
  const fade = useRef(new Animated.Value(1)).current;
  const drain = useRef(new Animated.Value(still ? DRAIN.end.opacity : 1)).current;
  const wash = useRef(new Animated.Value(still ? 1 : THICKEN.from)).current;
  const cursor = useRef(new Animated.Value(1)).current;
  const onShownRef = useRef(onShown);

  useEffect(() => {
    onShownRef.current = onShown;
  }, [onShown]);

  // Once per switch, including a second switch made while the overlay is still up.
  // biome-ignore lint/correctness/useExhaustiveDependencies: a new channel number IS a new switch
  useEffect(() => {
    if (visible) onShownRef.current?.("osd");
  }, [channel.channelNumber, visible]);

  // Once hidden, set the next switch's opening frame: opaque, the still undrained, the snow thin.
  useEffect(() => {
    if (shown) return;
    fade.setValue(1);
    drain.setValue(still ? DRAIN.end.opacity : 1);
    wash.setValue(still ? 1 : THICKEN.from);
  }, [drain, fade, shown, still, wash]);

  useEffect(() => {
    if (visible) {
      // Already the case unless the key came mid-fade; then this lifts the readout back up.
      fade.setValue(1);
      drain.setValue(still ? DRAIN.end.opacity : 1);
      wash.setValue(still ? 1 : THICKEN.from);
      if (still) return undefined;
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
    if (!fading) return undefined;
    const fadeOut = Animated.timing(fade, { duration: FADE_MS, toValue: 0, useNativeDriver: true });
    fadeOut.start();
    const timeout = setTimeout(() => setFading(false), FADE_MS);
    return () => {
      fadeOut.stop();
      clearTimeout(timeout);
    };
  }, [drain, fade, fading, still, visible, wash]);

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

  // Hidden, nothing in it animates: the snow's flicker and the bars hold still until the next key.
  const motion = shown ? reducedMotion : true;

  return (
    <View
      accessibilityElementsHidden={!shown}
      accessibilityLabel={
        shown ? `Tuning in to channel ${channel.channelNumber}, ${channel.channelName}` : undefined
      }
      aria-hidden={!shown}
      importantForAccessibility={shown ? "auto" : "no-hide-descendants"}
      pointerEvents="none"
      style={[fill, { opacity: shown ? 1 : 0 }]}
    >
      <Animated.View style={[fill, { backgroundColor: "#0B0C0E", opacity: fade }]}>
        {stillUri && failedUri !== stillUri ? (
          <Animated.View style={[fill, { filter: GREYED, opacity: drain }]}>
            <Image
              onError={() => setFailedUri(stillUri)}
              // The readout stays mounted after a switch, so a still that lands once it is hidden
              // was never held on screen: no mark.
              onLoad={() => {
                if (shown) onShownRef.current?.("still");
              }}
              resizeMode="cover"
              source={{ uri: stillUri }}
              style={fill}
            />
          </Animated.View>
        ) : null}
        <Animated.View style={[fill, { opacity: wash }]}>
          <AnalogSnow reducedMotion={motion} />
        </Animated.View>
        <View
          style={[fill, { alignItems: "center", gap: 12, justifyContent: "center", paddingHorizontal: 48 }]}
        >
          <SignalLoader accessibilityLabel="Tuning in" density="tv" label={null} reducedMotion={motion} />
          {/* The readout's few lines of text render with the key itself; hidden, no chrome text remains. */}
          {shown ? (
            <>
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
            </>
          ) : null}
        </View>
      </Animated.View>
    </View>
  );
};

export { ChannelSwitchOverlay };
