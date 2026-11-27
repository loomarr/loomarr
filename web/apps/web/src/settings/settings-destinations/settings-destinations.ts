import { type FillerSettingsSection, SETTINGS_SECTIONS } from "@/filler/filler-settings-section";

// The seven cards on the Settings index, plus Advanced as one quiet row under them (#2786, variant C).
type SettingsCardId =
  | "connections"
  | "ai"
  | "channels"
  | "filler"
  | "people"
  | "notifications"
  | "server"
  | "advanced";
type SettingsRole = "all" | "admin";

type SettingsStaticDestinationPath =
  | "/settings/access"
  | "/settings/connections"
  | "/settings/library"
  | "/settings/ai"
  | "/settings/defaults"
  | "/settings/notifications"
  | "/settings/system/playback"
  | "/settings/system/storage"
  | "/settings/system/backup"
  | "/settings/system/tasks"
  | "/settings/system/diagnostics"
  | "/settings/system/database"
  | "/settings/system/about"
  | "/settings/advanced"
  | "/filler/settings"
  | "/people";

type SettingsDestinationPath = SettingsStaticDestinationPath | `/filler/settings/${FillerSettingsSection}`;

interface SettingsDestinationBase {
  id: string;
  owner?: string;
  label: string;
  description: string;
  aliases: readonly string[];
  card: SettingsCardId;
  role: SettingsRole;
  // Listed on its card. A destination that is not is still found by search.
  browse: boolean;
  // Set when the destination is a part of another listed page rather than a page of its own: the
  // card lists it with "on <that page>", and the link opens that page.
  hostId?: string;
}

type SettingsDestination = SettingsDestinationBase &
  (
    | { path: SettingsStaticDestinationPath; section?: never }
    | { path: `/filler/settings/${FillerSettingsSection}`; section: FillerSettingsSection }
  );

const destination = (value: SettingsDestination): SettingsDestination => value;

// One catalog owns Settings wording and browser destinations. Backend owner ids stay stable while
// paths and labels can evolve here; action-only destinations simply omit owner. A card lists its
// destinations in this order.
const SETTINGS_DESTINATIONS = [
  destination({
    id: "connections",
    owner: "settings.connections",
    label: "Connections",
    description: "Connect your media server, download services, Tunarr, and TMDB.",
    aliases: ["Emby", "Jellyfin", "Seerr", "Sonarr", "Radarr", "Tunarr", "TMDB", "services"],
    card: "connections",
    path: "/settings/connections",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "library",
    label: "Library",
    description: "Choose the folders that hold your shows and movies.",
    aliases: ["media folders", "scan", "mounts", "path mapping", "where is my media"],
    card: "connections",
    path: "/settings/library",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "ai",
    owner: "settings.ai",
    label: "AI setup",
    description: "Choose the service and models Loomarr uses for suggestions and clip analysis.",
    aliases: ["OpenRouter", "OpenAI", "Ollama", "models", "suggestions", "vision", "transcription"],
    card: "ai",
    path: "/settings/ai",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "defaults",
    owner: "settings.defaults",
    label: "Channel defaults",
    description: "Choose what new and existing channels inherit unless you override them.",
    aliases: ["schedule ahead", "commercial frequency", "breaks per hour", "channel behavior"],
    card: "channels",
    path: "/settings/defaults",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "notifications",
    owner: "settings.notifications",
    label: "Notifications",
    description: "Choose how Loomarr tells you about activity and problems.",
    aliases: ["email", "Slack", "Discord", "browser push", "alerts"],
    card: "notifications",
    path: "/settings/notifications",
    role: "all",
    browse: true,
  }),
  destination({
    id: "location",
    owner: "settings.location",
    label: "Your location",
    description: "Set the country and local area Loomarr uses for relevant content.",
    aliases: ["where I live", "country", "city", "area", "radius", "local stations", "geography"],
    card: "channels",
    path: "/settings/access",
    role: "admin",
    browse: true,
    // Edited on the access page, which the People and access card lists as a page of its own.
    hostId: "access",
  }),
  destination({
    id: "sharing",
    owner: "settings.sharing",
    label: "Sharing address",
    description: "Set the Loomarr address people can open from invitations and recovery links.",
    aliases: ["public URL", "invitation link", "recovery link", "recipient address", "share"],
    card: "people",
    path: "/settings/access",
    role: "admin",
    // Still searchable, but not listed: it opens the access page, which is listed as itself.
    browse: false,
  }),
  destination({
    id: "people",
    label: "People",
    description: "Choose who can sign in, and what they can ask for.",
    aliases: ["users", "accounts", "invite", "members", "household", "admins"],
    card: "people",
    path: "/people",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "access",
    owner: "settings.access",
    label: "Sign-in, devices and sharing",
    description:
      "Manage sessions, single sign-on, paired devices, the sharing address, your location, secrets, and encryption.",
    aliases: ["security", "SSO", "login", "cookie", "API token", "pair device", "encryption"],
    card: "people",
    path: "/settings/access",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "playback",
    owner: "settings.playback",
    label: "Playback",
    description: "Choose how channels stream and how much live work this server can handle.",
    aliases: ["streaming", "transcode", "FFmpeg", "picture", "sound", "TV guide", "direct play"],
    card: "server",
    path: "/settings/system/playback",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "storage",
    owner: "settings.storage",
    label: "Artwork storage",
    description: "Choose where artwork lives and how much resized-image space it can use.",
    aliases: ["images", "disk space", "artwork folder", "cache", "uploads"],
    card: "server",
    path: "/settings/system/storage",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "backup",
    owner: "settings.backup",
    label: "Backups",
    description: "Choose when backups run, where they go, and how many to keep.",
    aliases: ["save my setup", "restore", "backup folder", "retention"],
    card: "server",
    path: "/settings/system/backup",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "tasks",
    owner: "settings.tasks",
    label: "Background tasks",
    description: "See scheduled work, change its timing, or run a task now.",
    aliases: ["jobs", "schedule", "cron", "maintenance", "run now"],
    card: "server",
    path: "/settings/system/tasks",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "diagnostics",
    owner: "settings.diagnostics",
    label: "Diagnostics",
    description: "Inspect recent failures and control how much troubleshooting data is kept.",
    aliases: ["logs", "errors", "process output", "debug", "troubleshoot"],
    card: "server",
    path: "/settings/system/diagnostics",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "database",
    label: "Database",
    description: "Check the active database or move this installation to PostgreSQL.",
    aliases: ["SQLite", "PostgreSQL", "migration", "move database"],
    card: "server",
    path: "/settings/system/database",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "about",
    label: "About Loomarr",
    description: "See the installed version and build information.",
    aliases: ["version", "build", "update"],
    card: "server",
    path: "/settings/system/about",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "advanced",
    owner: "settings.advanced",
    label: "Advanced settings",
    description: "Find and edit a setting by its exact key or environment variable.",
    aliases: ["all settings", "raw settings", "environment variables", "expert", "keys"],
    card: "advanced",
    path: "/settings/advanced",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "filler",
    label: "Filler settings",
    description: "Change how Loomarr finds, prepares, stores, and plays short clips.",
    aliases: ["commercials", "clips", "adverts", "interstitials"],
    card: "filler",
    path: "/filler/settings",
    role: "admin",
    browse: true,
  }),
  ...SETTINGS_SECTIONS.map((section) => {
    const owner = `filler.${section.id}`;
    return destination({
      id: owner,
      owner,
      label: section.label,
      description: section.description,
      aliases: section.aliases,
      card: "filler",
      path: `/filler/settings/${section.id}`,
      section: section.id,
      role: "admin",
      browse: true,
    });
  }),
] as const;

