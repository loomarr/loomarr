// PROTOTYPE — throwaway (Direction mocks, 2026-11-08, #2786)
// The Settings index as seven cards plus Advanced, three layouts. A: a grid of cards; a card opens
// its own short page list in place of the grid, with a way back. B: one list, the status word on
// the right; a row opens its pages inline beneath it. C: cards that say their first problem in a
// sentence and list their pages right inside, so nothing needs opening. Every status is a dot and a
// sentence-case word, never a filled chip. Motion is a 120ms colour fade on hover; no transforms.
import { StatusDot, Text } from "@loomarr/design-system";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp } from "lucide-react";
import { ADVANCED, CARDS, type CardStatus, type SettingsCard, type SettingsState } from "./data";

const fade = "transition-colors duration-[120ms]";
const focus = "focus-visible:ring-ring focus-visible:ring-2 focus-visible:outline-none";

const Status = ({ status }: { status: CardStatus }) => (
  <span className="text-static-300 inline-flex shrink-0 items-center gap-1.5 text-sm">
    <StatusDot tone={status.tone} label="" />
    {status.word}
  </span>
);

const Title = ({ phone }: { phone: boolean }) => (
  <header className="flex flex-col gap-1">
    <Text as="h1" textRole={phone ? "title" : "display"} style={{ margin: 0 }}>
      Settings
    </Text>
    <p className="text-static-400 m-0 text-sm">Search reaches any setting directly.</p>
  </header>
);

// A stand-in search field: the real index keeps its search (#2786: search still reaches anything).
const SearchStub = () => (
  <div className="border-static-700 text-static-400 flex h-10 items-center rounded-md border px-3 text-sm">
    Search settings
  </div>
);

const PageRow = ({ page, first }: { page: SettingsCard["pages"][number]; first: boolean }) => (
  <li className={first ? undefined : "border-static-700 border-t"}>
    <a
      href={page.path}
      className={`hover:bg-static-800 flex items-center gap-3 px-4 py-3 text-sm focus-visible:ring-inset ${fade} ${focus}`}
    >
      <span className="min-w-0 flex-1 truncate">
        {page.label}
        {page.note && <span className="text-static-400"> · {page.note}</span>}
      </span>
      <ChevronRight aria-hidden className="text-static-500 size-4 shrink-0" />
    </a>
  </li>
);

const AdvancedRow = () => (
  <a
    href={ADVANCED.path}
    className={`border-static-700 hover:bg-static-800 flex items-center gap-3 rounded-lg border border-dashed px-4 py-3 ${fade} ${focus}`}
  >
    <span className="min-w-0 flex-1">
      <span className="block text-sm font-semibold">{ADVANCED.label}</span>
      <span className="text-static-400 block truncate text-xs">{ADVANCED.description}</span>
    </span>
    <ChevronRight aria-hidden className="text-static-500 size-4 shrink-0" />
  </a>
);

const pageCount = (c: SettingsCard) => (c.pages.length === 1 ? "1 page" : `${c.pages.length} pages`);

// ---------------------------------------------------------------------------------------------
// A — a grid of cards; one opens to its page list.

const OpenCard = ({
  card,
  state,
  onBack,
}: {
  card: SettingsCard;
  state: SettingsState;
  onBack: () => void;
}) => {
  const status = card.status[state];
  return (
    <div className="flex flex-col gap-5">
      <button
        type="button"
        onClick={onBack}
        className={`text-static-300 hover:text-foreground flex w-fit items-center gap-1 text-sm ${fade} ${focus}`}
      >
        <ChevronLeft aria-hidden className="size-4" />
        Settings
      </button>
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <Text as="h1" textRole="title" style={{ margin: 0 }}>
          {card.title}
        </Text>
        <Status status={status} />
      </div>
      <p className="text-static-400 m-0 -mt-3 text-sm">{card.description}</p>
      <ul className="border-static-700 bg-static-900 m-0 flex list-none flex-col overflow-hidden rounded-lg border p-0">
        {card.pages.map((p, i) => (
          <PageRow key={p.label} page={p} first={i === 0} />
        ))}
      </ul>
    </div>
  );
};

const GridIndex = ({
  state,
  phone,
  open,
  onOpen,
}: {
  state: SettingsState;
  phone: boolean;
  open?: SettingsCard;
  onOpen: (id: string | undefined) => void;
}) => {
  if (open) return <OpenCard card={open} state={state} onBack={() => onOpen(undefined)} />;
  return (
    <div className="flex flex-col gap-6">
      <Title phone={phone} />
      <SearchStub />
      <ul className={`m-0 grid list-none gap-4 p-0 ${phone ? "grid-cols-1" : "grid-cols-3"}`}>
        {CARDS.map((c) => (
          <li key={c.id}>
            <button
              type="button"
              onClick={() => onOpen(c.id)}
              className={`border-static-700 bg-static-900 hover:bg-static-800 flex h-full w-full flex-col gap-2 rounded-lg border p-4 text-left ${fade} ${focus}`}
            >
              <span className="text-base font-semibold">{c.title}</span>
              <Status status={c.status[state]} />
              <span className="text-static-400 text-sm">{c.description}</span>
              <span className="text-static-500 mt-auto pt-1 text-xs">{pageCount(c)}</span>
            </button>
          </li>
        ))}
      </ul>
      <AdvancedRow />
    </div>
  );
};

// ---------------------------------------------------------------------------------------------
// B — one list, status on the right; a row opens its pages beneath it.

