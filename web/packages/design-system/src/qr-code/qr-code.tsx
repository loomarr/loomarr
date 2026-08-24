import { View } from "@tamagui/core";
import QRCodeEncoder from "qrcode";
import { useMemo } from "react";
import Svg, { Path, Rect } from "react-native-svg";

import { semanticColors } from "../tokens";

type QrCodeProps = {
  accessibilityLabel?: string;
  size?: number;
  value: string;
};

const matrixPath = (value: string) => {
  const matrix = QRCodeEncoder.create(value, { errorCorrectionLevel: "M" }).modules;
  const commands: string[] = [];

  for (let row = 0; row < matrix.size; row += 1) {
    let column = 0;
    while (column < matrix.size) {
      while (column < matrix.size && matrix.get(row, column) === 0) column += 1;
      const start = column;
      while (column < matrix.size && matrix.get(row, column) !== 0) column += 1;
      if (column > start) commands.push(`M${start + 4} ${row + 4}h${column - start}v1H${start + 4}z`);
    }
  }

  return { commands: commands.join(""), dimension: matrix.size + 8 };
};

/**
 * Loomarr's machine-readable pairing mark. The fixed high-contrast colors and quiet zone are part
 * of the scanning contract, so product surfaces choose only the payload and logical size.
 */
const QrCode = ({
  accessibilityLabel = "Pair Loomarr with this QR code",
  size = 180,
  value,
}: QrCodeProps) => {
  const path = useMemo(() => matrixPath(value), [value]);

  return (
    <View>
      <Svg
        aria-label={accessibilityLabel}
        height={size}
        role="img"
        viewBox={`0 0 ${path.dimension} ${path.dimension}`}
        width={size}
      >
        <Rect fill={semanticColors.brand.foreground} height="100%" width="100%" />
        <Path d={path.commands} fill={semanticColors.brand.ground} />
      </Svg>
    </View>
  );
};

export type { QrCodeProps };
export { QrCode };
