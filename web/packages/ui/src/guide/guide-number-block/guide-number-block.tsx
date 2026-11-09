import { type Density, decadeFill, Surface, Text } from "@loomarr/design-system";

interface GuideNumberBlockProps {
  /** The channel's era decade from the API (1980 for the 1980s); absent is neutral. */
  decade: number | undefined;
  density?: Density;
  number: number;
  textRole: "cardChannelNumber" | "guideNumber";
  /** The number's tone without a decade. */
  tone?: "primary" | "secondary";
}

// A guide row's channel number on its block (#2767): the channel's decade colour, the one place the
// guide shows it. Without a decade the block is clear and the number reads as it always has; the
// box stays, so every row's name sits at the same offset.
const GuideNumberBlock = ({ decade, density, number, textRole, tone }: GuideNumberBlockProps) => {
  const fill = decadeFill(decade);
  return (
    <Surface
      alignItems="center"
      backgroundColor={fill ?? "transparent"}
      borderRadius={2}
      borderWidth={0}
      flexShrink={0}
      justifyContent="center"
      paddingHorizontal={textRole === "guideNumber" ? 6 : 4}
    >
      <Text
        textRole={textRole}
        {...(density ? { density } : {})}
        {...(fill ? { tone: "onDecade" as const } : tone ? { tone } : {})}
      >
        {String(number)}
      </Text>
    </Surface>
  );
};

export { GuideNumberBlock };
