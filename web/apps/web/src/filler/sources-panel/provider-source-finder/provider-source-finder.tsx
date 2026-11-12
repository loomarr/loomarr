import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import type { FillerSourceSuggestionDTO } from "@loomarr/api/models/fillerSourceSuggestionDTO";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, Combobox, Stack, Text } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { toast } from "sonner";
import { SourceContentPreview } from "../source-content-preview";

interface ProviderSourceFinderProps {
  /** The provider to find a source for. Every word on the finder comes from its description. */
  provider: FillerProviderDescriptionDTO;
  enabled: boolean;
}

// How long typing pauses before the search runs. A pace for the operator's hands, not the
// provider's rule, so it is the same for every provider.
const SEARCH_PAUSE_MS = 500;

const suggestionValue = (suggestion: FillerSourceSuggestionDTO) =>
  `${suggestion.provider}:${suggestion.canonicalId}`;

// One search-or-paste field for one provider. Search and exact resolution are deliberately
// read-only; the separate final button is the only action that registers background work.
//
// What counts as an address is the provider's to say: the server answers a pasted address with the
// one source it names, already checked, and words with suggestions.
const ProviderSourceFinder = ({ provider, enabled }: ProviderSourceFinderProps) => {
  const kind = provider.id;
  const minimumQueryLength = provider.findMinQuery ?? 2;
  const inputID = useId();
  const currentInput = useRef("");
  const resolveGeneration = useRef(0);
  const queryClient = useQueryClient();
  const [input, setInput] = useState("");
  const [debounced, setDebounced] = useState("");
  const [selected, setSelected] = useState<FillerSourceSuggestionDTO>();
  const [verified, setVerified] = useState(false);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(input.trim()), SEARCH_PAUSE_MS);
    return () => window.clearTimeout(timer);
  }, [input]);

  const suggestionsQuery = fillerApi.useSuggestFillerSources(
    kind,
    { q: debounced, limit: 8 },
    {
      query: {
        enabled: enabled && !selected && debounced.length >= minimumQueryLength,
        placeholderData: (previous) => previous,
      },
    },
  );
  const answer = unwrap(suggestionsQuery.data, (body) => body);
  const exact = answer?.exact === true && !suggestionsQuery.isPlaceholderData;
  const suggestions = answer?.exact ? [] : (answer?.suggestions ?? []);

  // A pasted address comes back already checked: take it as the chosen source, as if it had been
  // picked from the list and verified.
  useEffect(() => {
    const [source] = answer?.suggestions ?? [];
    if (!exact || selected || !source || debounced !== input.trim()) return;
    setSelected(source);
    setVerified(true);
    currentInput.current = source.title;
    setInput(source.title);
  }, [answer, debounced, exact, input, selected]);

  const resolveSource = fillerApi.useResolveFillerSource();
  const resolveInput = useCallback(
    (value: string) => {
      const generation = ++resolveGeneration.current;
      const inputAtRequest = currentInput.current;
      resolveSource.mutate(
        { kind, data: { input: value } },
        {
          onSuccess: (response) => {
            if (generation !== resolveGeneration.current || inputAtRequest !== currentInput.current) return;
            const result = unwrap(response, (body) => body);
            if (result) {
              setSelected(result);
              setVerified(true);
              currentInput.current = result.title;
              setInput(result.title);
            }
          },
          onError: () => {
            if (generation !== resolveGeneration.current) return;
            setSelected(undefined);
            setVerified(false);
          },
        },
      );
    },
    [kind, resolveSource],
  );
  const addSource = fillerApi.useAddFillerSource({
    mutation: {
      onSuccess: () => {
        toast.success("Source added", { description: "Loomarr will check it automatically." });
        currentInput.current = "";
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
    currentInput.current = suggestion.title;
    setInput(suggestion.title);
    resolveInput(suggestion.canonicalUrl);
  };

  const cancelSelection = () => {
    resolveGeneration.current += 1;
    currentInput.current = "";
    setInput("");
    setDebounced("");
    setSelected(undefined);
    setVerified(false);
    document.getElementById(inputID)?.focus();
  };

  const submit = () => {
    if (!enabled || !input.trim()) return;
    if (!selected) {
      const value = input.trim();
      if (debounced === value) {
        void suggestionsQuery.refetch();
      } else {
        setDebounced(value);
      }
      return;
    }
    if (!verified || selected.alreadyAdded) return;
    addSource.mutate({ data: { kind, uri: selected.canonicalUrl, label: selected.title } });
  };

  const searching = !selected && suggestionsQuery.isFetching;
  const showSuggestions = enabled && !selected && !exact && input.trim().length >= minimumQueryLength;
  const failure = suggestionsQuery.error ?? resolveSource.error ?? addSource.error;
  const problem = failure ? toProblem(failure) : undefined;

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
        <div className="min-w-0 flex-1">
          {/* No option is highlighted until the arrow keys move there, so Enter on typed words still
              submits the form (a search), and on a highlighted suggestion picks it. */}
          <Combobox
            disabled={!enabled}
            emptyText={
              showSuggestions && !suggestionsQuery.isFetching && debounced.length >= minimumQueryLength
                ? provider.findNoMatches
                : undefined
            }
            id={inputID}
            inputValue={input}
            label={provider.findLabel ?? "Find a source"}
            loading={showSuggestions && suggestionsQuery.isFetching && suggestions.length === 0}
            onInputValueChange={(text) => {
              resolveGeneration.current += 1;
              currentInput.current = text;
              setInput(text);
              setSelected(undefined);
              setVerified(false);
            }}
            onSelect={(option) => {
              const choice = suggestions.find((suggestion) => suggestionValue(suggestion) === option.value);
              if (choice) choose(choice);
            }}
            options={
              showSuggestions
                ? suggestions.map((suggestion) => ({
                    description: suggestion.description || suggestion.canonicalId,
                    label: suggestion.title,
                    trailing: suggestion.alreadyAdded ? "Added" : targetLabel(suggestion),
                    value: suggestionValue(suggestion),
                  }))
                : []
            }
            placeholder={provider.findPlaceholder}
          />
        </div>
        <Action
          disabled={
            !enabled ||
            !input.trim() ||
            resolveSource.isPending ||
            addSource.isPending ||
            searching ||
            selected?.alreadyAdded ||
            Boolean(selected && !verified)
          }
          submit
          variant={selected ? undefined : "outline"}
        >
          {resolveSource.isPending
            ? "Checking…"
            : addSource.isPending
              ? "Adding…"
              : searching
                ? "Searching…"
                : selected
                  ? provider.findAdd
                  : "Search"}
        </Action>
      </form>

      {!enabled && (
        <Text as="p" className="mt-2" textRole="hint" tone="secondary">
          Resume this service to find a source.
        </Text>
      )}

      {selected && (
        <div className="bg-muted/35 mt-2 rounded-lg px-3 py-2">
          <Stack align="start" direction="row" gap="4" justify="between">
            <div className="min-w-0">
              <Text as="p" className="truncate" textRole="cardLabel">
                {selected.title}
              </Text>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Text as="span" textRole="hint" tone="secondary">
                {selected.alreadyAdded ? "Already added" : targetLabel(selected)}
              </Text>
              <Action onPress={cancelSelection} variant="ghost">
                Cancel
              </Action>
            </div>
          </Stack>
          <SourceContentPreview
            provider={provider}
            canonicalUrl={selected.canonicalUrl}
            previewItems={verified ? selected.previewItems : undefined}
            loading={resolveSource.isPending && !verified}
          />
          {verified &&
            !selected.alreadyAdded &&
            (!selected.previewItems || selected.previewItems.length === 0) && (
              <Text as="p" className="mt-2" textRole="hint" tone="secondary">
                Loomarr checks a few new items at a time, and reviews every clip before it can play.
              </Text>
            )}
        </div>
      )}

      {problem && (
        <Text as="p" className="mt-2" textRole="compact" tone="danger">
          {problem.detail ??
            problem.title ??
            `${provider.label} is not responding right now. Try again in a moment.`}
        </Text>
      )}
    </div>
  );
};

export { ProviderSourceFinder };
