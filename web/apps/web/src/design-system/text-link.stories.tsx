import { Text, TextLink } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";

// TextLink — a link written into the page: the info colour (or a `tone` that fits its surroundings),
// an underline (or one that draws on hover), the surrounding type, an optional glyph, and the one
// amber outline for focus. An in-app link is drawn onto the router's
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
      A page that routes its own references reads them the same: see{" "}
      <TextLink onPress={() => undefined}>the approval gate</TextLink>.
    </Text>
    <Text as="p" textRole="compact" tone="primary">
      <TextLink href="/filler" icon="goBack" iconPlacement="start">
        Back to filler
      </TextLink>
    </Text>
  </div>
);

// The tones and the hover-only underline, for a page that moves its links onto TextLink without
// changing how they rest: each row is a line of text that already has its own colour, and the link
// takes it (`inherit`), the amber of the primary action (`primary`), or the quiet text colour that
// wakes up under the pointer (`muted`). `underline="hover"` draws the line only under the pointer.
// Each link stands in a block of its own beside its caption, as the converted links do (a title in a
// list, a link under a form): a hover-only underline inside a running sentence would tell the link
// apart by colour alone, which the accessibility check rightly refuses.
const Tone = ({ caption, children }: { caption: string; children: ReactNode }) => (
  <div style={{ alignItems: "baseline", display: "flex", gap: 12 }}>
    <div style={{ flex: "0 0 220px" }}>
      <Text as="span" textRole="hint" tone="secondary">
        {caption}
      </Text>
    </div>
    <Text as="div" textRole="compact" tone="primary">
      {children}
    </Text>
  </div>
);

const Tones = () => (
  <div style={{ display: "flex", flexDirection: "column", gap: 16, maxWidth: 560, padding: 24 }}>
    <Tone caption="Takes the text around it">
      <TextLink href="/requests/1" tone="inherit" underline="hover">
        A request title
      </TextLink>
    </Tone>
    <Tone caption="The amber of the primary action">
      <TextLink href="/forgot-password" tone="primary" underline="hover">
        Forgot password?
      </TextLink>
    </Tone>
    <Tone caption="Quiet until the pointer reaches it">
      <TextLink href="/settings/library" tone="muted" underline="hover">
        Library settings
      </TextLink>
    </Tone>
    <Tone caption="Quiet, with a hairline underline">
      <TextLink href="/filler/incoming" tone="muted">
        3 being checked
      </TextLink>
    </Tone>
    <Tone caption="Info colour, underline on hover">
      <TextLink href="/login" underline="hover">
        Sign in instead
      </TextLink>
    </Tone>
  </div>
);

const meta = {
  title: "Loomarr Components/TextLink",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const TonesAtRest: Story = { render: () => <Tones /> };
const TonesLight: Story = { globals: { theme: "light" }, render: () => <Tones /> };
const MutedHovered: Story = {
  render: () => <Tones />,
  play: async ({ canvas, userEvent }) => {
    await userEvent.hover(canvas.getByRole("link", { name: "Library settings" }));
  },
};
const InheritFocused: Story = {
  render: () => <Tones />,
  play: async ({ canvas }) => {
    canvas.getByRole("link", { name: "A request title" }).focus();
  },
};
const Light: Story = { globals: { theme: "light" } };
const Narrow: Story = { args: { width: 260 } };
const Focused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("link", { name: "Open on Archive.org" }).focus();
  },
};

export default meta;
export { Default, Focused, InheritFocused, Light, MutedHovered, Narrow, TonesAtRest, TonesLight };
