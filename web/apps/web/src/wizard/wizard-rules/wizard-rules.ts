const COPY: Record<string, { title: string; description: string }> = {
  bootstrap: {
    title: "Create your admin account",
    description: "This account owns Loomarr. You can set it up before connecting a media server.",
  },
  playout: {
    title: "How should Loomarr play your channels?",
    // Says what the answer CHANGES, because it changes the rest of the wizard. An operator
    // who does not know what Tunarr is should still be able to answer confidently.
    description: "This decides what you'll need to set up next. Most people should keep the default.",
  },
  location: {
    title: "Where are your channels watched?",
    description: "This helps Loomarr find filler for your country and local area.",
  },
  checklist: {
    title: "Connect your services",
    description: "Loomarr live-tests each dependency. A red check tells you exactly what to fix.",
  },
  media: {
    title: "Where's your media?",
    description: "So Loomarr can put your shows and movies on the air.",
  },
  library: {
    title: "Give Tunarr your library",
    description: "Without this, Tunarr schedules slots that have no programme to play.",
  },
  users: {
    title: "Import media-server users",
    description:
      "Only the accounts you pick can sign in. You can skip this if you're the only one using Loomarr.",
  },
  channel: {
    title: "Your first channel",
    description: "Pick a starting point. You can edit it before Loomarr builds anything.",
  },
};

// A step the operator may pass on. Skipped reads neutral, never red (§6).
//
// The blocking set is "the shortest honest path to a live channel" (§13, config-design §6),
// which depends on the playout backend (§9.1, see requiredChecks in ../steps). A step that gates on
// a check the operator cannot satisfy is a dead end, because the wizard offers only
// Back/Continue and the rail goes back, never ahead: they'd be stranded on that screen for good.
// That matters most for `library`, whose entire purpose (§6) is to stop channels scheduling slots
// with no programme, and most of all for `tunarr`, which an internal-playout install can
// never turn green. (Live TV is not a step: it auto-wires on the Tunarr save, so it can't
// strand anyone here either.)
const SKIPPABLE: ReadonlySet<string> = new Set(["media", "library", "users"]);

// Skippable steps whose Continue still waits for their own work, as the mock draws it: Skip is
// the way past, Continue says the work is done. The Library step is skippable only until the
// media server's catalog is retired (#2151 step 16).
const CONTINUE_WAITS: ReadonlySet<string> = new Set(["media"]);

// A skippable step has no server check to satisfy, so it never BLOCKS Continue unless it is one
// that waits for its own work.
const continueGated = (id: string): boolean => !SKIPPABLE.has(id) || CONTINUE_WAITS.has(id);

export { COPY, continueGated, SKIPPABLE };
