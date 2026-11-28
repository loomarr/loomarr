import * as libraryApi from "@loomarr/api/endpoints/library";
import { LibraryDetailsFromDTODescription } from "@loomarr/api/models/libraryDetailsFromDTODescription";
import { LibraryMatchDTOApp } from "@loomarr/api/models/libraryMatchDTOApp";
import { ApiError } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { LibraryTitle, LibraryTitleMissing } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { useHouseholdTimeZone } from "@/channels/use-household-time-zone";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { useDocumentTitle } from "@/lib/use-document-title";
import { useRefetchAt } from "@/lib/use-refetch-at";
import {
  toastRemoved,
  toastRestored,
  toastRestoreFailed,
  toastRestoreRefused,
} from "@/library/extra-copy-toasts";
import { searchLibraryTitles } from "@/library/use-library-needs";
import { airingEpisode, airingsChangeAt, airingWhen, dateWords } from "./airing-words";

// A TITLE'S PAGE (#2151 decisions 25–26): one movie or series from the Library. Back returns to
// the grid as it was left (its filter and search are in the URL), or to the Library when the
// page was opened directly. Update details, Check files again and Wrong title? act on it; the
// scanner writes it again in the background, and the page follows.

const APP_NAMES: Record<LibraryMatchDTOApp, string> = {
  [LibraryMatchDTOApp.sonarr]: "Sonarr",
  [LibraryMatchDTOApp.radarr]: "Radarr",
};

// Where a detail came from, as Technical details names it ("Description from TMDB.").
const SOURCE_NAMES: Record<LibraryDetailsFromDTODescription, string> = {
  [LibraryDetailsFromDTODescription.sonarr]: "Sonarr",
  [LibraryDetailsFromDTODescription.radarr]: "Radarr",
  [LibraryDetailsFromDTODescription.nfo]: "its NFO file",
  [LibraryDetailsFromDTODescription.tmdb]: "TMDB",
};
const sourceName = (source?: LibraryDetailsFromDTODescription) => (source ? SOURCE_NAMES[source] : undefined);

