import { Check, ChevronLeft, ChevronRight, Plus, Search, Sparkles, X } from "lucide-react";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

type ReviewVariant = "a" | "b" | "c";

type PrototypeTitle = {
  id: string;
  name: string;
  year: number;
  inLibrary: boolean;
  rating?: string;
  note: string;
};

const STARTING_TITLES: PrototypeTitle[] = [
  {
    id: "boy-meets-world",
    name: "Boy Meets World",
    year: 1993,
    inLibrary: true,
    rating: "TV-G",
    note: "A central TGIF series from the 1990s.",
  },
  {
    id: "full-house",
    name: "Full House",
    year: 1987,
    inLibrary: false,
    rating: "TV-G",
    note: "One of the block's best-known family comedies.",
  },
  {
    id: "family-matters",
    name: "Family Matters",
    year: 1989,
    inLibrary: false,
    rating: "TV-G",
    note: "A long-running anchor of the Friday-night lineup.",
  },
  {
    id: "step-by-step",
    name: "Step by Step",
    year: 1991,
    inLibrary: false,
    rating: "TV-G",
    note: "A core family sitcom from the same programming block.",
  },
  {
    id: "dinosaurs",
    name: "Dinosaurs",
    year: 1991,
    inLibrary: false,
    rating: "TV-PG",
    note: "A distinctive TGIF-era family comedy.",
  },
  {
    id: "hangin-with-mr-cooper",
    name: "Hangin' with Mr. Cooper",
    year: 1992,
    inLibrary: false,
    rating: "TV-PG",
    note: "A recurring TGIF comedy from the early 1990s.",
  },
];

const SUGGESTED_TITLES: PrototypeTitle[] = [
  {
    id: "sabrina",
    name: "Sabrina the Teenage Witch",
    year: 1996,
    inLibrary: false,
    rating: "TV-G",
    note: "A major late-1990s TGIF title.",
  },
  {
    id: "perfect-strangers",
    name: "Perfect Strangers",
    year: 1986,
    inLibrary: true,
    rating: "TV-PG",
    note: "Part of the block's original Friday-night identity.",
  },
  {
    id: "just-the-ten-of-us",
    name: "Just the Ten of Us",
    year: 1988,
    inLibrary: false,
    note: "An early TGIF family sitcom.",
  },
  {
    id: "you-wish",
    name: "You Wish",
    year: 1997,
    inLibrary: false,
    rating: "TV-G",
    note: "A shorter-lived supernatural sitcom from the block.",
  },
];

const SEARCH_TITLES: PrototypeTitle[] = [
  ...SUGGESTED_TITLES,
  {
    id: "sister-sister",
    name: "Sister, Sister",
    year: 1994,
    inLibrary: false,
    rating: "TV-G",
    note: "A compatible 1990s family sitcom that later aired in the block.",
  },
  {
    id: "teen-angel",
    name: "Teen Angel",
    year: 1997,
    inLibrary: false,
    rating: "TV-G",
    note: "A short-lived TGIF supernatural comedy.",
  },
];

const VARIANTS: Array<{ id: ReviewVariant; label: string }> = [
  { id: "a", label: "Direct editor" },
  { id: "b", label: "Lineup + ideas" },
  { id: "c", label: "Summary first" },
];

const titleMeta = (title: PrototypeTitle) =>
  [String(title.year), "Series", title.rating].filter(Boolean).join(" · ");

const AvailabilityBadge = ({ title }: { title: PrototypeTitle }) => (
  <Badge variant={title.inLibrary ? "lock" : "tune"}>
    {title.inLibrary ? "In your library" : "Will be added"}
  </Badge>
);

const PrototypeSwitcher = ({ variant }: { variant: ReviewVariant }) => {
  const index = VARIANTS.findIndex((item) => item.id === variant);
  const current = VARIANTS[index] ?? VARIANTS[0]!;
  const previous = VARIANTS[(index - 1 + VARIANTS.length) % VARIANTS.length]!;
  const next = VARIANTS[(index + 1) % VARIANTS.length]!;

  return (
    <nav
      aria-label="Prototype variants"
      className="fixed right-4 bottom-4 z-50 flex items-center gap-1 rounded-full border border-border bg-card/95 p-1.5 shadow-xl backdrop-blur"
    >
      <a
        href={`/guide?variant=${previous.id}`}
        className="grid size-8 place-items-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
        aria-label={`Previous prototype: ${previous.label}`}
      >
        <ChevronLeft className="size-4" aria-hidden />
      </a>
      <span className="min-w-28 px-2 text-center font-medium text-xs">
        {variant.toUpperCase()} · {current.label}
      </span>
      <a
        href={`/guide?variant=${next.id}`}
        className="grid size-8 place-items-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
        aria-label={`Next prototype: ${next.label}`}
      >
        <ChevronRight className="size-4" aria-hidden />
      </a>
    </nav>
  );
};

