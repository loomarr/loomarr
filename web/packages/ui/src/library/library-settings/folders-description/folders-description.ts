import { joinNames } from "../../library-words";
import { AUTHORITY_NAMES } from "../authority-names";
import type { LibraryPathMapping } from "../library-settings.type";

const foldersDescription = (pathMap: readonly LibraryPathMapping[]) => {
  const names = [...new Set(pathMap.map((m) => AUTHORITY_NAMES[m.authority]))];
  return names.length > 0
    ? `Loomarr looks for your shows and movies in these folders. It found them through ${joinNames(names)}.`
    : "Loomarr looks for your shows and movies in these folders.";
};

export { foldersDescription };
