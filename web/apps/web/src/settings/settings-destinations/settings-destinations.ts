type SettingsTaskGroup = "setup" | "access" | "server" | "troubleshoot" | "filler" | "advanced";
type SettingsRole = "all" | "admin";
type FillerSettingsSection =
  | "folders"
  | "downloads"
  | "storage"
  | "incoming"
  | "breaks"
  | "review"
  | "playback"
  | "limits"
  | "tools";

type SettingsStaticDestinationPath =
  | "/settings/general"
  | "/settings/connections"
  | "/settings/ai"
  | "/settings/defaults"
  | "/settings/notifications"
  | "/settings/security"
  | "/settings/system/playback"
  | "/settings/system/storage"
  | "/settings/system/backup"
  | "/settings/system/tasks"
  | "/settings/system/diagnostics"
  | "/settings/system/database"
  | "/settings/system/about"
  | "/settings/all"
  | "/filler/settings";

type SettingsDestinationPath = SettingsStaticDestinationPath | `/filler/settings/${FillerSettingsSection}`;

interface SettingsDestinationBase {
  id: string;
  owner?: string;
  label: string;
  description: string;
  aliases: readonly string[];
  group: SettingsTaskGroup;
  role: SettingsRole;
  browse: boolean;
}

type SettingsDestination = SettingsDestinationBase &
  (
    | { path: SettingsStaticDestinationPath; section?: never }
    | { path: `/filler/settings/${FillerSettingsSection}`; section: FillerSettingsSection }
  );

const destination = (value: SettingsDestination): SettingsDestination => value;

