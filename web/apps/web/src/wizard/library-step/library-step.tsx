import { LibraryWizardStep } from "@loomarr/ui";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useLibraryFolders } from "@/library/use-library-folders";

// Wizard step: "Where's your media?" (#2151 decision 30). The folders Loomarr found through the
// media server, Sonarr and Radarr, each with Use this folder. Choosing one starts adding the media;
// the step shows the progress and never waits for it. Continue needs one folder Loomarr can open.

const LibraryStep = () => {
  const folders = useLibraryFolders();
  const body = folders.roots;
  const current = folders.status;

  if (folders.error && !body) {
    return <ErrorState error={folders.error} onRetry={() => void folders.refresh()} />;
  }
  if (!body || !current) return null;

  return (
    <>
      {folders.error ? <ErrorState error={folders.error} /> : null}
      <LibraryWizardStep
        checkedJustNow={folders.checked}
        checking={folders.checking}
        folders={body.roots}
        onAddFolder={folders.onAddFolder}
        onCheckAgain={folders.onCheckAgain}
        onUseFolder={folders.onUseFolder}
        pathMap={body.pathMap}
        scan={current.scan}
      />
    </>
  );
};

export { LibraryStep };
