import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Disclosure } from "@/components/ui/disclosure";
import { AutomaticDownloadsForm } from "../automatic-downloads-form";
import { SourceLocationForm } from "../source-location-form";
import type { SourceActions } from "../use-source-actions";

interface SourceSettingsProps {
  source: FillerSourceDTO;
  actions: SourceActions;
}

// "Source settings" in the source drawer: automatic downloads, a source-specific area, and
// removal — each shown only when the source offers it. Renders nothing when it offers none.
const SourceSettings = ({ source, actions }: SourceSettingsProps) => {
  const [open, setOpen] = useState(false);

  if (
    !(
      source.automaticDownloads ||
      source.actions.includes("edit_location") ||
      source.actions.includes("remove")
    )
  ) {
    return null;
  }

  return (
    <Disclosure open={open} onOpenChange={setOpen} className="border-border border-t pt-5">
      <Disclosure.SectionTrigger
        title="Source settings"
        description="Automatic downloads, location, and removal"
      />
      <Disclosure.Panel className="pt-4">
        <div className="flex flex-col gap-3">
          {source.automaticDownloads && <AutomaticDownloadsForm source={source} actions={actions} />}

          {source.actions.includes("edit_location") && (
            <SourceLocationForm source={source} actions={actions} />
          )}

          {source.actions.includes("remove") && (
            <div className="border-border border-t pt-3">
              <Button
                type="button"
                variant="ghost"
                disabled={actions.removing}
                onClick={() => actions.removeSource(source.id)}
                aria-label={`Remove ${source.target}`}
              >
                {actions.removingSourceID === source.id ? "Removing…" : "Remove source"}
              </Button>
              <p className="text-muted-foreground mt-1 text-xs">
                Clips already downloaded stay in your library.
              </p>
            </div>
          )}
        </div>
      </Disclosure.Panel>
    </Disclosure>
  );
};

export { SourceSettings };
