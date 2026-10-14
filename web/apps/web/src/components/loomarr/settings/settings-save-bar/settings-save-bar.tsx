import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
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
    <section
      aria-label="Unsaved changes"
      className={cn(
        "sticky bottom-0 flex items-center gap-3 border-border border-t bg-card px-6 py-3",
        className,
      )}
    >
      <p className="flex-1 text-sm">
        {dirtyCount} unsaved {dirtyCount === 1 ? "change" : "changes"}
      </p>
      <Button variant="ghost" onClick={onDiscard} disabled={saving}>
        Discard
      </Button>
      <Button onClick={onSave} disabled={saving}>
        {saving && <Loader2 className="animate-spin" aria-hidden />}
        {saving ? "Saving…" : "Save changes"}
      </Button>
    </section>
  );
};

export { SettingsSaveBar };
