import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage, useSettingsEntries } from "@/settings";

// Settings → Defaults (config-design §5, V9) — what a NEW channel inherits, plus how filler
// behaves. This folds the old "Channels & playback" and "Filler" pages together, because they
// answer one question between them: an operator setting up a fresh install is deciding what
// their channels will be like, and having that split across two tabs meant checking both to
// know the answer.
//
// The filler CATALOG is still its own top-level page — browsing and tagging clips is a
// different job from setting the drop-folder path, and the description says so rather than
// leaving someone hunting.
const DefaultsSettings = () => (
  <SettingsPage
    title="Defaults"
    description="What a new channel inherits — ordering, how gaps are filled, and the programming policy — plus where your filler comes from and how densely breaks play. Browse and tag the clips themselves on the Filler page; each channel can override any of this on its own Programming section."
    entries={useSettingsEntries()}
    blocks={[
      { group: "channels", title: "Scheduling defaults" },
      { group: "filler", title: "Filler library", check: "filler" },
    ]}
  />
);

const Route = createFileRoute("/_authed/settings/defaults")({
  component: DefaultsSettings,
});

export { Route };
