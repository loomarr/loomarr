import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerSourceSuggestionDTO } from "@loomarr/api/models/fillerSourceSuggestionDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { SourceContentPreview } from "../source-content-preview";

interface ProviderSourceFinderProps {
  kind: "archive" | "youtube";
  enabled: boolean;
}

const COPY = {
  archive: {
    label: "Find an Archive.org collection",
    placeholder: "Search collections or paste an archive.org URL",
    check: "Check source",
    add: "Add collection",
  },
  youtube: {
    label: "Find a YouTube channel or playlist",
    placeholder: "Search channels or paste a channel or playlist URL",
    check: "Check source",
    add: "Add source",
  },
} as const;

const isExactProviderInput = (kind: ProviderSourceFinderProps["kind"], value: string) => {
  if (kind === "archive") {
    return (
      /^(https?:\/\/)?(www\.)?archive\.org\/(details|metadata)\//i.test(value) ||
      /^[a-z0-9][a-z0-9_-]*[_-][a-z0-9_-]+$/i.test(value)
    );
  }
  return (
    /^(https?:\/\/)?((www|m|music)\.)?(youtube\.com|youtu\.be)\//i.test(value) ||
    /^@[a-z0-9._-]+$/i.test(value) ||
    /^UC[a-z0-9_-]{10,}$/i.test(value) ||
    /^(PL|UU|LL|FL|RD|OLAK5uy_)[a-z0-9_-]{8,}$/i.test(value)
  );
};

