import * as libraryApi from "@loomarr/api/endpoints/library";
import { createFileRoute } from "@tanstack/react-router";
import { LibraryTitlePage } from "@/library/library-title-page";

// A title's page. A member reads it too (#4476), with no fixes: the server leaves a member's title
// without its files and match, and the page offers a member nothing to change.
const LibraryTitleScreen = () => {
  const { id } = Route.useParams();
  return <LibraryTitlePage id={id} />;
};

const Route = createFileRoute("/_authed/library/$id")({
  component: LibraryTitleScreen,
  // The title's detail is the whole page.
  loader: async ({ context: { queryClient }, params: { id } }) => {
    await queryClient.ensureQueryData(libraryApi.getGetLibraryItemQueryOptions(id));
  },
});

export { Route };
