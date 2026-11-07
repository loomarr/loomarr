import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import { fillerSourceCountrySchema } from "@loomarr/core/schemas";
import { useForm, useStore } from "@tanstack/react-form";
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
  const form = useForm({
    defaultValues: { country: source.country ?? "" },
    validators: { onSubmit: fillerSourceCountrySchema },
    onSubmit: ({ value }) =>
      actions.updateSource(source.id, {
        enabled: source.enabled,
        geography: { country: value.country.trim().toUpperCase() },
      }),
  });
  const country = useStore(form.store, (state) => state.values.country);
  const clear = () => form.setFieldValue("country", "");

  return (
    <div className="border-border flex flex-col gap-3 border-t pt-3">
      <p className="text-muted-foreground text-sm">
        Change this only when the source covers a different area from your location.
      </p>
      <LocationPicker
        value={{ country: country || (source.effectiveCountry ?? "") }}
        onChange={(location) => form.setFieldValue("country", location.country)}
      />
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={actions.updating || !country.trim()}
          onClick={() => void form.handleSubmit()}
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
                clear();
                return;
              }
              actions.updateSource(
                source.id,
                { enabled: source.enabled, geography: { country: "" } },
                { onSuccess: clear },
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
