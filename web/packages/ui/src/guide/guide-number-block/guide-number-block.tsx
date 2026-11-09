import { decadeFill, Surface, Text } from "@loomarr/design-system";

interface GuideNumberBlockProps {
  /** The channel's era decade from the API (1980 for the 1980s); absent is neutral. */
  decade: number | undefined;
  number: number;
}

// A phone Now/Next row's channel number on its block (#2767): the channel's decade colour, the one
// place the row shows it (the grid's rail uses the mock's 4px bar instead). Without a decade the
// block is clear and the number reads as it always has, in the row's secondary tone; the box stays,
// so every row's name sits at the same offset.
const GuideNumberBlock = ({ decade, number }: GuideNumberBlockProps) => {
  const fill = decadeFill(decade);
  return (
    <Surface
      alignItems="center"
      backgroundColor={fill ?? "transparent"}
      borderRadius={2}
      borderWidth={0}
      flexShrink={0}
      justifyContent="center"
      paddingHorizontal={4}
    >
      <Text density="touch" textRole="cardChannelNumber" tone={fill ? "onDecade" : "secondary"}>
        {String(number)}
      </Text>
    </Surface>
  );
};

export { GuideNumberBlock };