const Brief = ({ compact = false }: { compact?: boolean }) => {
  const [editing, setEditing] = useState(false);
  const [brief, setBrief] = useState("A channel based on ABC's TGIF lineup from the 1990s");

  if (editing) {
    return (
      <form
        className="flex flex-col gap-2 rounded-lg border border-suggest/35 bg-suggest/5 p-3"
        onSubmit={(event) => {
          event.preventDefault();
          setEditing(false);
        }}
      >
        <label htmlFor="prototype-brief" className="font-medium text-sm">
          Channel description
        </label>
        <textarea
          id="prototype-brief"
          value={brief}
          onChange={(event) => setBrief(event.target.value)}
          rows={compact ? 2 : 3}
          className="resize-none rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-suggest"
        />
        <div className="flex justify-end gap-2">
          <Button type="button" size="sm" variant="ghost" onClick={() => setEditing(false)}>
            Cancel
          </Button>
          <Button type="submit" size="sm" variant="suggest">
            Find new ideas
          </Button>
        </div>
      </form>
    );
  }

  return (
    <div className="flex items-start justify-between gap-3">
      <p className="text-muted-foreground text-sm leading-relaxed">“{brief}”</p>
      <Button variant="ghost" size="sm" className="-my-1 shrink-0" onClick={() => setEditing(true)}>
        Edit
      </Button>
    </div>
  );
};

const TitleRow = ({
  title,
  action,
  actionLabel,
  quiet = false,
}: {
  title: PrototypeTitle;
  action: () => void;
  actionLabel: "Add" | "Remove";
  quiet?: boolean;
}) => (
  <li className={cn("flex items-start gap-3 py-3", !quiet && "border-border border-b last:border-b-0")}>
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-sm">{title.name}</span>
        <AvailabilityBadge title={title} />
      </div>
      <p className="mt-1 text-muted-foreground text-xs">{titleMeta(title)}</p>
      {!quiet && <p className="mt-1.5 text-muted-foreground text-xs leading-relaxed">{title.note}</p>}
    </div>
    <Button
      type="button"
      variant={actionLabel === "Add" ? "outline" : "ghost"}
      size="sm"
      className="shrink-0"
      onClick={action}
      aria-label={`${actionLabel} ${title.name}`}
    >
      {actionLabel === "Add" ? <Plus aria-hidden /> : <X aria-hidden />}
      {actionLabel}
    </Button>
  </li>
);

const ReviewFooter = ({ selected }: { selected: PrototypeTitle[] }) => {
  const missing = selected.filter((title) => !title.inLibrary).length;
  const [message, setMessage] = useState("");
  return (
    <footer className="sticky bottom-0 z-10 flex flex-wrap items-center justify-between gap-3 border-border border-t bg-card/95 py-4 backdrop-blur">
      <p className="text-sm">
        <span className="font-medium">
          {missing === 0
            ? "Everything is ready to play."
            : `${missing} ${missing === 1 ? "title" : "titles"} will be added.`}
        </span>{" "}
        <span className="text-muted-foreground">Nothing happens until you create the channel.</span>
      </p>
      <Button
        disabled={selected.length === 0}
        onClick={() => setMessage("Prototype only — no channel was created.")}
      >
        <Check aria-hidden /> Create channel
      </Button>
      {message && (
        <p role="status" className="w-full text-right text-muted-foreground text-xs">
          {message}
        </p>
      )}
    </footer>
  );
};

const usePrototypeLineup = () => {
  const [selectedIds, setSelectedIds] = useState(() => STARTING_TITLES.map((title) => title.id));
  const [extraIdeas, setExtraIdeas] = useState<PrototypeTitle[]>([]);
  const [query, setQuery] = useState("");
  const allTitles = useMemo(() => [...STARTING_TITLES, ...SUGGESTED_TITLES, ...extraIdeas], [extraIdeas]);
  const selected = selectedIds
    .map((id) => allTitles.find((title) => title.id === id) ?? SEARCH_TITLES.find((title) => title.id === id))
    .filter((title): title is PrototypeTitle => Boolean(title));
  const ideas = [...SUGGESTED_TITLES, ...extraIdeas].filter((title) => !selectedIds.includes(title.id));
  const searchResults = SEARCH_TITLES.filter(
    (title) =>
      query.trim().length > 1 &&
      title.name.toLowerCase().includes(query.trim().toLowerCase()) &&
      !selectedIds.includes(title.id),
  );

  return {
    selected,
    ideas,
    query,
    setQuery,
    searchResults,
    add: (title: PrototypeTitle) => {
      setSelectedIds((current) => (current.includes(title.id) ? current : [...current, title.id]));
      setQuery("");
    },
    remove: (title: PrototypeTitle) => setSelectedIds((current) => current.filter((id) => id !== title.id)),
    addVariety: () =>
      setExtraIdeas((current) =>
        current.length > 0
          ? current
          : SEARCH_TITLES.filter((title) => title.id === "sister-sister" || title.id === "teen-angel"),
      ),
  };
};

