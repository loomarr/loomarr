import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryIgnoredDTO } from "@loomarr/api/models/libraryIgnoredDTO";
import { unwrap } from "@loomarr/api/unwrap";
import {
  Action as BaseAction,
  FileName,
  Icon,
  icons,
  Stack,
  Surface,
  Tabs,
  Text,
  useWindowWidth,
} from "@loomarr/design-system";
import { LibraryNeedsCards, waitingTitles } from "@loomarr/ui";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type ComponentProps, type ReactNode, useEffect, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useRouteContent } from "@/lib/use-route-content";
import { toastBringBackFailed, toastIgnored, toastKept } from "../needs-toasts";
import { useLibraryNeeds } from "../use-library-needs";
import { useLibraryScanFrames } from "../use-library-scan-frames";

// NEEDS YOU'S OWN PAGE (#4476 A4): what waits for an answer, and what was ignored.

const IGNORED_PAGE = 50;
const IGNORED_KEY = "/v1/library/ignored";

type NeedsTab = "waiting" | "ignored";

// A phone is touched, so its controls get the touch target.
const useDensity = () => (useWindowWidth() >= 640 ? "pointer" : "touch");
const Action = (props: ComponentProps<typeof BaseAction>) => <BaseAction {...props} density={useDensity()} />;

const IgnoredList = ({
  items,
  onBringBack,
  onMore,
  bringing,
}: {
  items: readonly LibraryIgnoredDTO[];
  onBringBack: (item: LibraryIgnoredDTO) => void;
  /** The rows whose Bring back is on its way. */
  bringing?: ReadonlySet<string>;
  /** Absent once every ignored title is listed. */
  onMore?: () => void;
}) => {
  const buttons = useRef(new Map<string, { focus?: () => void }>());
  const pressed = useRef<{ id: string; index: number }>(null);
  // Focus waits for the list to reload, so a second Enter can't bring back the next file. It moves
  // only if the page lost it: someone who has gone elsewhere is left there.
  useEffect(() => {
    const row = pressed.current;
    if (!row) return;
    const listed = items.some((item) => item.id === row.id);
    if (listed && bringing?.has(row.id)) return;
    pressed.current = null;
    if (document.activeElement && document.activeElement !== document.body) return;
    // A refused row is still listed and takes focus back; a restored one hands it to its neighbour.
    const next = listed ? row : (items[row.index] ?? items[row.index - 1]);
    const target = next && buttons.current.get(next.id);
    if (target) target.focus?.();
    else focusHeading();
  });
  const bring = (item: LibraryIgnoredDTO, index: number) => {
    pressed.current = { id: item.id, index };
    onBringBack(item);
  };
  return (
    <Surface backgroundColor="transparent" borderWidth={0} gap="control">
      <Text textRole="body" tone="secondary">
        {items.length === 0
          ? "Nothing is ignored."
          : "Ignored files stay where they are. Bring one back and it waits here again."}
      </Text>
      {items.map((item, index) => (
        <Surface gap="inline" key={item.id} padding="control">
          <FileName name={item.name} strong />
          <Text textRole="compact" tone="secondary">
            {item.folder}
          </Text>
          <div>
            <Action
              accessibilityLabel={`Bring back ${item.name}`}
              disabled={bringing?.has(item.id)}
              onPress={() => bring(item, index)}
              ref={(node) => {
                if (node) buttons.current.set(item.id, node as unknown as { focus?: () => void });
                else buttons.current.delete(item.id);
              }}
              variant="outline"
            >
              Bring back
            </Action>
          </div>
        </Surface>
      ))}
      {onMore ? (
        <div>
          <Action onPress={onMore} variant="ghost">
            Show more
          </Action>
        </div>
      ) : null}
    </Surface>
  );
};

