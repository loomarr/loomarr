import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import { fillerSourceCountrySchema } from "@loomarr/core/schemas";
import { Action, Stack, Text } from "@loomarr/design-system";
import { useForm, useStore } from "@tanstack/react-form";
import { LocationPicker } from "@/components/loomarr/settings/installation-location";
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
    <Stack className="border-border border-t pt-3" gap="control">
      <Text as="p" textRole="compact" tone="secondary">
        Change this only when the source covers a different area from your location.
      </Text>
      <LocationPicker
        value={{ country: country || (source.effectiveCountry ?? "") }}
        onChange={(location) => form.setFieldValue("country", location.country)}
      />
      <Stack direction="row" gap="inline" wrap>
        <Action
          disabled={
            actions.updating ||
            !country.trim() ||
            country.trim().toUpperCase() === (source.country ?? "").toUpperCase()
          }
          onPress={() => void form.handleSubmit()}
          variant="outline"
        >
          Save different area
        </Action>
        {(country.trim() || source.locationSource === "source") && (
          <Action
            disabled={actions.updating}
            variant="ghost"
            onPress={() => {
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
          </Action>
        )}
      </Stack>
    </Stack>
  );
};

export { SourceLocationForm };