const SearchBox = ({
  query,
  onQueryChange,
  results,
  onAdd,
}: {
  query: string;
  onQueryChange: (value: string) => void;
  results: PrototypeTitle[];
  onAdd: (title: PrototypeTitle) => void;
}) => (
  <div className="relative">
    <Search
      className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
      aria-hidden
    />
    <Input
      value={query}
      onChange={(event) => onQueryChange(event.target.value)}
      placeholder="Search for another show…"
      className="pl-9"
      aria-label="Search for another show"
    />
    {results.length > 0 && (
      <ul className="absolute top-full right-0 left-0 z-20 mt-1 rounded-md border border-border bg-card p-1 shadow-lg">
        {results.map((title) => (
          <li key={title.id}>
            <button
              type="button"
              className="flex w-full items-center justify-between rounded px-3 py-2 text-left text-sm hover:bg-muted"
              onClick={() => onAdd(title)}
            >
              <span>
                <span className="font-medium">{title.name}</span>
                <span className="ml-2 text-muted-foreground text-xs">{title.year}</span>
              </span>
              <span className="text-muted-foreground text-xs">Add</span>
            </button>
          </li>
        ))}
      </ul>
    )}
  </div>
);

const DirectEditor = () => {
  const lineup = usePrototypeLineup();
  const ready = lineup.selected.filter((title) => title.inLibrary).length;
  const [updated, setUpdated] = useState(false);

  return (
    <section className="mx-auto flex w-full max-w-3xl flex-col gap-5">
      <div>
        <h2 className="font-semibold text-2xl">TGIF Nights</h2>
        <div className="mt-3 border-border border-y py-3">
          <Brief />
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm">
          <span className="font-medium">{lineup.selected.length} selected</span>
          <span className="text-muted-foreground">
            {" "}
            · {ready} in your library · {lineup.selected.length - ready} will be added
          </span>
        </p>
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            lineup.addVariety();
            setUpdated(true);
          }}
        >
          <Sparkles aria-hidden /> Add more ideas
        </Button>
      </div>
      {updated && (
        <p role="status" className="-mt-2 text-muted-foreground text-xs">
          Two new ideas were added below. Your lineup did not change.
        </p>
      )}

      <section aria-labelledby="direct-selected-heading">
        <h3 id="direct-selected-heading" className="font-medium">
          Selected lineup
        </h3>
        <ul className="mt-2 rounded-lg border border-border px-4">
          {lineup.selected.map((title) => (
            <TitleRow key={title.id} title={title} actionLabel="Remove" action={() => lineup.remove(title)} />
          ))}
        </ul>
      </section>

      <SearchBox
        query={lineup.query}
        onQueryChange={lineup.setQuery}
        results={lineup.searchResults}
        onAdd={lineup.add}
      />

      <section aria-labelledby="direct-ideas-heading" className="rounded-lg bg-muted/35 p-4">
        <div>
          <h3 id="direct-ideas-heading" className="font-medium">
            More suggestions
          </h3>
          <p className="mt-0.5 text-muted-foreground text-xs">Optional—add only what you want.</p>
        </div>
        <ul className="mt-2">
          {lineup.ideas.map((title) => (
            <TitleRow key={title.id} title={title} actionLabel="Add" action={() => lineup.add(title)} />
          ))}
        </ul>
      </section>
      <ReviewFooter selected={lineup.selected} />
    </section>
  );
};

