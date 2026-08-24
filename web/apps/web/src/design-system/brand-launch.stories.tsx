import type { Density } from "@loomarr/design-system";
import { BrandLaunch, LoomarrProvider, semanticThemes } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const frames = {
  pointer: { height: 540, label: "WEB", width: 860 },
  touch: { height: 640, label: "MOBILE", width: 375 },
  tv: { height: 540, label: "TV", width: 960 },
} as const;

const ReplayableLaunch = ({
  density,
  reducedMotion = false,
  theme = "dark",
}: {
  density: Density;
  reducedMotion?: boolean;
  theme?: "dark" | "light";
}) => {
  const [run, setRun] = useState(0);
  const frame = frames[density];
  return (
    <LoomarrProvider theme={theme}>
      <div
        style={{
          background: semanticThemes[theme].surface.canvas,
          height: frame.height,
          margin: "32px auto",
          maxWidth: "calc(100vw - 64px)",
          overflow: "hidden",
          position: "relative",
          width: frame.width,
        }}
      >
        <BrandLaunch density={density} key={run} reducedMotion={reducedMotion} />
        <button
          onClick={() => setRun((value) => value + 1)}
          style={{
            background: semanticThemes[theme].surface.raised,
            border: `1px solid ${semanticThemes[theme].border.control}`,
            borderRadius: 6,
            bottom: 16,
            color: semanticThemes[theme].content.primary,
            cursor: "pointer",
            fontFamily: "monospace",
            fontSize: 12,
            padding: "8px 12px",
            position: "absolute",
            right: 16,
          }}
          type="button"
        >
          REPLAY {frame.label}
        </button>
      </div>
    </LoomarrProvider>
  );
};

const meta = {
  title: "Loomarr Foundations/Brand Launch",
  component: ReplayableLaunch,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ReplayableLaunch>;

type Story = StoryObj<typeof meta>;
const Web: Story = { args: { density: "pointer" } };
const Mobile: Story = { args: { density: "touch" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { args: { density: "pointer", theme: "light" } };
const ReducedMotion: Story = { args: { density: "pointer", reducedMotion: true } };

export default meta;
export { Light, Mobile, ReducedMotion, Tv, Web };
