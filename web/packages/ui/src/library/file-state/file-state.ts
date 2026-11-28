import type { LibraryTitleFile } from "../library-title/library-title.type";

// WHAT A FILE IS TO ITS TITLE, in the words the Copies card and Technical details both say.

// A file whose header could not be read has no picture. An unread file has none yet either, and is
// not broken.
const isBrokenFile = (file: LibraryTitleFile) =>
  file.state === "unplayable" || (file.state === "extra" && !file.picture && file.ready !== false);

const fileState = (
  file: LibraryTitleFile,
): { tone: "danger" | "success" | "warning" | "neutral"; words: string } => {
  // A broken extra says both: it is one of the extra copies, and it can't be played.
  if (isBrokenFile(file)) {
    return { tone: "danger", words: file.state === "extra" ? "Extra · can't be played" : "Can't be played" };
  }
  switch (file.state) {
    case "plays":
      return { tone: "success", words: "Plays" };
    case "extra":
      return { tone: "warning", words: "Extra" };
    case "reading":
      return { tone: "neutral", words: "Reading" };
    default:
      return { tone: "neutral", words: "Waiting" };
  }
};

export { fileState, isBrokenFile };
