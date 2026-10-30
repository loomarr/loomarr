import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import { useState } from "react";
import { LocationPicker } from "@/components/loomarr/settings/installation-location";
import { Button } from "@/components/ui/button";
import type { SourceActions } from "../use-source-actions";

interface SourceLocationFormProps {
  source: FillerSourceDTO;
  actions: Pick<SourceActions, "updateSource" | "updating">;
}

// Source-specific geography is an exception, not part of adding every source. Empty means follow the
// Installation geography; an explicit value is for a collection whose real coverage differs. The
// draft country is seeded from the source once, when the form mounts — remount per source (`key`).
const SourceLocationForm = ({ source, actions }: SourceLocationFormProps) => {
  const [country, setCountry] = useState(source.country ?? "");

  return (
    <div className="border-border flex flex-col gap-3 border-t pt-3">
      <p className="text-muted-foreground text-sm">
        Change this only when the source covers a different area from your location.
      </p>
      <LocationPicker
        value={{ country: country || (source.effectiveCountry ?? "") }}
        onChange={(location) => setCountry(location.country)}
      />
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={actions.updating || !country.trim()}
          onClick={() =>
            actions.updateSource(source.id, {
              enabled: source.enabled,
              geography: { country: country.trim().toUpperCase() },
            })
          }
        >
          Save different area
        </Button>
        {(country.trim() || source.locationSource === "source") && (
          <Button
            type="button"
            variant="ghost"
            disabled={actions.updating}
            onClick={() => {
              if (source.locationSource !== "source") {
                setCountry("");
                return;
              }
              actions.updateSource(
                source.id,
                { enabled: source.enabled, geography: { country: "" } },
                { onSuccess: () => setCountry("") },
              );
            }}
          >
            Use my location
          </Button>
        )}
      </div>
    </div>
  );
};

export { SourceLocationForm };
