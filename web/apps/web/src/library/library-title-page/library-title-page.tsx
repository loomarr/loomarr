import * as libraryApi from "@loomarr/api/endpoints/library";
import { unwrap } from "@loomarr/api/unwrap";
import { LibraryTitle } from "@loomarr/ui";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { useDocumentTitle } from "@/lib/use-document-title";

// A TITLE'S PAGE (#2151 decisions 25–26): one movie or series from the Library. Back returns to
// the grid as it was left (its filter and search are in the URL), or to the Library when the
// page was opened directly.

const LibraryTitlePage = ({ id }: { id: string }) => {
  const navigate = useNavigate();
  const router = useRouter();
  const detail = libraryApi.useGetLibraryItem(id);
  const body = unwrap(detail.data);
  useDocumentTitle(body?.item.name ?? "Library");

  if (detail.error) return <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />;
  if (!body) return null;

  const { item } = body;
  return (
    <LibraryTitle
      onBack={() => {
        if (router.history.canGoBack()) router.history.back();
        else void navigate({ to: "/library" });
      }}
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
      }}
    />
  );
};

export { LibraryTitlePage };
