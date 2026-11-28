import * as libraryApi from "@loomarr/api/endpoints/library";
import type { LibraryIgnoredDTO } from "@loomarr/api/models/libraryIgnoredDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, FileName, Stack, Surface, Tabs, Text } from "@loomarr/design-system";
import { LibraryNeedsCards } from "@loomarr/ui";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type ReactNode, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useRouteContent } from "@/lib/use-route-content";
import { toastIgnored, toastUndoFailed } from "../needs-toasts";
import { useLibraryNeeds } from "../use-library-needs";
import { useLibraryScanFrames } from "../use-library-scan-frames";

// NEEDS YOU'S OWN PAGE (#4476 A4): what waits for an answer, and what was ignored.

const IGNORED_PAGE = 50;
const IGNORED_KEY = "/v1/library/ignored";

type NeedsTab = "waiting" | "ignored";

const IgnoredList = ({
  items,
  onBringBack,
  onMore,
}: {
  items: readonly LibraryIgnoredDTO[];
  onBringBack: (item: LibraryIgnoredDTO) => void;
  /** Absent once every ignored title is listed. */
  onMore?: () => void;
}) => (
  <Surface backgroundColor="transparent" borderWidth={0} gap="control">
    <Text textRole="body" tone="secondary">
      {items.length === 0
        ? "Nothing is ignored."
        : "Ignored files stay where they are. Bring one back and it waits here again."}
    </Text>
    {items.map((item) => (
      <Surface gap="inline" key={item.id} padding="control">
        <FileName name={item.name} strong />
        <Text textRole="compact" tone="secondary">
          {item.folder}
        </Text>
        <div>
          <Action
            accessibilityLabel={`Bring back ${item.name}`}
            onPress={() => onBringBack(item)}
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
  // An answered card remembers that it left. A new key forgets that, so a restored title shows.
  const [restored, setRestored] = useState(0);
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
    await libraryApi.restoreLibraryIgnored({ ids });
    await refresh();
    setRestored((n) => n + 1);
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
  const ignoreFolder = async (folder: string, ids: string[]) => {
    try {
      const body = unwrap(await libraryApi.ignoreLibraryNeeds({ ids }));
      if (!body) throw new Error("Couldn't ignore the folder");
      const done = body.ids;
      toastIgnored(`${done.length} in ${folder}`, () => restore(done));
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
      waiting={needs.loaded ? needs.needs.length + needs.orders.length + needs.problems.length : undefined}
    >
      {shown.error ? <ErrorState error={shown.error} onRetry={() => void shown.refetch()} /> : null}
      {tab === "waiting" && needs.loaded ? (
        <LibraryNeedsCards
          hideHeading
          key={restored}
          needs={needs.needs}
          onChooseOrder={needs.answers.onChooseOrder}
          onIgnore={ignoreOne}
          onIgnoreFolder={ignoreFolder}
          onLink={needs.answers.onLink}
          onOpenTitle={(id) => void navigate({ to: "/library/$id", params: { id } })}
          onSearch={needs.onSearch}
          orders={needs.orders}
          problems={needs.problems}
        />
      ) : null}
      {tab === "ignored" && ignored.data ? (
        <IgnoredList
          items={ignored.data.pages.flatMap((page) => page.items)}
          onBringBack={(item) => {
            const bring = () => restore([item.id]);
            bring().catch((e: unknown) => toastUndoFailed(e, bring));
          }}
          onMore={ignored.hasNextPage ? () => void ignored.fetchNextPage() : undefined}
        />
      ) : null}
    </LibraryNeedsView>
  );
};

export type { NeedsTab };
export { IgnoredList, LibraryNeedsPage, LibraryNeedsView };
