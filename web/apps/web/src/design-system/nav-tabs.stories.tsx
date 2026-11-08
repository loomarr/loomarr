import { type LinkLike, NavTabs } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";

// THE tab bar for this app — Settings, Filler and Queue all render through it, so "you are here"
// looks the same everywhere. Every tab is a real link: each tab is a destination, so it can be
// middle-clicked, copied, and announced as a place rather than an action.
//
// ⚠ `linkComponent` is injected rather than imported so the component stays router-agnostic —
// Storybook has no TanStack Router, and neither will the native shell. Here it is a plain anchor;
// the app passes the real `Link`.
const storyLink: LinkLike = ({ activeOptions: _activeOptions, children, search: _search, to, ...rest }) => (
  <a href={to} {...rest}>
    {children}
  </a>
);

const meta = {
  title: "Loomarr Components/NavTabs",
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

// The active pill can be any tab — here the last, which has no count beside it.
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
      { id: "/settings/access", label: "Access and devices", to: "/settings/access" },
      { id: "/settings/advanced", label: "Advanced settings", to: "/settings/advanced" },
    ],
  },
};

// A member's Queue: one tab. The bar keeps its shape rather than vanishing for half the users.
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

// The v2 mock's Queue tab bar, used by Requests: transparent tabs with a 2px underline on the
// active one. The count on a tab that holds work waiting on the viewer takes the `suggest` tint.
const Underline: Story = {
  args: {
    label: "Requests sections",
    variant: "underline",
    activeId: "needs-you",
    tabs: [
      { id: "needs-you", label: "Needs you", to: "/requests/needs-you", count: 3, attention: true },
      { id: "in-progress", label: "In progress", to: "/requests/in-progress", count: 5 },
      { id: "done", label: "Done", to: "/requests/done", count: 12 },
    ],
  },
};

export default meta;
export { ActiveWithoutCount, NoCounts, Overflowing, SingleTab, Underline, WithCounts };
