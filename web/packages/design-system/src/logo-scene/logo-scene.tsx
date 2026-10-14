import { View } from "@tamagui/core";
import { type ReactNode, useState } from "react";
import Svg, { Circle, Defs, LinearGradient, Polygon, RadialGradient, Rect, Stop } from "react-native-svg";

import { brandChroma, brandContract } from "../tokens";

// LOGO SCENE — a stand-in picture behind the On-screen logo preview (#1659; approved mock
// project/prototypes/watermark.html, `.scene`): a bright frame (sun over dunes), a dark one (a room
// with a lit window) and an old 4:3 film letterboxed in a 16:9 frame. Geometry only, no imagery,
// and the same in both themes because it stands in for a programme. The mock's shapes, in a
// 160 × 90 box (1 cqh = 0.9, 1 cqw = 1.6).

type LogoSceneKind = "bright" | "dark" | "box";

/** The picture's colours, from the mock's scene CSS: a programme, so not themed. */
const sceneArt = {
  frame: "#000000",
  edge: brandContract.outline,
  bright: { sky: ["#f4ecd2", "#ffe9a8"], ground: ["#f7c97b", "#d9a45a"], sun: "#fff7d6", halo: "#ffefb8" },
  dunes: ["#b98a46", "#d6a85d", "#c4944e"],
  dark: { glow: ["#1c2a3a", "#0c1118", "#06080b"], window: "#0e1722", frameLine: "#1f3144", lamp: "#ffd27a" },
  box: { sky: ["#7aa5b8", "#cfc7a0"], ground: "#8f7a52", block: "#d94f4f" },
} as const;

const W = 160;
const H = 90;
// The active picture: the full frame, or the 4:3 film between its bars (left/right 12.5%).
const PICTURE = { box: { left: 20, width: 120 }, full: { left: 0, width: W } } as const;

interface LogoSceneProps {
  /** Drawn over the active picture, given its size in px (the bug anchors inside it). */
  children?: (picture: { height: number; width: number }) => ReactNode;
  /** What the frame shows, for a screen reader. */
  label: string;
  scene: LogoSceneKind;
}

const Bright = ({ id }: { id: string }) => {
  const a = sceneArt.bright;
  // The dunes: the bottom 45%, clipped to the mock's polygon.
  const top = H * 0.55;
  const dh = H * 0.45;
  const corners: readonly (readonly [number, number])[] = [
    [0, 0.4],
    [0.25, 0.1],
    [0.5, 0.45],
    [0.75, 0.05],
    [1, 0.35],
    [1, 1],
    [0, 1],
  ];
  const pts = corners.map(([x, y]) => `${x * W},${top + y * dh}`).join(" ");
  return (
    <>
      <Defs>
        <LinearGradient id={`${id}-sky`} x1="0" x2="0" y1="0" y2="1">
          <Stop offset="0" stopColor={a.sky[0]} />
          <Stop offset="0.55" stopColor={a.sky[1]} />
          <Stop offset="0.55" stopColor={a.ground[0]} />
          <Stop offset="1" stopColor={a.ground[1]} />
        </LinearGradient>
        <LinearGradient id={`${id}-dunes`} x1="0" x2="1" y1="0.4" y2="0.6">
          <Stop offset="0.3" stopColor={sceneArt.dunes[0]} />
          <Stop offset="0.3" stopColor={sceneArt.dunes[1]} />
          <Stop offset="0.62" stopColor={sceneArt.dunes[1]} />
          <Stop offset="0.62" stopColor={sceneArt.dunes[2]} />
        </LinearGradient>
      </Defs>
      <Rect fill={`url(#${id}-sky)`} height={H} width={W} />
      {/* The sun: 16% wide at 14%/14%, with a 4 cqh halo. */}
      <Circle cx={W * 0.14 + W * 0.08} cy={H * 0.14 + W * 0.08} fill={a.halo} r={W * 0.08 + 3.6} />
      <Circle cx={W * 0.14 + W * 0.08} cy={H * 0.14 + W * 0.08} fill={a.sun} r={W * 0.08} />
      <Polygon fill={`url(#${id}-dunes)`} points={pts} />
    </>
  );
};

