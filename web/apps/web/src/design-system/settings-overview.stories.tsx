import { SearchField, SettingsOverview, type SettingsOverviewCard } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { cardDestinations, destinationById, SETTINGS_CARDS } from "@/settings/settings-destinations";

// The Settings index (#2786, Direction mock variant C): seven cards plus Advanced, built from the
// real destination catalog. Each card has a status word; a card with a problem says its first one
// with a link to the page that fixes it, and comes first. The links are plain anchors here; the
// app passes router links. The problems are written the way the index words them.

type Problem = { kind: "attention" | "overdue" | "notSetUp"; text: string; fix: string };

const cards = (isAdmin: boolean, problems: Partial<Record<string, Problem>>): SettingsOverviewCard[] =>
  SETTINGS_CARDS.flatMap((card) => {
    const pages = cardDestinations(card.id, isAdmin);
    if (pages.length === 0) return [];
    const problem = problems[card.id];
    const fix = problem ? destinationById(problem.fix) : undefined;
    return [
      {
        id: card.id,
        title: card.title,
        description: card.description,
        status: problem?.kind ?? "ok",
        problem:
          problem && fix
            ? { text: problem.text, pageLabel: fix.label, link: <a href={fix.path}>{fix.label}</a> }
            : undefined,
        pages: pages.map((page) => {
          const host = page.hostId ? destinationById(page.hostId) : undefined;
          return {
            id: page.id,
            label: page.label,
            note: host ? `on ${host.label}` : undefined,
            link: <a href={page.path}>{page.label}</a>,
          };
        }),
      },
    ];
  });

const SOME_PROBLEMS: Record<string, Problem> = {
  connections: { kind: "attention", text: "A download service isn't answering.", fix: "connections" },
  filler: { kind: "attention", text: "Clips are using 91% of their space.", fix: "filler.storage" },
  notifications: {
    kind: "notSetUp",
    text: "Nothing tells you when something goes wrong.",
    fix: "notifications",
  },
  server: { kind: "attention", text: "Backups missed 2 nights.", fix: "backup" },
};

const search = (
  <SearchField
    clearLabel="Clear settings search"
    label="Find a setting"
    labelHidden
    onChangeText={() => undefined}
    placeholder="Try “Jellyfin”, “backups”, or FILLER_FETCH_EVERY"
    value=""
  />
);

const advanced = {
  label: "Advanced settings",
  description: "Find and edit a setting by its exact key or environment variable.",
  link: <a href="/settings/advanced">Advanced settings</a>,
};

const meta = {
  title: "Loomarr Components/Settings Overview",
  component: SettingsOverview,
  args: { advanced, cards: cards(true, SOME_PROBLEMS), search },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof SettingsOverview>;

type Story = StoryObj<typeof meta>;

// An admin with four things to look at: they lead, each with the page that fixes it.
const AdminSomeProblems: Story = {};
const AdminAllReady: Story = { args: { cards: cards(true, {}) } };
// A member can visit only Notifications, so that is the one card, and there is no Advanced row.
const Member: Story = {
  args: {
    advanced: undefined,
    cards: cards(false, { notifications: SOME_PROBLEMS.notifications as Problem }),
  },
};

export default meta;
export { AdminAllReady, AdminSomeProblems, Member };
