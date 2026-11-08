import * as fillerApi from "@loomarr/api/endpoints/filler";
import { addFillerSourceSchema } from "@loomarr/core/schemas";
import { Field, Surface, Text } from "@loomarr/design-system";
import { useForm, useStore } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
// ⚠ No `Card` and no `Label`. The local add flow lives directly inside Your files, and its fields
// are labelled by the provider's own address label plus `aria-label` — a static visible label above
// an input whose meaning changes with the kind would contradict it.
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useFillerProviders } from "../../use-filler-providers";

// "Add a source" (V37), for the providers whose sources are typed addresses — a folder, a media
// library — rather than found through a finder (#1255). The kinds, their names, the field's label and
// its example all come from the provider registry, so a provider added there appears here with no
// client change; the field's label and example move together because a label promising a path beside
// a placeholder showing a URL is worse than either alone.
//
// ⚠ The folder's example says "full path" deliberately. The server refuses a relative path (it would
// resolve against Loomarr's working directory, which differs between a container and a `go run`),
// so saying so up front is cheaper than a 400 that arrives after the operator has typed.
const LocalSourceForm = () => {
  const queryClient = useQueryClient();
  const { typed } = useFillerProviders();
  const addSource = fillerApi.useAddFillerSource({
    mutation: {
      onSuccess: () => {
        form.setFieldValue("uri", "");
        toast.success("Source added", { description: "Loomarr will check it on its download schedule." });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });
      },
    },
  });
  // Until the first kind is chosen the form offers the registry's first.
  const providerFor = (kind: string) => typed.find((candidate) => candidate.id === kind) ?? typed[0];
  const form = useForm({
    defaultValues: { kind: "", uri: "" },
    validators: { onSubmit: addFillerSourceSchema },
    onSubmit: ({ value }) => {
      const kind = providerFor(value.kind)?.id;
      if (kind) addSource.mutate({ data: { kind, uri: value.uri.trim() } });
    },
  });
  const provider = providerFor(useStore(form.store, (state) => state.values.kind));
  if (!provider) return null;

  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm font-medium">Add a folder or library</p>
      <form
        className="flex flex-wrap items-center gap-2"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void form.handleSubmit();
        }}
      >
        <form.Field name="kind">
          {(field) => (
            <Select value={provider.id} onValueChange={(kind) => field.handleChange(kind)}>
              <SelectTrigger id="new-source-kind" className="w-45" aria-label="Kind of source to add">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {typed.map((candidate) => (
                  <SelectItem key={candidate.id} value={candidate.id}>
                    {candidate.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </form.Field>
        <form.Field name="uri">
          {(field) => (
            <Surface backgroundColor="transparent" borderWidth={0} flexBasis={256} flexGrow={1} minWidth={0}>
              <Field
                accessibilityLabel={provider.addressLabel}
                autoCapitalize="none"
                autoCorrect={false}
                id="new-source-uri"
                onBlur={field.handleBlur}
                onChangeText={field.handleChange}
                placeholder={provider.addressHint}
                value={field.state.value}
              />
            </Surface>
          )}
        </form.Field>
        <form.Subscribe selector={(state) => state.values.uri}>
          {(uri) => (
            <Button type="submit" variant="outline" disabled={addSource.isPending || !uri.trim()}>
              {addSource.isPending ? "Adding…" : "+ Add source"}
            </Button>
          )}
        </form.Subscribe>
      </form>
      {addSource.error != null && <ErrorState error={addSource.error} />}
      <Text as="span" textRole="captionMono">
        Uses your location. Every clip is checked before it can play.
      </Text>
    </div>
  );
};

export { LocalSourceForm };