const LineupWorkspace = () => {
  const lineup = usePrototypeLineup();
  const [updated, setUpdated] = useState(false);

  return (
    <section className="mx-auto flex w-full max-w-5xl flex-col gap-5">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="font-semibold text-2xl">TGIF Nights</h2>
          <p className="mt-1 text-muted-foreground text-sm">
            Build the lineup on the left. Ideas stay optional.
          </p>
        </div>
        <Button
          variant="suggest"
          size="sm"
          onClick={() => {
            lineup.addVariety();
            setUpdated(true);
          }}
        >
          <Sparkles aria-hidden /> Find more ideas
        </Button>
      </header>
      <div className="border-border border-y py-3">
        <Brief compact />
      </div>
      {updated && (
        <p role="status" className="text-muted-foreground text-xs">
          New ideas are ready. Nothing was added to your lineup.
        </p>
      )}

      <div className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,3fr)_minmax(19rem,2fr)]">
        <section aria-labelledby="workspace-lineup-heading" className="rounded-lg border border-border p-4">
          <div className="flex items-baseline justify-between gap-3">
            <h3 id="workspace-lineup-heading" className="font-medium">
              Your lineup
            </h3>
            <span className="text-muted-foreground text-xs">{lineup.selected.length} titles</span>
          </div>
          <ul className="mt-2">
            {lineup.selected.map((title) => (
              <TitleRow
                key={title.id}
                title={title}
                actionLabel="Remove"
                action={() => lineup.remove(title)}
                quiet
              />
            ))}
          </ul>
        </section>

        <aside aria-labelledby="workspace-ideas-heading" className="rounded-lg bg-muted/35 p-4">
          <h3 id="workspace-ideas-heading" className="font-medium">
            Ideas to consider
          </h3>
          <p className="mt-0.5 text-muted-foreground text-xs">Search the catalog or choose a suggestion.</p>
          <div className="mt-3">
            <SearchBox
              query={lineup.query}
              onQueryChange={lineup.setQuery}
              results={lineup.searchResults}
              onAdd={lineup.add}
            />
          </div>
          <ul className="mt-3">
            {lineup.ideas.map((title) => (
              <TitleRow
                key={title.id}
                title={title}
                actionLabel="Add"
                action={() => lineup.add(title)}
                quiet
              />
            ))}
          </ul>
        </aside>
      </div>
      <ReviewFooter selected={lineup.selected} />
    </section>
  );
};

const SummaryFirst = () => {
  const lineup = usePrototypeLineup();
  const [editing, setEditing] = useState(false);
  const [updated, setUpdated] = useState(false);
  const ready = lineup.selected.filter((title) => title.inLibrary).length;
  const names = lineup.selected.slice(0, 3).map((title) => title.name);

  return (
    <section className="mx-auto flex w-full max-w-3xl flex-col gap-5">
      <header>
        <h2 className="font-semibold text-2xl">TGIF Nights</h2>
        <div className="mt-3 border-border border-y py-3">
          <Brief />
        </div>
      </header>

      <section className="rounded-xl border border-border bg-card p-5 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <p className="font-semibold text-3xl">{lineup.selected.length}</p>
            <p className="text-muted-foreground text-sm">selected shows</p>
          </div>
          <Button variant="outline" onClick={() => setEditing((value) => !value)}>
            {editing ? "Done editing" : "Review titles"}
          </Button>
        </div>
        <p className="mt-4 text-sm">
          {ready} ready now · {lineup.selected.length - ready} will be added
        </p>
        <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
          {names.join(", ")}
          {lineup.selected.length > names.length ? `, and ${lineup.selected.length - names.length} more` : ""}
          .
        </p>
        <p className="mt-4 border-border border-t pt-4 text-muted-foreground text-sm">
          About 3 hours of programming before the first repeat.
        </p>
      </section>

      <div className="flex flex-wrap gap-2">
        <Button
          variant="suggest"
          onClick={() => {
            lineup.addVariety();
            setEditing(true);
            setUpdated(true);
          }}
        >
          <Sparkles aria-hidden /> Add more ideas
        </Button>
        <Button variant="ghost" onClick={() => setEditing(true)}>
          Add a specific title
        </Button>
      </div>
      {updated && (
        <p role="status" className="-mt-2 text-muted-foreground text-xs">
          New ideas are ready below. Your selected shows stayed the same.
        </p>
      )}

      {editing && (
        <section aria-label="Edit lineup" className="rounded-lg border border-border p-4">
          <div className="grid gap-5 md:grid-cols-2">
            <div>
              <h3 className="font-medium">Selected</h3>
              <ul className="mt-2">
                {lineup.selected.map((title) => (
                  <TitleRow
                    key={title.id}
                    title={title}
                    actionLabel="Remove"
                    action={() => lineup.remove(title)}
                    quiet
                  />
                ))}
              </ul>
            </div>
            <div>
              <h3 className="font-medium">More suggestions</h3>
              <div className="mt-2">
                <SearchBox
                  query={lineup.query}
                  onQueryChange={lineup.setQuery}
                  results={lineup.searchResults}
                  onAdd={lineup.add}
                />
              </div>
              <ul className="mt-2">
                {lineup.ideas.map((title) => (
                  <TitleRow
                    key={title.id}
                    title={title}
                    actionLabel="Add"
                    action={() => lineup.add(title)}
                    quiet
                  />
                ))}
              </ul>
            </div>
          </div>
        </section>
      )}
      <ReviewFooter selected={lineup.selected} />
    </section>
  );
};

const SuggestionReviewPrototype = ({ variant }: { variant: ReviewVariant }) => (
  <>
    {variant === "a" ? <DirectEditor /> : variant === "b" ? <LineupWorkspace /> : <SummaryFirst />}
    <PrototypeSwitcher variant={variant} />
  </>
);

export type { ReviewVariant };
export { SuggestionReviewPrototype };
