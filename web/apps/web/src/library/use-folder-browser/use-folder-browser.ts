import * as libraryApi from "@loomarr/api/endpoints/library";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import type { LibraryBrowseListing, LibraryFolderBrowser } from "@loomarr/ui";
import { useState } from "react";

import { useLoomarrEventListener } from "@/events/events-provider";

// THE FOLDER BROWSER'S DATA (#2151): the folder open now, read from GET /v1/library/browse, and
// what each folder in it holds, which arrives afterwards as library_folder frames: nothing polls.
// A frame fills its row wherever it is listed; one counted a few minutes ago comes with the list.

type Summary = NonNullable<LibraryBrowseListing["folders"][number]["summary"]>;

const useFolderBrowser = (): LibraryFolderBrowser => {
  // undefined until the browser is first opened, so nothing is listed for a page that never adds a folder.
  const [path, setPath] = useState<string>();
  const [counted, setCounted] = useState<Readonly<Record<string, Summary>>>({});
  useLoomarrEventListener({
    onLibraryFolder: ({ path: folder, folders, videos, state }) =>
      setCounted((was) => ({ ...was, [folder]: { folders, videos, state } })),
  });
  const browse = libraryApi.useBrowseLibraryFolders(
    { path: path ?? "" },
    { query: { enabled: path !== undefined } },
  );
  const body = unwrap(browse.data);
  const listing: LibraryBrowseListing | undefined = body && {
    ...body,
    folders: body.folders.map((folder) => ({ ...folder, summary: counted[folder.path] ?? folder.summary })),
  };
  const problem = browse.error ? toProblem(browse.error) : undefined;
  return {
    listing,
    loading: browse.isFetching,
    error: problem
      ? [problem.title, problem.detail].filter(Boolean).join(". ") || "Couldn't open that folder."
      : undefined,
    onOpen: setPath,
  };
};

export { useFolderBrowser };
