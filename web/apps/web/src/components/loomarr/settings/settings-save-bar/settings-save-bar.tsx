import { Action, Stack, Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { SettingsSaveBarProps } from "./settings-save-bar.type";

// Sonarr's sticky save bar (config-design §5). Explicit per-page saving, chosen over
// per-field autosave because connection settings usually change TOGETHER — a URL and its
// token — and a half-saved pair caught mid-test is a footgun that looks like a broken
// integration. The bar only appears once something is dirty, so a page being read rather
// than edited stays quiet.
const SettingsSaveBar = ({
  dirtyCount,
  onSave,
  onDiscard,
  saving = false,
  className,
}: SettingsSaveBarProps) => {
  if (dirtyCount === 0) return null;

  return (
    <Stack
      align="center"
      aria-label="Unsaved changes"
      className={cn("border-border bg-card sticky bottom-0 border-t px-6 py-3", className)}
      direction="row"
      gap="control"
      render="section"
    >
      <Text as="p" className="flex-1" textRole="compact">
        {dirtyCount} unsaved {dirtyCount === 1 ? "change" : "changes"}
      </Text>
      <Action disabled={saving} onPress={onDiscard} variant="ghost">
        Discard
      </Action>
      <Action disabled={saving} onPress={onSave}>
        {saving ? "Saving…" : "Save changes"}
      </Action>
    </Stack>
  );
};

export { SettingsSaveBar };
