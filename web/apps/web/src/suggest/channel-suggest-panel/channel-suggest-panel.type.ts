type ChannelSuggestPanelProps = {
  // Called after an approved proposal creates a channel, with the new channel id, so the
  // Guide can navigate to it. Approval is admin-gated server-side (§7); the panel only shows
  // the approve control to admins.
  onCreated: (channelId: string) => void;
  // Seeds the describe form. The wizard's guided first channel hands off with `?intent=`
  // (§13's blank-page killer), which the Guide forwards here. It is a STARTING POINT, not a
  // controlled value — the operator edits it before anything is generated.
  initialIntent?: string;
  // An opaque, server-authorized Journey id from My Requests. Its intent is fetched from the
  // normal private Job endpoint rather than exposed in the URL.
  initialJobId?: string;
  // Opens straight into the manual/AI-off builder (#1817, G2) on mount, skipping the describe
  // form even when AI is configured — Home's "Describe your own channel" explicitly chose the
  // manual path, so this must not second-guess it by trying AI first.
  initialManual?: boolean;
  // Seeds the manual builder from one channel idea (Home's "Edit first"). Implies initialManual.
  initialIdeaId?: string;
  // The Guide owns URL handoff state. A fresh start must clear it as well as this panel's
  // session state, otherwise a reload resumes the Journey the operator discarded.
  onStartFresh?: () => void;
  // Lets the Guide's one PageHeader name the current task instead of nesting another
  // workflow heading inside the panel.
  onStageChange?: (stage: ChannelSuggestionStage) => void;
  className?: string;
};

type ChannelSuggestionStage = "describe" | "generating" | "review" | "updating" | "failed";

export type { ChannelSuggestionStage, ChannelSuggestPanelProps };
