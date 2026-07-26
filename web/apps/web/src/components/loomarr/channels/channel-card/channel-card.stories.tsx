import type { Meta, StoryObj } from "@storybook/react-vite";
import { MoreVertical } from "lucide-react";
import { Button } from "@/components/ui";
import { ChannelCard } from "./channel-card";

// ChannelCard states (§3): healthy · pending-slots · drift · error · creating. The story
// set IS the gallery contract — the visual suite snapshots each at both viewports (§5).
const meta = {
  title: "Channels/ChannelCard",
  component: ChannelCard,
} satisfies Meta<typeof ChannelCard>;

type Story = StoryObj<typeof meta>;

const Healthy: Story = {
  args: {
    number: 42,
    name: "90s Action",
    onAir: "live",
    nowNext: { now: { title: "Heat", until: "9:42 PM" }, next: { title: "Con Air" } },
    health: "healthy",
  },
};

const PendingSlots: Story = {
  args: {
    number: 43,
    name: "Saturday Cartoons",
    onAir: "reconciling",
    nowNext: { now: { title: "Filler" }, gap: true, next: { title: "Animaniacs" } },
    health: "pending-slots",
  },
};

const Drift: Story = {
  args: {
    number: 44,
    name: "Cozy Mysteries",
    onAir: "live",
    nowNext: { now: { title: "Poirot", until: "10:15 PM" } },
    health: "drift",
  },
};

const Errored: Story = {
  args: { number: 45, name: "Late Night", onAir: "off", health: "error" },
};

const Creating: Story = {
  args: { number: 46, name: "New Channel", onAir: "off", health: "creating" },
};

// With a trailing action (the admin per-row ⋮ menu on the Channels list). It clusters with
// the on-air dot in the top-right, inside the card — a stand-in button here since the real
// menu owns live hooks; the slot placement is what this snapshots.
const WithMenu: Story = {
  args: {
    number: 42,
    name: "90s Action",
    onAir: "live",
    nowNext: { now: { title: "Heat", until: "9:42 PM" }, next: { title: "Con Air" } },
    health: "healthy",
    trailing: (
      <Button variant="ghost" size="icon" className="size-8" aria-label="Actions">
        <MoreVertical className="size-4" aria-hidden />
      </Button>
    ),
  },
};

export default meta;
export { Creating, Drift, Errored, Healthy, PendingSlots, WithMenu };