// One catalog owns Settings wording and browser destinations. Backend owner ids stay stable while
// paths and labels can evolve here; action-only destinations simply omit owner.
const SETTINGS_DESTINATIONS = [
  destination({
    id: "connections",
    owner: "settings.connections",
    label: "Connections",
    description: "Connect your media server, download services, Tunarr, and TMDB.",
    aliases: ["Emby", "Jellyfin", "Seerr", "Sonarr", "Radarr", "Tunarr", "TMDB", "services"],
    group: "setup",
    path: "/settings/connections",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "ai",
    owner: "settings.ai",
    label: "AI setup",
    description: "Choose the service and models Loomarr uses for suggestions and clip analysis.",
    aliases: ["OpenRouter", "OpenAI", "Ollama", "models", "suggestions", "vision", "transcription"],
    group: "setup",
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
    group: "setup",
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
    group: "setup",
    path: "/settings/notifications",
    role: "all",
    browse: true,
  }),
  destination({
    id: "location",
    owner: "settings.location",
    label: "Your location",
    description: "Set the country and local area Loomarr uses for relevant content.",
    aliases: ["where I live", "country", "city", "area", "market", "geography"],
    group: "setup",
    path: "/settings/general",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "sharing",
    owner: "settings.sharing",
    label: "Sharing address",
    description: "Set the Loomarr address people can open from invitations and recovery links.",
    aliases: ["public URL", "invitation link", "recovery link", "recipient address", "share"],
    group: "access",
    path: "/settings/general",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "access",
    owner: "settings.access",
    label: "Sign-in and devices",
    description: "Manage sessions, single sign-on, paired devices, secrets, and encryption.",
    aliases: ["security", "SSO", "login", "cookie", "API token", "pair device", "encryption"],
    group: "access",
    path: "/settings/security",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "playback",
    owner: "settings.playback",
    label: "Playback",
    description: "Choose how channels stream and how much live work this server can handle.",
    aliases: ["streaming", "transcode", "FFmpeg", "picture", "sound", "TV guide", "direct play"],
    group: "server",
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
    group: "server",
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
    group: "server",
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
    group: "troubleshoot",
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
    group: "troubleshoot",
    path: "/settings/system/diagnostics",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "database",
    label: "Database",
    description: "Check the active database or move this installation to PostgreSQL.",
    aliases: ["SQLite", "PostgreSQL", "migration", "move database"],
    group: "troubleshoot",
    path: "/settings/system/database",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "about",
    label: "About Loomarr",
    description: "See the installed version and build information.",
    aliases: ["version", "build", "update"],
    group: "troubleshoot",
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
    group: "advanced",
    path: "/settings/all",
    role: "admin",
    browse: true,
  }),
  destination({
    id: "filler",
    label: "Filler settings",
    description: "Change how Loomarr finds, prepares, stores, and plays short clips.",
    aliases: ["commercials", "clips", "adverts", "interstitials"],
    group: "filler",
    path: "/filler/settings",
    role: "admin",
    browse: true,
  }),
  ...(
    [
      [
        "filler.folders",
        "Clip folders",
        "Choose where clips live and how dropped files enter Loomarr.",
        ["drop folder", "watch folder", "clip library"],
        "folders",
      ],
      [
        "filler.downloads",
        "Automatic downloads",
        "Choose how often sources are checked and how many clips they add.",
        ["fetch schedule", "check sources", "download clips"],
        "downloads",
      ],
      [
        "filler.storage",
        "Filler storage limits",
        "Stop automatic downloads before clips fill this drive.",
        ["disk full", "stop clips filling disk", "clip limit", "catalog limit", "drive capacity"],
        "storage",
      ],
      [
        "filler.incoming",
        "Incoming history",
        "Choose how long ready clips remain visible in Incoming.",
        ["recent clips", "ready window", "24 hours"],
        "incoming",
      ],
      [
        "filler.breaks",
        "Break assembly",
        "Choose the usual length and number of clips in a commercial break.",
        ["commercial length", "clips per break", "ad break"],
        "breaks",
      ],
      [
        "filler.review",
        "Clip review",
        "Choose which automatic checks can identify and split incoming clips.",
        ["AI tagging", "vision", "transcribe", "auto split"],
        "review",
      ],
      [
        "filler.playback",
        "Clip eligibility and sound",
        "Choose which clips may play, how often they repeat, and their sound level.",
        ["language", "loudness", "quality", "repeat cooldown", "duration"],
        "playback",
      ],
      [
        "filler.limits",
        "Processing limits",
        "Limit how much clip preparation Loomarr performs in one pass.",
        ["pipeline", "background processing", "CPU", "GPU", "per pass"],
        "limits",
      ],
      [
        "filler.tools",
        "Processing tools",
        "Set executable and model paths for unusual installations.",
        ["yt-dlp", "FFmpeg", "whisper", "binary paths"],
        "tools",
      ],
    ] as const
  ).map(([owner, label, description, aliases, section]) =>
    destination({
      id: owner,
      owner,
      label,
      description,
      aliases,
      group: "filler",
      path: `/filler/settings/${section}`,
      section,
      role: "admin",
      browse: false,
    }),
  ),
] as const;

const SETTINGS_TASK_GROUPS: readonly { id: SettingsTaskGroup; label: string; description: string }[] = [
  { id: "setup", label: "Set up Loomarr", description: "Services and everyday defaults" },
  { id: "access", label: "Access and devices", description: "Sharing, sign-in, and trusted devices" },
  { id: "server", label: "This server", description: "Playback, storage, and backups" },
  { id: "troubleshoot", label: "Troubleshoot", description: "Background work and technical details" },
  { id: "filler", label: "Filler", description: "How short clips are found, prepared, and played" },
];

const destinationForOwner = (owner: string): SettingsDestination | undefined =>
  SETTINGS_DESTINATIONS.find((item) => item.owner === owner);

const canVisitDestination = (item: SettingsDestination, isAdmin: boolean): boolean =>
  item.role === "all" || isAdmin;

export type {
  FillerSettingsSection,
  SettingsDestination,
  SettingsDestinationPath,
  SettingsRole,
  SettingsTaskGroup,
};
export { canVisitDestination, destinationForOwner, SETTINGS_DESTINATIONS, SETTINGS_TASK_GROUPS };