const LibraryTitlePage = ({ id }: { id: string }) => {
  const timeZone = useHouseholdTimeZone();
  const navigate = useNavigate();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { isAdmin } = useAuth();
  const detail = libraryApi.useGetLibraryItem(id);
  const body = unwrap(detail.data);
  useDocumentTitle(body?.item.name ?? "Library");
  // An airing starts and ends with nothing sent, so the title is read again at that moment: the
  // words follow the clock, and an ended airing gives way to the channel's next one.
  useRefetchAt({
    deadline: airingsChangeAt(body?.nextAirings ?? []),
    answeredAt: detail.dataUpdatedAt,
    refetch: detail.refetch,
  });

  const back = () => {
    if (router.history.canGoBack()) router.history.back();
    else void navigate({ to: "/library" });
  };

  // A 404 is an unknown title, or one hidden from this member: trying again can't change it.
  if (detail.error instanceof ApiError && detail.error.status === 404) {
    return <LibraryTitleMissing onBack={back} />;
  }
  if (detail.error) return <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />;
  // The route's loader asked for the title, so it is here before this page mounts.
  if (!body) return null;

  // An answer moves the title in and out of Needs you and can rename it on the shelf.
  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: libraryApi.getGetLibraryItemQueryKey(id) }),
      queryClient.invalidateQueries({ queryKey: libraryApi.getListLibraryNeedsQueryKey() }),
      queryClient.invalidateQueries({ queryKey: libraryApi.getLibraryStatusQueryKey() }),
      queryClient.invalidateQueries({ queryKey: ["/v1/library/items"] }),
    ]);
  // A call that answers with an error status rejects, so the page says it didn't go through.
  const act = async (call: Promise<unknown>) => {
    await call;
    await refresh();
  };

  const removeExtra = async (path: string) => {
    const name = path.split(/[\\/]/).pop() ?? path;
    const removed = unwrap(await libraryApi.removeLibraryExtraCopy(id, { path }));
    const undo = (token: string): Promise<unknown> =>
      act(libraryApi.restoreLibraryExtraCopy(id, { undo: token })).then(
        () => toastRestored(name),
        (error: unknown) =>
          // A 4xx refuses for good, and says why. Anything else can be tried again.
          error instanceof ApiError && error.status >= 400 && error.status < 500
            ? toastRestoreRefused(error.message, error.problem.detail)
            : toastRestoreFailed(name, () => void undo(token)),
      );
    toastRemoved(name, removed ? () => void undo(removed.undo) : undefined);
    await refresh();
  };

  // match, files, detailsFrom and the counts are admin-only, and so is every fix: a member's page
  // offers none, so it calls nothing a member would be refused.
  const { item } = body;
  const match = body.match ?? { by: "none" as const };
  const page = match.page;
  return (
    <LibraryTitle
      fix={
        isAdmin
          ? {
              onCheckFiles: () => act(libraryApi.checkLibraryItemFiles(id)),
              // An episode is its own title, so its files come with its own answer.
              onEpisodeFiles: async (episodeId) => {
                const options = libraryApi.getGetLibraryItemQueryOptions(episodeId);
                return unwrap(await queryClient.fetchQuery({ ...options, staleTime: 0 }))?.files ?? [];
              },
              episodeFilesAt: detail.dataUpdatedAt,
              onChooseOrder: () => void navigate({ to: "/library/needs" }),
              onForget: () => act(libraryApi.unanswerLibraryItem(id)),
              onLink: (externalId) => act(libraryApi.linkLibraryItem(id, { externalId })),
              onOpenApp: page ? () => window.open(page, "_blank", "noopener,noreferrer") : undefined,
              onRemoveExtra: removeExtra,
              onSearch: searchLibraryTitles,
              onUpdateDetails: () => act(libraryApi.updateLibraryItemDetails(id)),
            }
          : undefined
      }
      onBack={back}
      title={{
        kind: item.kind,
        name: item.name,
        year: item.year,
        backdrop: body.backdropImage ? (
          <ServedImage alt="" image={body.backdropImage} priority sizes="(min-width: 1216px) 1152px, 100vw" />
        ) : undefined,
        logo: body.logoImage ? (
          <ServedImage alt="" fit="contain" image={body.logoImage} priority sizes="420px" />
        ) : undefined,
        tagline: body.tagline,
        score: body.score,
        credits: body.credits,
        airings: body.nextAirings.flatMap((a) => {
          const when = airingWhen(a, timeZone);
          const episode = a.episode ? airingEpisode(a.episode) : undefined;
          return when ? [{ channel: a.channel, when, forecast: a.forecast, episode }] : [];
        }),
        playing: body.playing,
        about: [
          { label: "Studio", value: body.studios.join(", ") },
          { label: "Country", value: body.countries.join(", ") },
          {
            label: item.kind === "series" ? "First aired" : "Released",
            value: body.premiereDate ? dateWords(body.premiereDate) : "",
          },
        ],
        overview: body.overview,
        genres: body.genres,
        rating: body.rating,
        runtimeMillis: body.runtimeMillis,
        files: body.files ?? [],
        unplayable: item.unplayable ?? 0,
        seasons: body.seasons.map((season) => ({
          ...season,
          episodes: season.episodes.map(({ still, ...episode }) => ({
            ...episode,
            still: still ? <ServedImage alt="" image={still} sizes="160px" /> : undefined,
          })),
        })),
        match: { by: match.by, app: match.app ? APP_NAMES[match.app] : undefined },
        channels: body.channels,
        detailsFrom: {
          description: sourceName(body.detailsFrom?.description),
          rating: sourceName(body.detailsFrom?.rating),
          episodeNames: sourceName(body.detailsFrom?.episodeNames),
        },
      }}
    />
  );
};

export { LibraryTitlePage };
