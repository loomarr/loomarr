// PROTOTYPE — throwaway (Direction mocks, 2026-11-08, #2785)
// Stub data for Home's admin additions on the channel wall: the "Needs you" counts and who is
// watching what right now. Names and counts are invented (the repo is public). Each switch has an
// empty side, so the hidden strip and the empty watching panel can be judged too.

const ADMIN_LAYOUTS = [
  { key: "A", name: "Strip above the wall" },
  { key: "B", name: "One quiet line" },
  { key: "C", name: "Beside the wall" },
] as const;
type AdminLayout = (typeof ADMIN_LAYOUTS)[number]["key"];

const NEEDS = ["some", "zero"] as const;
type Needs = (typeof NEEDS)[number];

const WATCHING = ["some", "none"] as const;
type Watching = (typeof WATCHING)[number];

interface AdminSearch {
  admin: "0" | "1";
  layout: AdminLayout;
  needs: Needs;
  watching: Watching;
}

const parseAdminSearch = (raw: Record<string, unknown>): AdminSearch => ({
  admin: raw.admin === "1" || raw.admin === 1 ? "1" : "0",
  layout: ADMIN_LAYOUTS.some((l) => l.key === raw.layout) ? (raw.layout as AdminLayout) : "A",
  needs: NEEDS.includes(raw.needs as Needs) ? (raw.needs as Needs) : "some",
  watching: WATCHING.includes(raw.watching as Watching) ? (raw.watching as Watching) : "some",
});

// Where each count leads: today's pages, no new routes.
type NeedTo = "/requests/needs-you" | "/filler/manage" | "/settings/system/tasks" | "/filler/settings";

interface NeedItem {
  id: string;
  count: number;
  // The number as shown: a count, or the quota as a share.
  value: string;
  // What the number is, in a person's words, for a count of one and of many.
  one: string;
  many: string;
  // Short form for the quiet line and the phone.
  short: string;
  to: NeedTo;
  // Filler quota is a warning, not a to-do: it shows only once space runs short.
  warn?: boolean;
}

const needItems = (needs: Needs): NeedItem[] => {
  const zero = needs === "zero";
  return [
    {
      id: "approvals",
      count: zero ? 0 : 3,
      value: zero ? "0" : "3",
      one: "request to approve",
      many: "requests to approve",
      short: "to approve",
      to: "/requests/needs-you",
    },
    {
      id: "filler",
      count: zero ? 0 : 2,
      value: zero ? "0" : "2",
      one: "filler choice to make",
      many: "filler choices to make",
      short: "filler choices",
      to: "/filler/manage",
    },
    {
      id: "tasks",
      count: zero ? 0 : 1,
      value: zero ? "0" : "1",
      one: "task overdue",
      many: "tasks overdue",
      short: "overdue",
      to: "/settings/system/tasks",
    },
    {
      id: "quota",
      count: zero ? 0 : 1,
      value: "91%",
      one: "of filler space used",
      many: "of filler space used",
      short: "filler space",
      to: "/filler/settings",
      warn: true,
    },
  ];
};

const needLabel = (n: NeedItem) => (n.count === 1 || n.warn ? n.one : n.many);

interface Viewer {
  id: string;
  name: string;
  // The channel they are on, by the Home stub's channel id.
  channelId: string;
  device: string;
}

const viewers = (watching: Watching): Viewer[] =>
  watching === "none"
    ? []
    : [
        { id: "v-sam", name: "Sam", channelId: "ch-3", device: "Living room TV" },
        { id: "v-alex", name: "Alex", channelId: "ch-5", device: "Bedroom TV" },
        { id: "v-jo", name: "Jo", channelId: "ch-3", device: "Phone" },
      ];

export type { AdminLayout, AdminSearch, NeedItem, Needs, Viewer, Watching };
export { ADMIN_LAYOUTS, NEEDS, needItems, needLabel, parseAdminSearch, viewers, WATCHING };