/** The Waiting tab with nothing left to ask. */
const AllCaughtUp = ({
  ignored,
  onBack,
  onShowIgnored,
}: {
  /** How many titles are ignored; none or unknown offers no way to them. */
  ignored?: number;
  onBack: () => void;
  onShowIgnored: () => void;
}) => (
  <Stack align="center" gap="3" paddingVertical={64}>
    <Icon decorative glyph={icons.success} size="tv" />
    <Text as="h2" textRole="heading">
      All caught up
    </Text>
    <Text textRole="body" tone="secondary">
      Loomarr named every file it found. When it can't name a new one, it waits here.
    </Text>
    <Stack align="center" direction="row" gap="4" wrap>
      <Action icon="back" onPress={onBack} tone="primary">
        Back to the Library
      </Action>
      {ignored ? (
        <Action onPress={onShowIgnored} variant="ghost">
          {ignored === 1 ? "1 ignored file" : `${ignored} ignored files`}
        </Action>
      ) : null}
    </Stack>
  </Stack>
);

// With no card or row left to take focus, the page's heading takes it, so it stays in the page.
// It is focusable only while it holds focus, and draws the amber look rather than the browser's ring.
const focusHeading = () => {
  const heading = document.querySelector("h1");
  if (!heading) return;
  heading.setAttribute("tabindex", "-1");
  heading.style.outline = "2px solid var(--actionFocus)";
  heading.addEventListener(
    "blur",
    () => {
      heading.removeAttribute("tabindex");
      heading.style.outline = "";
    },
    { once: true },
  );
  heading.focus({ preventScroll: true });
};

/** The page's frame: the header, the way back and the two tabs. */
const LibraryNeedsView = ({
  tab,
  waiting,
  ignored,
  onTabChange,
  onBack,
  children,
}: {
  tab: NeedsTab;
  /** Each tab's count; absent while it loads. */
  waiting?: number;
  ignored?: number;
  onTabChange: (tab: NeedsTab) => void;
  onBack: () => void;
  children: ReactNode;
}) => (
  <Stack>
    <div className="px-6 pt-3">
      <Action icon="back" onPress={onBack} variant="ghost">
        Library
      </Action>
    </div>
    <PageHeader
      description="Files Loomarr couldn't name on its own. Answer once and it remembers."
      title="Needs you"
    />
    <Stack gap="4" maxWidth={896} paddingBottom={112} paddingHorizontal={24} paddingTop={8} width="100%">
      <Tabs
        density={useDensity()}
        appearance="underline"
        label="Which files"
        onValueChange={onTabChange}
        options={[
          { value: "waiting", label: "Waiting", count: waiting?.toString() },
          { value: "ignored", label: "Ignored", count: ignored?.toString() },
        ]}
        value={tab}
      />
      {children}
    </Stack>
  </Stack>
);