const Dark = ({ id }: { id: string }) => {
  const a = sceneArt.dark;
  // The window: right 18%, bottom 14%, 22% × 34%; its lamp: right 22%, bottom 22%, 6% × 12%.
  const win = { height: H * 0.34, width: W * 0.22, x: W * 0.6, y: H * 0.52 };
  return (
    <>
      <Defs>
        <RadialGradient cx="70%" cy="60%" fx="70%" fy="60%" id={`${id}-room`} rx="99%" ry="85%">
          <Stop offset="0" stopColor={a.glow[0]} />
          <Stop offset="0.6" stopColor={a.glow[1]} />
          <Stop offset="1" stopColor={a.glow[2]} />
        </RadialGradient>
      </Defs>
      <Rect fill={`url(#${id}-room)`} height={H} width={W} />
      {/* The window's faint glow: the mock's rgba(76, 201, 232, .12) shadow, the brand cyan. */}
      <Rect
        fill={brandChroma[3]}
        height={win.height + 8}
        opacity={0.05}
        rx={6}
        width={win.width + 8}
        x={win.x - 4}
        y={win.y - 4}
      />
      <Rect
        fill={a.window}
        height={win.height}
        rx={1.6}
        stroke={a.frameLine}
        strokeWidth={0.5}
        width={win.width}
        x={win.x}
        y={win.y}
      />
      <Rect fill={a.lamp} height={H * 0.12} opacity={0.55} width={W * 0.06} x={W * 0.72} y={H * 0.66} />
    </>
  );
};
const Box = ({ id }: { id: string }) => {
  const a = sceneArt.box;
  const { left, width } = PICTURE.box;
  return (
    <>
      <Defs>
        <LinearGradient id={`${id}-film`} x1="0" x2="0" y1="0" y2="1">
          <Stop offset="0" stopColor={a.sky[0]} />
          <Stop offset="0.62" stopColor={a.sky[1]} />
          <Stop offset="0.62" stopColor={a.ground} />
          <Stop offset="1" stopColor={a.ground} />
        </LinearGradient>
      </Defs>
      <Rect fill={`url(#${id}-film)`} height={H} width={width} x={left} />
      {/* A red block 20%/20% into the picture, 24% × 38%. */}
      <Rect
        fill={a.block}
        height={H * 0.38}
        rx={2.4}
        width={width * 0.24}
        x={left + width * 0.2}
        y={H * 0.2}
      />
    </>
  );
};

let sceneSeq = 0;

// One 16:9 frame: the scene, then whatever the caller lays over its active picture (in px).
const LogoScene = ({ children, label, scene }: LogoSceneProps) => {
  const [size, setSize] = useState<{ height: number; width: number } | null>(null);
  // Gradient ids are document-global on web: each frame needs its own.
  const [id] = useState(() => `logo-scene-${++sceneSeq}`);
  const pic = scene === "box" ? PICTURE.box : PICTURE.full;
  return (
    <View
      aria-label={label}
      aspectRatio={16 / 9}
      backgroundColor={sceneArt.frame}
      borderColor={sceneArt.edge}
      borderRadius={6}
      borderWidth={1}
      onLayout={(e) => setSize({ height: e.nativeEvent.layout.height, width: e.nativeEvent.layout.width })}
      overflow="hidden"
      role="img"
      width="100%"
    >
      <Svg
        height="100%"
        preserveAspectRatio="none"
        style={{ position: "absolute" }}
        viewBox={`0 0 ${W} ${H}`}
        width="100%"
      >
        {scene === "bright" ? <Bright id={id} /> : scene === "dark" ? <Dark id={id} /> : <Box id={id} />}
      </Svg>
      {size && children ? (
        <View
          bottom={0}
          left={`${(pic.left / W) * 100}%`}
          overflow="hidden"
          position="absolute"
          top={0}
          width={`${(pic.width / W) * 100}%`}
        >
          {children({ height: size.height, width: (size.width * pic.width) / W })}
        </View>
      ) : null}
    </View>
  );
};

export type { LogoSceneKind, LogoSceneProps };
export { LogoScene };
