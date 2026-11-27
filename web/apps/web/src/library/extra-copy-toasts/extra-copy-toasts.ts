import { toast } from "sonner";

// What removing an extra copy says (#4476 D3). Its toast is the one place the removal can be
// undone; after it, the file waits in the trash folder.

const toastRemoved = (name: string, undo?: () => void) =>
  toast(`Moved ${name} to the trash`, {
    duration: 10_000,
    action: undo ? { label: "Undo", onClick: undo } : undefined,
  });

const toastRestored = (name: string) => toast.success(`Put ${name} back`);

// `refused` when the undo can't happen at all, rather than not just now.
const toastNotRestored = (name: string, refused: boolean) =>
  toast.error(
    `${refused ? "Couldn't undo." : "Couldn't put it back just now."} ${name} is in Loomarr's trash for 30 days.`,
  );

export { toastNotRestored, toastRemoved, toastRestored };