// One search-or-paste field for one provider. Search and exact resolution are deliberately
// read-only; the separate final button is the only action that registers background work.
const ProviderSourceFinder = ({ kind, enabled }: ProviderSourceFinderProps) => {
  const copy = COPY[kind];
  const minimumQueryLength = kind === "youtube" ? 3 : 2;
  const listID = useId();
  const queryClient = useQueryClient();
  const [input, setInput] = useState("");
  const [debounced, setDebounced] = useState("");
  const [active, setActive] = useState(-1);
  const [selected, setSelected] = useState<FillerSourceSuggestionDTO>();
  const [verified, setVerified] = useState(false);
  const [lastExactInput, setLastExactInput] = useState("");

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(input.trim()), kind === "youtube" ? 600 : 400);
    return () => window.clearTimeout(timer);
  }, [input, kind]);

  const exactProviderInput = isExactProviderInput(kind, debounced);
  const exactInput = isExactProviderInput(kind, input.trim());
  const suggestionsQuery = fillerApi.useSuggestFillerSources(
    kind,
    { q: debounced, limit: 8 },
    {
      query: {
        enabled: enabled && !selected && !exactProviderInput && debounced.length >= minimumQueryLength,
        placeholderData: (previous) => previous,
      },
    },
  );
  const suggestions = unwrap(suggestionsQuery.data, (body) => body.suggestions) ?? [];

  const resolveSource = fillerApi.useResolveFillerSource({
    mutation: {
      onSuccess: (response) => {
        const result = unwrap(response, (body) => body);
        if (result) {
          setSelected(result);
          setVerified(true);
          setInput(result.title);
        }
      },
      onError: () => {
        setSelected(undefined);
        setVerified(false);
      },
    },
  });
  useEffect(() => {
    if (!enabled || selected || !exactProviderInput || debounced === lastExactInput) return;
    setLastExactInput(debounced);
    resolveSource.mutate({ kind, data: { input: debounced } });
  }, [debounced, enabled, exactProviderInput, kind, lastExactInput, resolveSource, selected]);
  const addSource = fillerApi.useAddFillerSource({
    mutation: {
      onSuccess: () => {
        toast.success("Source added", { description: "Loomarr will check it automatically." });
        setInput("");
        setDebounced("");
        setSelected(undefined);
        setVerified(false);
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerSourcesQueryKey() });
      },
    },
  });

  const choose = (suggestion: FillerSourceSuggestionDTO) => {
    setSelected(suggestion);
    setVerified(false);
    setInput(suggestion.title);
    setActive(-1);
    resolveSource.mutate({ kind, data: { input: suggestion.canonicalUrl } });
  };

  const submit = () => {
    if (!enabled || !input.trim()) return;
    if (!selected) {
      const value = input.trim();
      if (isExactProviderInput(kind, value)) {
        resolveSource.mutate({ kind, data: { input: value } });
      } else if (debounced === value) {
        void suggestionsQuery.refetch();
      } else {
        setDebounced(value);
      }
      return;
    }
    if (!verified || selected.alreadyAdded) return;
    addSource.mutate({
      data: {
        kind,
        uri: kind === "archive" ? selected.canonicalId : selected.canonicalUrl,
        label: selected.title,
      },
    });
  };

  const showSuggestions = enabled && !selected && !exactInput && input.trim().length >= minimumQueryLength;

  const targetLabel = (suggestion: FillerSourceSuggestionDTO) => {
    if (suggestion.itemCount) return `${suggestion.itemCount.toLocaleString()} items`;
    if (suggestion.targetType === "channel") return "Channel";
    if (suggestion.targetType === "playlist") return "Playlist";
    return "Collection";
  };

  return (
    <div className="mt-3 max-w-2xl">
      <form
        className="flex flex-col gap-2 sm:flex-row"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <Input
          role="combobox"
          aria-label={copy.label}
          aria-autocomplete="list"
          aria-controls={listID}
          aria-expanded={showSuggestions && suggestions.length > 0}
          aria-activedescendant={active >= 0 ? `${listID}-${active}` : undefined}
          autoComplete="off"
          className="min-w-0 flex-1"
          placeholder={copy.placeholder}
          value={input}
          disabled={!enabled}
          onChange={(event) => {
            setInput(event.target.value);
            setSelected(undefined);
            setVerified(false);
            setLastExactInput("");
            setActive(-1);
          }}
          onKeyDown={(event) => {
            if (!showSuggestions || suggestions.length === 0) return;
            if (event.key === "ArrowDown") {
              event.preventDefault();
              setActive((current) => Math.min(current + 1, suggestions.length - 1));
            } else if (event.key === "ArrowUp") {
              event.preventDefault();
              setActive((current) => Math.max(current - 1, 0));
            } else if (event.key === "Escape") {
              setActive(-1);
            } else if (event.key === "Enter" && active >= 0) {
              event.preventDefault();
              const choice = suggestions[active];
              if (choice) choose(choice);
            }
          }}
        />
        <Button
          type="submit"
          variant={selected ? "default" : "outline"}
          className="transition-none"
          disabled={
            !enabled ||
            !input.trim() ||
            resolveSource.isPending ||
            addSource.isPending ||
            (!selected && !exactInput && suggestionsQuery.isFetching) ||
            selected?.alreadyAdded ||
            Boolean(selected && !verified)
          }
        >
          {resolveSource.isPending
            ? "Checking…"
            : addSource.isPending
              ? "Adding…"
              : !selected && !exactInput && suggestionsQuery.isFetching
                ? "Searching…"
                : selected
                  ? copy.add
                  : exactInput
                    ? copy.check
                    : "Search"}
        </Button>
      </form>

      {!enabled && (
        <p className="mt-2 text-muted-foreground text-xs">Resume this service to find a source.</p>
      )}

      {showSuggestions && (
        <div
          id={listID}
          role="listbox"
          aria-label={`${copy.label} results`}
          className="mt-1 max-h-72 w-full overflow-y-auto rounded-lg border border-border bg-popover p-1"
        >
          {suggestionsQuery.isFetching && suggestions.length === 0 && (
            <p className="px-3 py-2 text-muted-foreground text-sm">Searching…</p>
          )}
          {!suggestionsQuery.isFetching &&
            suggestions.length === 0 &&
            debounced.length >= minimumQueryLength && (
              <p className="px-3 py-2 text-muted-foreground text-sm">
                {kind === "archive"
                  ? "No collections found. Paste the exact URL to check it."
                  : "No channels found. Paste a channel or playlist URL to check it."}
              </p>
            )}
          {suggestions.map((suggestion, index) => (
            <button
              id={`${listID}-${index}`}
              key={`${suggestion.provider}:${suggestion.canonicalId}`}
              type="button"
              role="option"
              aria-selected={active === index}
              className={cn(
                "flex w-full items-start justify-between gap-4 rounded-md px-3 py-2 text-left",
                active === index ? "bg-accent" : "hover:bg-accent/60",
              )}
              onMouseEnter={() => setActive(index)}
              onClick={() => choose(suggestion)}
            >
              <span className="min-w-0">
                <span className="block truncate font-medium text-sm">{suggestion.title}</span>
                <span className="block truncate text-muted-foreground text-xs">
                  {suggestion.description || suggestion.canonicalId}
                </span>
              </span>
              <span className="shrink-0 text-muted-foreground text-xs">
                {suggestion.alreadyAdded ? "Added" : targetLabel(suggestion)}
              </span>
            </button>
          ))}
        </div>
      )}

      {selected && (
        <div className="mt-2 rounded-lg bg-muted/35 px-3 py-2">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <p className="truncate font-medium text-sm">{selected.title}</p>
            </div>
            <span className="shrink-0 text-muted-foreground text-xs">
              {selected.alreadyAdded ? "Already added" : targetLabel(selected)}
            </span>
          </div>
          <SourceContentPreview
            kind={kind}
            canonicalUrl={selected.canonicalUrl}
            previewItems={verified ? selected.previewItems : undefined}
            loading={resolveSource.isPending && !verified}
          />
          {verified &&
            !selected.alreadyAdded &&
            (!selected.previewItems || selected.previewItems.length === 0) && (
              <p className="mt-2 text-muted-foreground text-xs">
                Loomarr checks a few new items at a time, and reviews every clip before it can play.
              </p>
            )}
        </div>
      )}

      {(suggestionsQuery.error || resolveSource.error || addSource.error) && (
        <p className="mt-2 text-onair-300 text-sm">
          {suggestionsQuery.error?.detail ??
            resolveSource.error?.detail ??
            addSource.error?.detail ??
            `${kind === "archive" ? "Archive.org" : "YouTube"} is not responding right now. Try again in a moment.`}
        </p>
      )}
    </div>
  );
};

export { ProviderSourceFinder };
