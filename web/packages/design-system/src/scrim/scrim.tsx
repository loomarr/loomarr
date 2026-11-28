import { useId, useState } from "react";
import { View } from "react-native";
import Svg, { Defs, LinearGradient, Mask, Rect, Stop } from "react-native-svg";

import { playerChrome } from "../tokens";
import type { ScrimProps } from "./scrim.type";

// The native adapter: the web's gradient drawn with SVG. The feather is a length, not a share, so the
// gradient's ends are placed from the measured box.
const Scrim = ({ feather, from, soften = 0, strength = 0.95, testID }: ScrimProps) => {
  const [size, setSize] = useState<{ height: number; width: number } | undefined>(undefined);
  const id = useId().replace(/[^a-zA-Z0-9]/g, "");
  const ink = `ink${id}`;
  const fade = `fade${id}`;
  const mask = `mask${id}`;

  const ends =
    from === "right"
      ? { x1: (size?.width ?? 0) - feather, x2: size?.width ?? 0, y1: 0, y2: 0 }
      : { x1: 0, x2: 0, y1: feather, y2: 0 };

  return (
    <View
      aria-hidden
      onLayout={(event) => setSize(event.nativeEvent.layout)}
      pointerEvents="none"
      style={{ bottom: -soften, left: 0, position: "absolute", right: 0, top: -soften }}
      testID={testID}
    >
      {size ? (
        <Svg height={size.height} width={size.width}>
          <Defs>
            <LinearGradient gradientUnits="userSpaceOnUse" id={ink} {...ends}>
              <Stop offset={0} stopColor={playerChrome.surface} stopOpacity={strength} />
              <Stop offset={1} stopColor={playerChrome.surface} stopOpacity={0} />
            </LinearGradient>
            {soften > 0 ? (
              <>
                <LinearGradient
                  gradientUnits="userSpaceOnUse"
                  id={fade}
                  x1={0}
                  x2={0}
                  y1={0}
                  y2={size.height}
                >
                  <Stop offset={0} stopColor="white" stopOpacity={0} />
                  <Stop offset={soften / size.height} stopColor="white" stopOpacity={1} />
                  <Stop offset={1 - soften / size.height} stopColor="white" stopOpacity={1} />
                  <Stop offset={1} stopColor="white" stopOpacity={0} />
                </LinearGradient>
                <Mask
                  height={size.height}
                  id={mask}
                  maskUnits="userSpaceOnUse"
                  width={size.width}
                  x={0}
                  y={0}
                >
                  <Rect fill={`url(#${fade})`} height={size.height} width={size.width} />
                </Mask>
              </>
            ) : null}
          </Defs>
          <Rect
            fill={`url(#${ink})`}
            height={size.height}
            width={size.width}
            {...(soften > 0 ? { mask: `url(#${mask})` } : {})}
          />
        </Svg>
      ) : null}
    </View>
  );
};

export { Scrim };
