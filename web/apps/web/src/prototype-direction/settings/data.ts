// PROTOTYPE — throwaway (Direction mocks, 2026-11-08, #2786)
// Stub data for the Settings index as seven cards plus Advanced. Each card lists today's pages by
// their real names and paths (settings-destinations.ts and filler-settings-section.ts on main): no
// page merges and no route changes. Status words follow #2754/#2755: a dot and a sentence-case word.
// The problems are invented (the repo is public).
import type { StatusTone } from "@loomarr/design-system";

const SETTINGS_VARIANTS = [
  { key: "A", name: "Grid of cards" },
  { key: "B", name: "List, status on the right" },
  { key: "C", name: "Cards, first issue inline" },
] as const;

const SETTINGS_STATES = ["some-issues", "all-ready"] as const;
type SettingsState = (typeof SETTINGS_STATES)[number];

interface SettingsPage {
  label: string;
  path: string;
  // Only when the page is reached through another card's page (location lives on the access page).
  note?: string;
}

interface CardStatus {
  tone: StatusTone;
  word: string;
  // The first thing to fix, in a sentence, and the page that fixes it.
  issue?: { text: string; page: string };
}

interface SettingsCard {
  id: string;
  title: string;
  description: string;
  pages: SettingsPage[];
  status: Record<SettingsState, CardStatus>;
}

const READY: CardStatus = { tone: "ok", word: "Ready" };

const CARDS: SettingsCard[] = [
  {
    id: "connections",
    title: "Connections",
    description: "Your media server, download services and where your shows live.",
    pages: [
      { label: "Connections", path: "/settings/connections" },
      { label: "Library", path: "/settings/library" },
    ],
    status: {
      "some-issues": {
        tone: "warn",
        word: "Needs attention",
        issue: { text: "Sonarr hasn’t answered since 6:40 PM.", page: "Connections" },
      },
      "all-ready": READY,
    },
  },
  {
    id: "ai",
    title: "AI",
    description: "The service and models behind suggestions and clip checks.",
    pages: [{ label: "AI setup", path: "/settings/ai" }],
    status: { "some-issues": READY, "all-ready": READY },
  },
  {
    id: "channels",
    title: "Channels",
    description: "What every channel starts with unless you change it.",
    pages: [
      { label: "Channel defaults", path: "/settings/defaults" },
      { label: "Your location", path: "/settings/access", note: "on Sign-in, devices and sharing" },
    ],
    status: { "some-issues": READY, "all-ready": READY },
  },
  {
    id: "filler",
    title: "Filler",
    description: "How short clips between shows are found, kept and played.",
    pages: [
      { label: "Filler settings", path: "/filler/settings" },
      { label: "Automatic downloads", path: "/filler/settings/downloads" },
      { label: "Clip folders", path: "/filler/settings/folders" },
      { label: "Storage limits", path: "/filler/settings/storage" },
      { label: "Clip details", path: "/filler/settings/details" },
      { label: "Incoming history", path: "/filler/settings/incoming" },
      { label: "Break assembly", path: "/filler/settings/breaks" },
      { label: "Clip review", path: "/filler/settings/review" },
      { label: "Clip eligibility and sound", path: "/filler/settings/playback" },
      { label: "Processing limits", path: "/filler/settings/limits" },
      { label: "Processing tools", path: "/filler/settings/tools" },
    ],
    status: {
      "some-issues": {
        tone: "warn",
        word: "Needs attention",
        issue: { text: "Clips are using 91% of their space.", page: "Storage limits" },
      },
      "all-ready": READY,
    },
  },
  {
    id: "people",
    title: "People and access",
    description: "Who can watch, how they sign in, and paired TVs.",
    pages: [
      { label: "People", path: "/people" },
      { label: "Sign-in, devices and sharing", path: "/settings/access" },
    ],
    status: { "some-issues": READY, "all-ready": READY },
  },
  {
    id: "notifications",
    title: "Notifications",
    description: "How Loomarr tells you when something needs you.",
    pages: [{ label: "Notifications", path: "/settings/notifications" }],
    status: {
      "some-issues": {
        tone: "off",
        word: "Off",
        issue: { text: "Nothing tells you when a channel stops.", page: "Notifications" },
      },
      "all-ready": { tone: "ok", word: "On" },
    },
  },
  {
    id: "server",
    title: "Server",
    description: "Playback, storage, backups and what runs in the background.",
    pages: [
      { label: "Playback", path: "/settings/system/playback" },
      { label: "Artwork storage", path: "/settings/system/storage" },
      { label: "Backups", path: "/settings/system/backup" },
      { label: "Background tasks", path: "/settings/system/tasks" },
      { label: "Diagnostics", path: "/settings/system/diagnostics" },
      { label: "Database", path: "/settings/system/database" },
      { label: "About Loomarr", path: "/settings/system/about" },
    ],
    status: {
      "some-issues": {
        tone: "warn",
        word: "Needs attention",
        issue: { text: "Backups haven’t run for 9 days.", page: "Backups" },
      },
      "all-ready": READY,
    },
  },
];

// Advanced is not a card: one quiet row under the seven.
const ADVANCED: SettingsPage & { description: string } = {
  label: "Advanced settings",
  path: "/settings/advanced",
  description: "Find and edit a setting by its exact key or environment variable.",
};

const cardById = (id: string | undefined) => CARDS.find((c) => c.id === id);

export type { CardStatus, SettingsCard, SettingsPage, SettingsState };
export { ADVANCED, CARDS, cardById, SETTINGS_STATES, SETTINGS_VARIANTS };
