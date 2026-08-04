// The Incoming tab owns everything it needs, so it takes nothing but the one thing the SHELL
// already knows and it cannot re-derive without a second request: whether tagging a clip should
// open the shared dialog. Everything else — the queue, the filing mutations, the busy row — is
// this tab's own concern and lives inside it.
interface IncomingTabProps {
  // onEditTags opens the shell's ClipTagDialog. The dialog is shared with the Catalog tab (one
  // clip, one editor, wherever you reached it from), so the shell owns it and this hands the
  // path up rather than mounting a second copy.
  onEditTags: (path: string) => void;
}

export type { IncomingTabProps };
