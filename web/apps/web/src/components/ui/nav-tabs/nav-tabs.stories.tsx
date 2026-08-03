import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { widthFrame } from "@/test/story-utils";
import { NavTabs } from "./nav-tabs";

// THE tab bar for this app — Settings, Filler and Queue all render through it, so "you are here"
// looks the same everywhere.
//
// The pill treatment came from Settings (maintainer's pick, 2026-08-02) and replaced an
// underline bar on the other two. Every tab is a real `<Link>`: all three bars already kept their
// position in the URL, so each tab was a destination all along — two of them just rendered as
// buttons calling `navigate()`, which looks identical but cannot be middle-clicked, copied as a
// link, or announced to assistive tech as a place rather than an action.
//
// ⚠ `linkComponent` is injected rather than imported so the component stays router-agnostic —
// Storybook has no TanStack Router, and neither will the native shell. Here it is a plain anchor;
// the app passes the real `Link`.
const storyLink = ({
  to,
  className,
  children,
  ...rest
}: {
  to: string;
  className?: string;
  children?: ReactNode;
  [key: string]: unknown;
}) => (
  <a href={to} className={className} {...rest}>
    {children}
  </a>
);

const meta = {
  title: "UI/NavTabs",
  component: NavTabs,
  decorators: [widthFrame(720)],
  args: { linkComponent: storyLink },
} satisfies Meta<typeof NavTabs>;

type Story = StoryObj<typeof meta>;

// Filler's bar. ⚠ Sources carries NO count: Catalog and Incoming are queues where the number is
// the reason to look, while Sources is a destination. A "0" there would read as an empty list.
const WithCounts: Story = {
  args: {
    label: "Filler sections",
    activeId: "catalog",
    tabs: [
      { id: "catalog", label: "Catalog", to: "/filler", count: 42 },
      { id: "incoming", label: "Incoming", to: "/filler", count: 3 },
      { id: "sources", label: "Sources", to: "/filler" },
    ],
  },
};

// The active pill can be any tab — here the last, which is also the case where the amber fill
// has to survive a tab that has no count pill beside it.
const ActiveWithoutCount: Story = {
  args: {
    label: "Filler sections",
    activeId: "sources",
    tabs: [
      { id: "catalog", label: "Catalog", to: "/filler", count: 42 },
      { id: "incoming", label: "Incoming", to: "/filler", count: 3 },
      { id: "sources", label: "Sources", to: "/filler" },
    ],
  },
};

// Settings: six destinations, no counts at all — nothing there is a queue.
const NoCounts: Story = {
  args: {
    label: "Settings",
    activeId: "/settings/connections",
    tabs: [
      { id: "/settings/connections", label: "Connections", to: "/settings/connections" },
      { id: "/settings/ai", label: "AI", to: "/settings/ai" },
      { id: "/settings/defaults", label: "Defaults", to: "/settings/defaults" },
      { id: "/settings/system", label: "System", to: "/settings/system" },
      { id: "/settings/security", label: "Security", to: "/settings/security" },
      { id: "/settings/all", label: "All settings", to: "/settings/all" },
    ],
  },
};

// A member's Queue: one tab. Approving is admin-only (§11) and a history of other people's
// decisions is not theirs to read — the bar keeps its shape rather than vanishing for half the
// users.
const SingleTab: Story = {
  args: {
    label: "Queue sections",
    activeId: "flight",
    tabs: [{ id: "flight", label: "In flight", to: "/queue", count: 7 }],
  },
};

// ⚠ Overflowing labels SCROLL sideways rather than wrapping — a bar that wraps to two rows
// pushes the page content down as tabs are added, and the active pill can end up on a line the
// operator is not looking at.
const Overflowing: Story = {
  args: {
    label: "Many sections",
    activeId: "d",
    tabs: [
      { id: "a", label: "Connections", to: "/x", count: 3 },
      { id: "b", label: "Artificial intelligence", to: "/x" },
      { id: "c", label: "Programming defaults", to: "/x", count: 12 },
      { id: "d", label: "System and machine", to: "/x" },
      { id: "e", label: "Security and access", to: "/x", count: 1 },
      { id: "f", label: "Every setting there is", to: "/x" },
    ],
  },
};

export default meta;
export { ActiveWithoutCount, NoCounts, Overflowing, SingleTab, WithCounts };
