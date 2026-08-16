import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

// Settings → Defaults (config-design §5) — only settings that are ACTUALLY consulted as a
// per-channel fallback. A curated page is a promise about ownership, so operational knobs and
// global caps stay with their owning workflow even though All settings still exposes every key.
const DefaultsSettings = () => (
  <SettingsPage
    title="Channel defaults"
    description="Used by every channel that follows the default. Changes affect existing channels that still inherit them; channel-specific choices stay unchanged."
    entries={useSettingsEntries()}
    blocks={[
      {
        group: "channels",
        title: "Schedule horizon",
        description:
          "How much programming Loomarr keeps ready. A channel or scheduling rule can choose a different horizon.",
        keys: ["sched.window_hours"],
      },
      {
        group: "filler",
        title: "Commercial breaks",
        description:
          "The starting break frequency for channels. Each channel's Filler page can follow this, turn breaks off, or choose a custom frequency.",
        keys: ["filler.breaks_per_hour"],
      },
    ]}
  />
);

const Route = createFileRoute("/_authed/settings/defaults")({
  component: DefaultsSettings,
});

export { Route };
