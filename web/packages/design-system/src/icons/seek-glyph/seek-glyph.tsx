import { isWeb } from "@tamagui/core";
import type { LucideIcon, LucideProps } from "lucide-react-native";
import { Text as SvgText } from "react-native-svg";

import { typography } from "../../tokens";

// The numeral's face: the data font at its heaviest weight both platforms embed. Native has one
// static file per weight, named by the weight; the web's variable font takes the weight as a value.
const face = isWeb
  ? { fontFamily: typography.family.data.web, fontWeight: "600" as const }
  : { fontFamily: typography.family.data.native.weights["600"] };

// The player's ⟲/⟳ (§9.4): the rotate arrow with the step's seconds drawn inside it, as the mock
// draws them. The numeral is part of the glyph, not text: it scales, colours and hides with it.
const withSeconds = (Arrow: LucideIcon, seconds: string): LucideIcon => {
  const Glyph = ({ color, ...props }: LucideProps) => (
    <Arrow color={color} {...props}>
      <SvgText {...face} fill={color} fontSize={9.25} stroke="none" textAnchor="middle" x={12} y={15.3}>
        {seconds}
      </SvgText>
    </Arrow>
  );
  return Glyph as LucideIcon;
};

export { withSeconds };
