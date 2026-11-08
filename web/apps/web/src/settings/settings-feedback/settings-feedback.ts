import type { SettingResult } from "@loomarr/api/models/settingResult";
import { toProblem } from "@loomarr/api/mutator";
import { toast } from "sonner";

// One saved / failed vocabulary for every Settings write, so a change is confirmed the same way
// whether it was staged behind the save bar or committed on the spot (an environment hand-back,
// a cleared value, a model pick). The toast is the confirmation: the "changed by" line under a
// field only shows on hover and is easy to miss on a long page.

// A save that reached the server. Per-key results decide the wording; the caller still owns
// clearing its buffer.
const announceSettingsResults = (results: readonly SettingResult[]) => {
  const saved = results.filter((result) => result.status === "saved").length;
  if (saved > 0) toast.success(saved === 1 ? "Change saved" : "Changes saved");
  if (results.length > saved) toast.error("Some changes need your attention");
};

// A save that never reached a verdict (network, auth, server error).
const announceSettingsFailure = (error: unknown, headline = "Couldn't save your changes") => {
  toast.error(headline, { description: toProblem(error).title });
};

// A change committed on the spot rather than through the save bar.
const announceSettingsChange = (message: string) => {
  toast.success(message);
};

export { announceSettingsChange, announceSettingsFailure, announceSettingsResults };
