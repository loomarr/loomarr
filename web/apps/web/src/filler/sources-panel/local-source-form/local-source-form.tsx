import * as fillerApi from "@loomarr/api/endpoints/filler";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
// ⚠ No `Card` and no `Label`. The local add flow lives directly inside Your files, and its fields
// are labelled by the provider's own address label plus `aria-label` — a static visible label above
// an input whose meaning changes with the kind would contradict it.
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";
import { Input } from "@/components/ui/input";
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
  const [chosen, setChosen] = useState<string>();
  const [uri, setURI] = useState("");
  const addSource = fillerApi.useAddFillerSource({
    mutation: {
      onSuccess: () => {
        setURI("");
        toast.success("Source added", { description: "Loomarr will check it on its download schedule." });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });
      },
    },
  });
  // Until the first kind is chosen the form offers the registry's first.
  const provider = typed.find((candidate) => candidate.id === chosen) ?? typed[0];
  if (!provider) return null;

  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm font-medium">Add a folder or library</p>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          if (!uri.trim()) return;
          addSource.mutate({ data: { kind: provider.id, uri: uri.trim() } });
        }}
      >
        <Select value={provider.id} onValueChange={setChosen}>
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
        <Input
          id="new-source-uri"
          className="min-w-64 flex-1 font-mono"
          value={uri}
          placeholder={provider.addressHint}
          aria-label={provider.addressLabel}
          onChange={(event) => setURI(event.target.value)}
        />
        <Button type="submit" variant="outline" disabled={addSource.isPending || !uri.trim()}>
          {addSource.isPending ? "Adding…" : "+ Add source"}
        </Button>
      </form>
      {addSource.error != null && <ErrorState error={addSource.error} />}
      <Caption>Uses your location. Every clip is checked before it can play.</Caption>
    </div>
  );
};

export { LocalSourceForm };
