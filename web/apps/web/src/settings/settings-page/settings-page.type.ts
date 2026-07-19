import type { SettingEntry } from "@loomarr/api";
import type { ReactNode } from "react";

// A page groups one or more registry groups behind a single save bar (config-design §5).
interface SettingsBlock {
  // The registry `group` value(s) this block renders.
  group: string;
  title: string;
  // The named connection check this block can run, when it has one (§8 per-block Test).
  check?: string;
}

interface SettingsPageProps {
  title: string;
  description?: string;
  blocks: SettingsBlock[];
  entries: SettingEntry[];
  // Rendered above the blocks — Connections puts the re-runnable checklist here (§5).
  children?: ReactNode;
}

export type { SettingsBlock, SettingsPageProps };
