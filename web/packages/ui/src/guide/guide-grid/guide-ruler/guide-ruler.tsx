import { formatGuideTime, type GuideLayout } from "@loomarr/core/guide";
import { brandChroma, Surface, Text } from "@loomarr/design-system";
import { type LayoutChangeEvent, View } from "react-native";

import { nowLine } from "../guide-row";

const RULER = 30;
const hourMs = 3_600_000;
// A ruler label ("12:00 PM" plus its inset) needs this much room. A long span labels every second,
// third… hour instead of letting the labels run into each other; every hour keeps its line.
const LABEL_MIN_PX = 80;
const LABEL_STRIDES = [1, 2, 3, 4, 6] as const;

const labelStride = (timelineWidth: number, span: number): number => {
  const hourPx = (timelineWidth * hourMs) / span;
  if (hourPx <= 0) return 1;
  return LABEL_STRIDES.find((stride) => stride * hourPx >= LABEL_MIN_PX) ?? 6;
};

// The now label sits centred-left on the now line (translateX NOW_LABEL_SHIFT) and an hour's label
// starts just right of its tick. Both widths are generous estimates of a mono "12:00 PM" box, so
// a label is hidden slightly early rather than drawn over the other (#3410). The tick mark stays.
const NOW_LABEL_SHIFT = 20;
const NOW_LABEL_PX = 64;
const TICK_LABEL_INSET = 8;
const TICK_LABEL_PX = 64;

const tickLabelHitsNow = (tickPx: number, nowPx: number): boolean => {
  const labelLeft = tickPx + TICK_LABEL_INSET;
  const nowLeft = nowPx - NOW_LABEL_SHIFT;
  return labelLeft < nowLeft + NOW_LABEL_PX && labelLeft + TICK_LABEL_PX > nowLeft;
};

type GuideRulerProps = {
  layout: GuideLayout;
  nowMs: number;
  /** Where now falls in the window, 0-1; undefined when it is outside it. */
  nowRatio?: number;
  railWidth: number;
  /** The window's length in ms. */
  span: number;
  /** The timeline's measured width; 0 before the first layout. */
  timelineWidth: number;
  windowWidth: number;
  onTimelineWidth: (width: number) => void;
};

// The hour scale over the timeline: a tick per hour, labelled at a stride that keeps the labels
// apart, and the now label on the now line. It measures the timeline for the rows beneath it.
const GuideRuler = ({
  layout,
  nowMs,
  nowRatio,
  onTimelineWidth,
  railWidth,
  span,
  timelineWidth,
  windowWidth,
}: GuideRulerProps) => {
  const firstHour = Math.ceil(layout.fromMs / hourMs) * hourMs;
  const ticks: number[] = [];
  for (let t = firstHour; t < layout.toMs; t += hourMs) ticks.push(t);
  const stride = labelStride(timelineWidth, span);
  // Before the first layout the timeline's width is unknown; the window less the rail stands in.
  const rulerPx = timelineWidth || Math.max(0, windowWidth - railWidth);
  const nowPx = nowRatio === undefined ? undefined : nowRatio * rulerPx;

  return (
    <Surface
      backgroundColor="surfaceCanvas"
      borderBottomColor="borderDecorative"
      borderBottomWidth={1}
      borderWidth={0}
      flexDirection="row"
    >
      <Surface
        backgroundColor="surfaceCanvas"
        borderColor="borderDecorative"
        borderRightWidth={1}
        borderWidth={0}
        width={railWidth}
      />
      <View
        onLayout={(e: LayoutChangeEvent) => onTimelineWidth(e.nativeEvent.layout.width)}
        style={{ flex: 1, height: RULER, overflow: "hidden", position: "relative" }}
      >
        {ticks.map((t, i) => (
          <Surface
            backgroundColor="transparent"
            borderColor="borderDecorative"
            borderLeftWidth={1}
            borderWidth={0}
            bottom={0}
            justifyContent="center"
            left={`${((t - layout.fromMs) / span) * 100}%`}
            paddingLeft={7}
            position="absolute"
            top={0}
            key={t}
          >
            {i % stride === 0 &&
            !(nowPx !== undefined && tickLabelHitsNow(((t - layout.fromMs) / span) * rulerPx, nowPx)) ? (
              <Text numberOfLines={1} textRole="mono">
                {formatGuideTime(t, layout.timezone)}
              </Text>
            ) : null}
          </Surface>
        ))}
        {nowRatio === undefined ? null : (
          <>
            {nowLine(nowRatio)}
            <View
              style={{
                backgroundColor: brandChroma[5],
                borderRadius: 3,
                left: `${nowRatio * 100}%`,
                paddingHorizontal: 5,
                paddingVertical: 1,
                position: "absolute",
                top: 4,
                transform: [{ translateX: -20 }],
              }}
            >
              {/* The brand chroma's ink in both themes, as on a decade fill: the light theme's
                  inverse (white) is 3.9:1 on this red. */}
              <Text textRole="mono" tone="onDecade">
                {formatGuideTime(nowMs, layout.timezone)}
              </Text>
            </View>
          </>
        )}
      </View>
    </Surface>
  );
};

export { GuideRuler, RULER };
