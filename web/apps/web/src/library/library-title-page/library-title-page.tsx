import * as libraryApi from "@loomarr/api/endpoints/library";
import { LibraryMatchDTOApp } from "@loomarr/api/models/libraryMatchDTOApp";
import { unwrap } from "@loomarr/api/unwrap";
import { LibraryTitle } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { useDocumentTitle } from "@/lib/use-document-title";
import { searchLibraryTitles } from "@/library/use-library-needs";

// A TITLE'S PAGE (#2151 decisions 25–26): one movie or series from the Library. Back returns to
// the grid as it was left (its filter and search are in the URL), or to the Library when the
// page was opened directly. Update details, Check files again and Wrong title? act on it; the
// scanner writes it again in the background, and the page follows.

const APP_NAMES: Record<LibraryMatchDTOApp, string> = {
  [LibraryMatchDTOApp.sonarr]: "Sonarr",
  [LibraryMatchDTOApp.radarr]: "Radarr",
};

// A call that answers with an error status still resolves; this makes it a rejection, so the page
// says it didn't go through.
const done = (res: { status: number }) => {
  if (res.status !== 204) throw new Error(`status ${res.status}`);
};

const LibraryTitlePage = ({ id }: { id: string }) => {
  const navigate = useNavigate();
  const router = useRouter();
  const queryClient = useQueryClient();
  const detail = libraryApi.useGetLibraryItem(id);
  const body = unwrap(detail.data);
  useDocumentTitle(body?.item.name ?? "Library");

  if (detail.error) return <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />;
  if (!body) return null;

  // An answer moves the title in and out of Needs you and can rename it on the shelf.
  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: libraryApi.getGetLibraryItemQueryKey(id) }),
      queryClient.invalidateQueries({ queryKey: libraryApi.getListLibraryNeedsQueryKey() }),
      queryClient.invalidateQueries({ queryKey: libraryApi.getLibraryStatusQueryKey() }),
      queryClient.invalidateQueries({ queryKey: ["/v1/library/items"] }),
    ]);
  const act = async (call: Promise<{ status: number }>) => {
    done(await call);
    await refresh();
  };

  const { item, match } = body;
  const page = match.page;
  return (
    <LibraryTitle
      onBack={() => {
        if (router.history.canGoBack()) router.history.back();
        else void navigate({ to: "/library" });
      }}
      onCheckFiles={() => act(libraryApi.checkLibraryItemFiles(id))}
      onForget={() => act(libraryApi.unanswerLibraryItem(id))}
      onLink={(tmdbId) => act(libraryApi.linkLibraryItem(id, { tmdbId }))}
      onOpenApp={page ? () => window.open(page, "_blank", "noopener,noreferrer") : undefined}
      onSearch={searchLibraryTitles}
      onUpdateDetails={() => act(libraryApi.updateLibraryItemDetails(id))}
      title={{
        kind: item.kind,
        name: item.name,
        year: item.year,
        poster: item.poster ? <ServedImage alt="" image={item.poster} priority sizes="208px" /> : undefined,
        overview: body.overview,
        genres: body.genres,
        rating: body.rating,
        runtimeMillis: body.runtimeMillis,
        files: body.files,
        seasons: body.seasons,
        match: { by: match.by, app: match.app ? APP_NAMES[match.app] : undefined },
      }}
    />
  );
};

export { LibraryTitlePage };