const LibraryNeedsPage = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  // Which titles are listed: a filter of the page, not an answer being drafted.
  const [tab, setFilterTab] = useState<NeedsTab>("waiting");
  // An answered card remembers that it left. Counting a title's restores asks about it again.
  const [restored, setRestored] = useState<Record<string, number>>({});
  // The titles whose Bring back is on its way: the server refuses a second restore.
  const bringing = useRef(new Set<string>());
  // The same set as drawn: their rows are disabled.
  const [bringingShown, setBringingShown] = useState<ReadonlySet<string>>(new Set<string>());
  useLibraryScanFrames();
  const needs = useLibraryNeeds();
  const ignored = useInfiniteQuery({
    queryKey: [IGNORED_KEY, "pages"],
    queryFn: async ({ pageParam }) => {
      const body = unwrap(await libraryApi.listLibraryIgnored({ limit: IGNORED_PAGE, offset: pageParam }));
      if (!body) throw new Error("Couldn't list the ignored titles");
      return body;
    },
    initialPageParam: 0,
    getNextPageParam: (last, pages) => {
      const listed = pages.reduce((sum, page) => sum + page.items.length, 0);
      return last.items.length > 0 && listed < last.total ? listed : undefined;
    },
  });
  useRouteContent(needs.loaded);

  const refresh = () =>
    Promise.all(
      [
        libraryApi.getListLibraryNeedsQueryKey(),
        libraryApi.getLibraryStatusQueryKey(),
        ["/v1/library/items"],
        [IGNORED_KEY],
      ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
    );
  const restore = async (ids: string[]) => {
    try {
      await libraryApi.restoreLibraryIgnored({ ids });
    } finally {
      // A refusal means the list on screen is stale, so it is read again either way.
      await refresh();
    }
    askAgain(ids);
  };
  const askAgain = (ids: string[]) =>
    setRestored((all) => ({ ...all, ...Object.fromEntries(ids.map((id) => [id, (all[id] ?? 0) + 1])) }));
  const unkeep = async (id: string) => {
    try {
      await libraryApi.unanswerLibraryItem(id);
    } finally {
      await refresh();
    }
    askAgain([id]);
  };
  const bringBack = (id: string) => {
    if (bringing.current.has(id)) return;
    bringing.current.add(id);
    setBringingShown(new Set(bringing.current));
    const bring = () => restore([id]);
    bring()
      .catch((e: unknown) => toastBringBackFailed(e, bring))
      .finally(() => {
        bringing.current.delete(id);
        setBringingShown(new Set(bringing.current));
      });
  };

  const nameOf = (id: string) => {
    const path = needs.needs.find((need) => need.id === id)?.files[0];
    return path?.split(/[\\/]/).pop() ?? "1 title";
  };
  const ignoreOne = async (id: string) => {
    const name = nameOf(id);
    await needs.answers.onIgnore(id);
    void queryClient.invalidateQueries({ queryKey: [IGNORED_KEY] });
    toastIgnored(name, () => restore([id]));
  };
  const keepOne = async (id: string) => {
    const name = nameOf(id);
    await needs.onKeep(id);
    toastKept(name, () => unkeep(id));
  };
  const ignoreFolder = async (folder: string, ids: string[]) => {
    try {
      const body = unwrap(await libraryApi.ignoreLibraryNeeds({ ids }));
      if (!body) throw new Error("Couldn't ignore the folder");
      const done = body.ids;
      toastIgnored(`${done.length === 1 ? "1 file" : `${done.length} files`} in ${folder}`, () =>
        restore(done),
      );
    } finally {
      void refresh();
    }
  };

  const total = ignored.data?.pages.at(-1)?.total;
  // The list behind the chosen tab: its failure shows, and Try again asks for it.
  const shown = tab === "waiting" ? needs : ignored;

  return (
    <LibraryNeedsView
      ignored={total}
      onBack={() => void navigate({ to: "/library" })}
      onTabChange={setFilterTab}
      tab={tab}
      waiting={needs.loaded ? waitingTitles(needs.needs, needs.orders, needs.problems) : undefined}
    >
      {shown.error ? <ErrorState error={shown.error} onRetry={() => void shown.refetch()} /> : null}
      {tab === "waiting" && needs.loaded ? (
        <LibraryNeedsCards
          empty={
            <AllCaughtUp
              ignored={total}
              onBack={() => void navigate({ to: "/library" })}
              onShowIgnored={() => setFilterTab("ignored")}
            />
          }
          hideHeading
          needs={needs.needs}
          restored={restored}
          onChooseOrder={needs.answers.onChooseOrder}
          onIgnore={ignoreOne}
          onIgnoreFolder={ignoreFolder}
          onKeep={keepOne}
          onNoneLeft={focusHeading}
          onLink={needs.answers.onLink}
          onOpenTitle={(id) => void navigate({ to: "/library/$id", params: { id } })}
          onSearch={needs.onSearch}
          orders={needs.orders}
          problems={needs.problems}
        />
      ) : null}
      {tab === "ignored" && ignored.data ? (
        <IgnoredList
          bringing={bringingShown}
          items={ignored.data.pages.flatMap((page) => page.items)}
          onBringBack={(item) => bringBack(item.id)}
          onMore={ignored.hasNextPage ? () => void ignored.fetchNextPage() : undefined}
        />
      ) : null}
    </LibraryNeedsView>
  );
};

export type { NeedsTab };
export { AllCaughtUp, IgnoredList, LibraryNeedsPage, LibraryNeedsView };
