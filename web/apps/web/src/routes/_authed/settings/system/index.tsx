import { Icon, icons, Stack, Text } from "@loomarr/design-system";
import { createFileRoute, Link } from "@tanstack/react-router";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import {
  SETTINGS_DESTINATIONS,
  type SettingsDestination,
  type SettingsDestinationPath,
} from "@/settings/settings-destinations";

type SystemDestinationPath = Extract<SettingsDestinationPath, `/settings/system/${string}`>;
type SystemDestination = SettingsDestination & { path: SystemDestinationPath; section?: never };

const isSystemDestination = (item: SettingsDestination): item is SystemDestination =>
  item.path.startsWith("/settings/system/");

// The Server card's pages, split for this landing into running the server and looking into it.
const GROUPS: readonly { id: string; label: string; description: string; pages: readonly string[] }[] = [
  {
    id: "server",
    label: "Run this server",
    description: "Playback, artwork storage, and backups.",
    pages: ["playback", "storage", "backup"],
  },
  {
    id: "troubleshoot",
    label: "Troubleshoot",
    description: "Background work, diagnostics, database tools, and version details.",
    pages: ["tasks", "diagnostics", "database", "about"],
  },
];

const SystemHome = () => (
  <Stack className="h-full min-h-0">
    <PageHeader
      title="This server"
      description="Keep playback running, protect this installation, or investigate a problem."
    />
    <div className="min-h-0 flex-1 overflow-auto">
      <div className="mx-auto max-w-5xl space-y-8 p-6">
        {GROUPS.map((group) => {
          const destinations = SETTINGS_DESTINATIONS.filter(
            (item): item is SystemDestination => group.pages.includes(item.id) && isSystemDestination(item),
          );
          return (
            <section key={group.id} aria-labelledby={`system-group-${group.id}`}>
              <div className="mb-3">
                <Text as="h2" id={`system-group-${group.id}`} textRole="heading">
                  {group.label}
                </Text>
                <Text as="p" className="mt-0.5" textRole="compact" tone="secondary">
                  {group.description}
                </Text>
              </div>
              <div className="grid gap-2 md:grid-cols-2">
                {destinations.map((item) => (
                  <Link
                    key={item.id}
                    to={item.path}
                    className="group border-border bg-card hover:border-signal/40 hover:bg-muted/40 focus-visible:ring-ring flex min-w-0 items-start gap-3 rounded-lg border px-4 py-3 transition-colors focus-visible:ring-2 focus-visible:outline-none"
                  >
                    <span className="min-w-0 flex-1">
                      <Text as="span" className="block" textRole="cardLabel">
                        {item.label}
                      </Text>
                      <Text as="span" className="mt-0.5 block" textRole="hint" tone="secondary">
                        {item.description}
                      </Text>
                    </span>
                    <Stack
                      className="text-muted-foreground group-hover:text-foreground mt-0.5 transition-colors"
                      direction="row"
                      render="span"
                    >
                      <Icon decorative glyph={icons.goForward} size="compact" tone="inherit" />
                    </Stack>
                  </Link>
                ))}
              </div>
            </section>
          );
        })}
      </div>
    </div>
  </Stack>
);

// no-loader: a static list of links to the System pages
const Route = createFileRoute("/_authed/settings/system/")({
  component: SystemHome,
});

export { Route };
