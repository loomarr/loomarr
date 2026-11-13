import { Text, TextLink } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// TextLink — a link written into the page: the info colour, an underline, the surrounding type, an
// optional glyph, and the one amber outline for focus. An in-app link is drawn onto the router's
// `Link` with `render`; the stories use an anchor, which is what a router link renders.
const Workshop = ({ width = 520 }: { width?: number }) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: width, padding: 24 }}>
    <Text as="p" textRole="hint" tone="secondary">
      <TextLink external href="https://archive.example/details/reel-12">
        Open on Archive.org
      </TextLink>
    </Text>
    <Text as="p" textRole="hint" tone="secondary">
      That check did not pass.{" "}
      <TextLink href="/help?page=troubleshooting" icon="goTo">
        Fix
      </TextLink>
    </Text>
    <Text as="p" textRole="compact" tone="primary">
      Add your key above and save to load the models, or{" "}
      <TextLink external href="https://keys.example/new">
        get a key
      </TextLink>{" "}
      first. A longer link wraps with the words around it, such as{" "}
      <TextLink href="/help?page=concepts">the page that explains every part of a channel</TextLink>.
    </Text>
    <Text as="p" textRole="compact" tone="primary">
      <TextLink href="/filler" icon="goBack" iconPlacement="start">
        Back to filler
      </TextLink>
    </Text>
  </div>
);

const meta = {
  title: "Loomarr Components/TextLink",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };
const Narrow: Story = { args: { width: 260 } };
const Focused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("link", { name: "Open on Archive.org" }).focus();
  },
};

export default meta;
export { Default, Focused, Light, Narrow };