interface SettingsCard {
  id: Exclude<SettingsCardId, "advanced">;
  title: string;
  description: string;
}

// The cards in their resting order. Cards with a problem move ahead of the rest on the index.
const SETTINGS_CARDS: readonly SettingsCard[] = [
  {
    id: "connections",
    title: "Connections",
    description: "Your media server, download services and where your shows live.",
  },
  { id: "ai", title: "AI", description: "The service and models behind suggestions and clip checks." },
  { id: "channels", title: "Channels", description: "What every channel starts with unless you change it." },
  { id: "filler", title: "Filler", description: "How short clips between shows are found, kept and played." },
  {
    id: "people",
    title: "People and access",
    description: "Who can watch, how they sign in, and paired TVs.",
  },
  {
    id: "notifications",
    title: "Notifications",
    description: "How Loomarr tells you when something needs you.",
  },
  {
    id: "server",
    title: "Server",
    description: "Playback, storage, backups and what runs in the background.",
  },
];

const destinationForOwner = (owner: string): SettingsDestination | undefined =>
  SETTINGS_DESTINATIONS.find((item) => item.owner === owner);

const destinationById = (id: string): SettingsDestination | undefined =>
  SETTINGS_DESTINATIONS.find((item) => item.id === id);

const canVisitDestination = (item: SettingsDestination, isAdmin: boolean): boolean =>
  item.role === "all" || isAdmin;

// The pages a card lists for this viewer, in catalog order.
const cardDestinations = (card: SettingsCardId, isAdmin: boolean): SettingsDestination[] =>
  SETTINGS_DESTINATIONS.filter(
    (item) => item.card === card && item.browse && canVisitDestination(item, isAdmin),
  );

// The name search results file a destination under: its card's title.
const cardTitle = (card: SettingsCardId): string =>
  card === "advanced" ? "Advanced" : (SETTINGS_CARDS.find((item) => item.id === card)?.title ?? "Settings");

export type { SettingsCard, SettingsCardId, SettingsDestination, SettingsDestinationPath, SettingsRole };
export {
  canVisitDestination,
  cardDestinations,
  cardTitle,
  destinationById,
  destinationForOwner,
  SETTINGS_CARDS,
  SETTINGS_DESTINATIONS,
};