const ListIndex = ({
  state,
  phone,
  open,
  onOpen,
}: {
  state: SettingsState;
  phone: boolean;
  open?: SettingsCard;
  onOpen: (id: string | undefined) => void;
}) => (
  <div className="flex flex-col gap-6">
    <Title phone={phone} />
    <SearchStub />
    <ul className="border-static-700 bg-static-900 m-0 flex list-none flex-col overflow-hidden rounded-lg border p-0">
      {CARDS.map((c, i) => {
        const isOpen = open?.id === c.id;
        const Chevron = isOpen ? ChevronUp : ChevronDown;
        return (
          <li key={c.id} className={i > 0 ? "border-static-700 border-t" : undefined}>
            <button
              type="button"
              aria-expanded={isOpen}
              onClick={() => onOpen(isOpen ? undefined : c.id)}
              className={`hover:bg-static-800 flex w-full items-center gap-4 px-4 py-3.5 text-left focus-visible:ring-inset ${fade} ${focus}`}
            >
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-semibold">{c.title}</span>
                {!phone && <span className="text-static-400 block truncate text-sm">{c.description}</span>}
              </span>
              <Status status={c.status[state]} />
              <Chevron aria-hidden className="text-static-500 size-4 shrink-0" />
            </button>
            {isOpen && (
              <ul className="border-static-700 bg-background m-0 flex list-none flex-col border-t p-0 pl-4">
                {c.pages.map((p, j) => (
                  <PageRow key={p.label} page={p} first={j === 0} />
                ))}
              </ul>
            )}
          </li>
        );
      })}
    </ul>
    <AdvancedRow />
  </div>
);

// ---------------------------------------------------------------------------------------------
// C — cards that name their first problem and list their pages inside.

const IssueCard = ({ card, state }: { card: SettingsCard; state: SettingsState }) => {
  const status = card.status[state];
  // A long list (Filler) shows its first four and a count, so every card stays short.
  const shown = card.pages.slice(0, 4);
  const more = card.pages.length - shown.length;
  return (
    <section
      aria-labelledby={`card-${card.id}`}
      className="border-static-700 bg-static-900 flex flex-col gap-3 rounded-lg border p-4"
    >
      <div className="flex items-baseline justify-between gap-3">
        <h2 id={`card-${card.id}`} className="m-0 text-base font-semibold">
          {card.title}
        </h2>
        <Status status={status} />
      </div>
      {status.issue ? (
        <p className="border-static-700 m-0 border-l-2 pl-3 text-sm">
          {status.issue.text}{" "}
          <a
            href={card.pages.find((p) => p.label === status.issue?.page)?.path}
            className={`text-static-300 hover:text-foreground underline underline-offset-2 ${fade} ${focus}`}
          >
            Open {status.issue.page}
          </a>
        </p>
      ) : (
        <p className="text-static-400 m-0 text-sm">{card.description}</p>
      )}
      <ul className="m-0 flex list-none flex-wrap gap-x-1 gap-y-0.5 p-0 text-sm">
        {shown.map((p, i) => (
          <li key={p.label} className="flex items-center gap-1">
            {i > 0 && (
              <span aria-hidden className="text-static-600">
                ·
              </span>
            )}
            <a
              href={p.path}
              className={`text-static-300 hover:text-foreground rounded px-1 py-0.5 ${fade} ${focus}`}
            >
              {p.label}
            </a>
          </li>
        ))}
        {more > 0 && (
          <li className="flex items-center gap-1">
            <span aria-hidden className="text-static-600">
              ·
            </span>
            <a
              href={card.pages[0]?.path}
              className={`text-static-400 hover:text-foreground rounded px-1 py-0.5 ${fade} ${focus}`}
            >
              {more} more
            </a>
          </li>
        )}
      </ul>
    </section>
  );
};

const IssueIndex = ({ state, phone }: { state: SettingsState; phone: boolean }) => {
  // Cards with a problem come first, in catalog order; the rest keep theirs.
  const ordered = [
    ...CARDS.filter((c) => c.status[state].issue),
    ...CARDS.filter((c) => !c.status[state].issue),
  ];
  const problems = CARDS.filter((c) => c.status[state].issue).length;
  return (
    <div className="flex flex-col gap-6">
      <Title phone={phone} />
      <SearchStub />
      <p className="text-static-400 m-0 text-sm">
        {problems === 0
          ? "Everything is ready."
          : `${problems} ${problems === 1 ? "thing needs" : "things need"} a look.`}
      </p>
      <div className={`grid gap-4 ${phone ? "grid-cols-1" : "grid-cols-2"}`}>
        {ordered.map((c) => (
          <IssueCard key={c.id} card={c} state={state} />
        ))}
      </div>
      <AdvancedRow />
    </div>
  );
};

// ---------------------------------------------------------------------------------------------

const SettingsIndex = ({
  variant,
  state,
  phone,
  open,
  onOpen,
}: {
  variant: string;
  state: SettingsState;
  phone: boolean;
  open?: SettingsCard;
  onOpen: (id: string | undefined) => void;
}) => (
  <div className={`mx-auto flex max-w-5xl flex-col pb-8 ${phone ? "p-4" : "p-8"}`}>
    {variant === "C" ? (
      <IssueIndex state={state} phone={phone} />
    ) : variant === "B" ? (
      <ListIndex state={state} phone={phone} open={open} onOpen={onOpen} />
    ) : (
      <GridIndex state={state} phone={phone} open={open} onOpen={onOpen} />
    )}
  </div>
);

export { SettingsIndex };
