import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { AddFillerSourceInputBodyKind } from "@loomarr/api/models/addFillerSourceInputBodyKind";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
// ⚠ No `Card` and no `Label`. The local add flow lives directly inside Your files, and its fields
// are labelled by their per-kind PLACEHOLDER plus `aria-label` — a static visible label above an
// input whose meaning changes with the kind would contradict it.
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

// Per-kind copy for "Add a source" (the mock's `newSourcePlaceholder`).
//
// ⚠ One table rather than nested ternaries at the two call sites. With two kinds a ternary was
// readable; with four it is a bug waiting to happen — and the two fields must stay in step, since
// a label promising a path beside a placeholder showing a URL is worse than either alone.
//
// ⚠ The FOLDER copy says "full path" deliberately. The server refuses a relative path (it would
// resolve against Loomarr's working directory, which differs between a container and a `go run`),
// so saying so up front is cheaper than a 400 that arrives after the operator has typed.
const SOURCE_KIND_COPY = {
  folder: { label: "Folder path", placeholder: "/data/filler — a full path Loomarr can read" },
  library: { label: "Library name", placeholder: "Commercials — as it appears on your media server" },
} as const satisfies Partial<Record<AddFillerSourceInputBodyKind, { label: string; placeholder: string }>>;

// The kinds this form adds: a key above that the API does not accept fails the build.
type LocalSourceKind = keyof typeof SOURCE_KIND_COPY;

// "Add a source" (V37), local kinds only — remote sources arrive through the source finder (#1255).
// The kind is explicit rather than sniffed from the URI. ⚠ The state type is SOURCE_KIND_COPY's keys,
// which is what the dropdown renders, so an option the state cannot hold is a compile error. It
// defaults to `library` because that is the mock's first option, and the first option is what an
// operator sees before touching anything.
const LocalSourceForm = () => {
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<LocalSourceKind>("library");
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

  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm font-medium">Add a folder or library</p>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          if (!uri.trim()) return;
          addSource.mutate({ data: { kind, uri: uri.trim() } });
        }}
      >
        <Select value={kind} onValueChange={(value) => setKind(value as LocalSourceKind)}>
          <SelectTrigger id="new-source-kind" className="w-45" aria-label="Kind of source to add">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="library">Media server library</SelectItem>
            <SelectItem value="folder">Watched folder</SelectItem>
          </SelectContent>
        </Select>
        <Input
          id="new-source-uri"
          className="min-w-64 flex-1 font-mono"
          value={uri}
          placeholder={SOURCE_KIND_COPY[kind].placeholder}
          aria-label={SOURCE_KIND_COPY[kind].label}
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
