import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import {
  SETTINGS_DESTINATIONS,
  type SettingsDestination,
  type SettingsDestinationPath,
  type SettingsTaskGroup,
} from "@/settings/settings-destinations";

type SystemDestinationPath = Extract<SettingsDestinationPath, `/settings/system/${string}`>;
type SystemDestination = SettingsDestination & { path: SystemDestinationPath; section?: never };

const isSystemDestination = (item: SettingsDestination): item is SystemDestination =>
  item.path.startsWith("/settings/system/");

const GROUPS: readonly { id: SettingsTaskGroup; label: string; description: string }[] = [
  {
    id: "server",
    label: "Run this server",
    description: "Playback, artwork storage, and backups.",
  },
  {
    id: "troubleshoot",
    label: "Troubleshoot",
    description: "Background work, diagnostics, database tools, and version details.",
  },
];

const SystemHome = () => (
  <div className="flex h-full min-h-0 flex-col">
    <PageHeader
      title="This server"
      description="Keep playback running, protect this installation, or investigate a problem."
    />
    <div className="min-h-0 flex-1 overflow-auto">
      <div className="mx-auto max-w-5xl space-y-8 p-6">
        {GROUPS.map((group) => {
          const destinations = SETTINGS_DESTINATIONS.filter(
            (item): item is SystemDestination => item.group === group.id && isSystemDestination(item),
          );
          return (
            <section key={group.id} aria-labelledby={`system-group-${group.id}`}>
              <div className="mb-3">
                <h2 id={`system-group-${group.id}`} className="font-medium text-base">
                  {group.label}
                </h2>
                <p className="mt-0.5 text-muted-foreground text-sm">{group.description}</p>
              </div>
              <div className="grid gap-2 md:grid-cols-2">
                {destinations.map((item) => (
                  <Link
                    key={item.id}
                    to={item.path}
                    className="group flex min-w-0 items-start gap-3 rounded-lg border border-border bg-card px-4 py-3 transition-colors hover:border-signal/40 hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block font-medium text-sm">{item.label}</span>
                      <span className="mt-0.5 block text-muted-foreground text-xs leading-relaxed">
                        {item.description}
                      </span>
                    </span>
                    <ArrowRight
                      className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-foreground"
                      aria-hidden
                    />
                  </Link>
                ))}
              </div>
            </section>
          );
        })}
      </div>
    </div>
  </div>
);

const Route = createFileRoute("/_authed/settings/system/")({
  component: SystemHome,
});

export { Route };
