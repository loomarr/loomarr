import type { Density } from "@loomarr/design-system";
import {
  ActivityIndicator,
  LoomarrProvider,
  ProgressTrack,
  SignalLoader,
  Skeleton,
  semanticThemes,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

const LoadingGallery = ({
  density = "pointer",
  reducedMotion,
  theme = "dark",
}: {
  density?: Density;
  reducedMotion?: boolean;
  theme?: "dark" | "light";
}) => {
  const colors = semanticThemes[theme];
  const section = {
    background: colors.surface.raised,
    border: `1px solid ${colors.border.decorative}`,
    borderRadius: 16,
    padding: density === "tv" ? 32 : 24,
  } as const;
  const label = {
    color: colors.content.secondary,
    fontFamily: "monospace",
    fontSize: density === "tv" ? 16 : 12,
    letterSpacing: "0.08em",
    margin: "0 0 20px",
  } as const;

  return (
    <LoomarrProvider theme={theme}>
      <main
        style={{
          background: colors.surface.canvas,
          boxSizing: "border-box",
          color: colors.content.primary,
          display: "grid",
          gap: 20,
          minHeight: "100vh",
          padding: density === "tv" ? 48 : 32,
        }}
      >
        <header>
          <p style={{ ...label, color: colors.action.primary, marginBottom: 8 }}>LOOMARR FOUNDATIONS</p>
          <h1 style={{ fontFamily: "sans-serif", fontSize: density === "tv" ? 42 : 30, margin: 0 }}>
            Loading
          </h1>
          <p style={{ color: colors.content.secondary, fontFamily: "sans-serif", marginBottom: 0 }}>
            Match the indicator to the kind of waiting—not every wait is a tuning moment.
          </p>
        </header>

        <section style={section}>
          <p style={label}>INLINE ACTIVITY</p>
          <div style={{ alignItems: "center", display: "flex", gap: 28 }}>
            <ActivityIndicator accessibilityLabel="Saving" reducedMotion={reducedMotion} size="compact" />
            <ActivityIndicator accessibilityLabel="Refreshing guide" reducedMotion={reducedMotion} />
            <ActivityIndicator
              accessibilityLabel="Loading details"
              reducedMotion={reducedMotion}
              size="control"
              tone="secondary"
            />
          </div>
        </section>

        <section style={section}>
          <p style={label}>SIGNAL ACQUISITION</p>
          <div
            style={{ display: "grid", gap: 36, gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))" }}
          >
            <SignalLoader density={density} reducedMotion={reducedMotion} />
            <SignalLoader
              density={density}
              detail="Connecting to channel · 00:04"
              label="ACQUIRING SIGNAL"
              reducedMotion={reducedMotion}
            />
          </div>
        </section>

        <section style={section}>
          <p style={label}>SKELETONS</p>
          <div style={{ display: "grid", gap: 16, gridTemplateColumns: "minmax(180px, 320px) 1fr" }}>
            <Skeleton reducedMotion={reducedMotion} shape="media" />
            <div style={{ display: "grid", gap: 14 }}>
              <Skeleton reducedMotion={reducedMotion} width="72%" />
              <Skeleton reducedMotion={reducedMotion} width="96%" />
              <Skeleton reducedMotion={reducedMotion} width="58%" />
              <div style={{ alignItems: "center", display: "flex", gap: 12, marginTop: 8 }}>
                <Skeleton reducedMotion={reducedMotion} shape="circle" />
                <Skeleton reducedMotion={reducedMotion} width="44%" />
              </div>
            </div>
          </div>
        </section>

        <section style={section}>
          <p style={label}>DETERMINATE PROGRESS</p>
          <p style={{ color: colors.content.primary, fontFamily: "sans-serif", margin: "0 0 8px" }}>
            Matching programme artwork
          </p>
          <p
            style={{
              color: colors.content.secondary,
              fontFamily: "monospace",
              fontSize: 13,
              margin: "0 0 14px",
            }}
          >
            STAGE 2 OF 3 · 68%
          </p>
          <ProgressTrack accessibilityLabel="Matching programme artwork" percent={68} width="100%" />
        </section>
      </main>
    </LoomarrProvider>
  );
};

const meta = {
  title: "Loomarr Foundations/Loading",
  component: LoadingGallery,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof LoadingGallery>;

type Story = StoryObj<typeof meta>;
const Dark: Story = { args: { density: "pointer", reducedMotion: true, theme: "dark" } };
const Light: Story = { args: { density: "pointer", reducedMotion: true, theme: "light" } };
const Tv: Story = { args: { density: "tv", reducedMotion: true, theme: "dark" } };
const ReducedMotion: Story = { args: { density: "pointer", reducedMotion: true, theme: "dark" } };
const Animated: Story = {
  args: { density: "pointer", reducedMotion: false, theme: "dark" },
  tags: ["motion-only"],
};

export default meta;
export { Animated, Dark, Light, ReducedMotion, Tv };
