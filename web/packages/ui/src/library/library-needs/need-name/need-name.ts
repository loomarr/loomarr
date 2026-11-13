import { fileName } from "../file-name";
import type { LibraryNeed } from "../library-needs.type";

// What a need is called where it is named: its best guess, else its first file.
const needName = (need: LibraryNeed) => need.best?.name ?? fileName(need.files[0] ?? "");

export { needName };
