import { Text as TamaguiText, useTheme, View } from "@tamagui/core";
import Svg, { ClipPath, Defs, G, Rect } from "react-native-svg";

import { brandChroma, brandContract, semanticColors, typography } from "../tokens";

type BrandTone = "fullColor" | "monochrome" | "inverse";
type BrandSize = "small" | "medium" | "large";
const segmentNames = ["signal", "caution", "lock", "tune", "suggest", "onair", "static"] as const;

type BrandMarkProps = {
  accessibilityLabel?: string;
  contained?: boolean;
  decorative?: boolean;
  size?: number;
  tone?: BrandTone;
  width?: number;
};

const BrandMark = ({
  accessibilityLabel = "Loomarr",
  contained = true,
  decorative = false,
  size = 48,
  tone = "fullColor",
  width,
}: BrandMarkProps) => {
  const theme = useTheme();
  const foreground = tone === "inverse" ? semanticColors.brand.ground : theme.contentPrimary.val;
  const segments = tone === "fullColor" ? brandChroma : brandChroma.map(() => foreground);

  if (!contained) {
    return (
      <Svg
        aria-hidden={decorative || undefined}
        aria-label={decorative ? undefined : accessibilityLabel}
        height={size}
        role={decorative ? undefined : "img"}
        viewBox="0 0 98 7"
        width={width ?? size * 14}
      >
        {segments.map((color, index) => (
          <Rect
            fill={color}
            height="7"
            key={segmentNames[index]}
            width={tone === "fullColor" ? 14 : 13.2}
            x={index * 14}
            y="0"
          />
        ))}
      </Svg>
    );
  }

  return (
    <Svg
      aria-hidden={decorative || undefined}
      aria-label={decorative ? undefined : accessibilityLabel}
      height={size}
      role={decorative ? undefined : "img"}
      viewBox="0 0 32 32"
      width={size}
    >
      <Defs>
        <ClipPath id="loomarr-mark-card">
          <Rect height="28" rx="6.5" width="28" x="2" y="2" />
        </ClipPath>
      </Defs>
      <Rect fill={semanticColors.brand.ground} height="28" rx="6.5" width="28" x="2" y="2" />
      <G clipPath="url(#loomarr-mark-card)">
        {segments.map((color, index) => (
          <Rect fill={color} height="28" key={segmentNames[index]} width="4" x={2 + index * 4} y="2" />
        ))}
      </G>
      <Rect
        fill="none"
        height="27"
        rx="6"
        stroke={brandContract.outline}
        strokeWidth="1"
        width="27"
        x="2.5"
        y="2.5"
      />
    </Svg>
  );
};

type BrandWordmarkProps = {
  size?: BrandSize;
  tone?: Exclude<BrandTone, "fullColor">;
};

const wordmarkSizes = {
  small: { fontSize: 14, lineHeight: 18 },
  medium: { fontSize: 24, lineHeight: 29 },
  large: { fontSize: 32, lineHeight: 38 },
} as const;

const BrandWordmark = ({ size = "medium", tone = "monochrome" }: BrandWordmarkProps) => {
  const scale = wordmarkSizes[size];
  return (
    <TamaguiText
      color={tone === "inverse" ? semanticColors.brand.ground : "$contentPrimary"}
      fontFamily={typography.family.body.web}
      fontSize={scale.fontSize}
      fontWeight={brandContract.wordmark.weight as 700}
      letterSpacing={scale.fontSize * brandContract.wordmark.trackingEm}
      lineHeight={scale.lineHeight}
    >
      LOOMARR
    </TamaguiText>
  );
};

type BrandLockupProps = {
  orientation?: "horizontal" | "stacked";
  showTagline?: boolean;
  size?: BrandSize;
  tone?: BrandTone;
};

const markHeights = { small: 6, medium: 10, large: 14 } as const;
const horizontalMarkWidths = { small: 56, medium: 84, large: 112 } as const;
const stackedMarkWidths = { small: 84, medium: 140, large: 200 } as const;
const lockupGaps = { small: 8, medium: 12, large: 12 } as const;

const BrandLockup = ({
  orientation = "horizontal",
  showTagline = false,
  size = "medium",
  tone = "fullColor",
}: BrandLockupProps) => {
  const inverse = tone === "inverse";
  return (
    <View
      alignItems="center"
      flexDirection={orientation === "stacked" ? "column" : "row"}
      gap={lockupGaps[size]}
    >
      <BrandMark
        contained={false}
        decorative
        size={markHeights[size]}
        tone={tone}
        width={orientation === "stacked" ? stackedMarkWidths[size] : horizontalMarkWidths[size]}
      />
      <View alignItems={orientation === "stacked" ? "center" : "flex-start"} gap={2}>
        <BrandWordmark size={size} tone={inverse ? "inverse" : "monochrome"} />
        {showTagline ? (
          <TamaguiText
            color={inverse ? semanticColors.brand.ground : "$contentSecondary"}
            fontFamily={typography.family.data.web}
            fontSize={size === "large" ? 14 : size === "medium" ? 13 : 11}
          >
            always something on
          </TamaguiText>
        ) : null}
      </View>
    </View>
  );
};

export type { BrandLockupProps, BrandMarkProps, BrandSize, BrandTone, BrandWordmarkProps };
export { BrandLockup, BrandMark, BrandWordmark, brandChroma };
