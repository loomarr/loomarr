import type { SettingEntry } from "@loomarr/api";
import type { ReactNode } from "react";

// A page groups one or more registry groups behind a single save bar (config-design §5).
interface SettingsBlock {
  // The registry `group` value(s) this block renders.
  group: string;
  title: string;
  // One sentence explaining the decision this block contains. Setting docs explain individual
  // controls; this explains why the controls belong together.
  description?: string;
  // Optional ordered subset when one registry group has several user-facing sections.
  // The key remains declared once; this is only placement on the owning workflow page.
  keys?: string[];
  // The named connection check this block can run, when it has one (§8 per-block Test).
  check?: string;
  // Marks a connection the current install can leave disconnected. The status still reports
  // truthfully; the label explains that a failure need not become work for this operator.
  optional?: boolean;
  // A workflow prerequisite may make one field temporarily unavailable while retaining its
  // desired value. The reason is rendered beside and associated with the disabled control.
  disabledReasons?: Record<string, string>;
  // A note pinned to the bottom of this block's body, below its fields and its Test row — for
  // something about the SERVICE rather than about its settings. TMDB's attribution (§22).
  //
  // ⚠ Inside the collapsible panel, so it is hidden while the block is collapsed, and connection
  // blocks collapse when their check passes. Maintainer's call: the notice reads as belonging to
  // TMDB, which a page-level footer under four unrelated blocks did not.
  footer?: ReactNode;
}

interface SettingsPageProps {
  title: string;
  description?: string;
  blocks: SettingsBlock[];
  entries: SettingEntry[];
  // Rendered above the blocks — Connections puts the re-runnable checklist here (§5).
  children?: ReactNode;
  // Rendered below the blocks, for things that read as a consequence of the settings
  // above: the AI page's model picker only makes sense once a provider is chosen. A
  // render prop so the footer can react to LIVE edits — it receives `liveValue(key)`,
  // the current value of any key honoring unsaved edits (so the model picker hides the
  // instant the provider dropdown flips to OpenAI, not only after Save). A plain
  // ReactNode is still accepted for footers that need no live state.
  footer?:
    | ReactNode
    | ((ctx: {
        liveValue: (key: string) => string;
        setEdit: (key: string, value: string) => void;
      }) => ReactNode);
}

export type { SettingsBlock, SettingsPageProps };
