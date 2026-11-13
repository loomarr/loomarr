import { DockedPanel, LoomarrProvider, semanticThemes } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The docked panel sits inside a positioned frame that is not the page: here a stand-in for the
// player's picture. It has no scrim and no focus trap, so the picture stays beside it, and it wears
// the player's dark chrome in both themes.
const DockedPanelGallery = ({
  edge = "right",
  theme = "dark",
}: {
  edge?: "left" | "right";
  theme?: "dark" | "light";
}) => {
  const colors = semanticThemes[theme];
  return (
    <LoomarrProvider theme={theme}>
      <main
        style={{
          background: colors.surface.canvas,
          boxSizing: "border-box",
          minHeight: "100vh",
          padding: 32,
        }}
      >
        <div
          style={{
            background: "#202833",
            color: "#F4F6FA",
            height: 420,
            overflow: "hidden",
            position: "relative",
          }}
        >
          <p style={{ margin: 24 }}>Playback keeps running</p>
          <DockedPanel
            closeLabel="Close channels"
            edge={edge}
            meta="now to next 2h"
            onClose={() => undefined}
            title="Channels"
          >
            <ul style={{ margin: 0, padding: 10 }}>
              <li>07 Westerns</li>
              <li>08 Cartoons</li>
            </ul>
          </DockedPanel>
        </div>
      </main>
    </LoomarrProvider>
  );
};

const meta = {
  title: "Loomarr Foundations/Docked Panel",
  component: DockedPanelGallery,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DockedPanelGallery>;

type Story = StoryObj<typeof meta>;
const Right: Story = {};
const Left: Story = { args: { edge: "left" } };
const Light: Story = { args: { theme: "light" } };

export default meta;
export { Left, Light, Right };
