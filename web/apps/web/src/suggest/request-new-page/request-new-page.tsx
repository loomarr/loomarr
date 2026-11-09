import * as channelsApi from "@loomarr/api/endpoints/channels";
import { Action, ScrollFrame, Surface, semanticSpace, useWindowWidth } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";
import { useAuth } from "@/auth/use-auth";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useDocumentTitle } from "@/lib/use-document-title";
import { ChannelSuggestPanel } from "../channel-suggest-panel";
import { originationHeading } from "../origination-flow";
import { useOriginationFlow } from "../use-origination-flow";
import type { RequestNewPageProps } from "./request-new-page.type";

// What is open on top of the page and owns Escape for itself: a dialog, a list, a menu.
const OWNS_ESCAPE = '[role="dialog"], [role="listbox"], [role="menu"]';

// The web shell's form tier (`--container-form` in styles.css) and Tailwind's `sm` breakpoint.
const FORM_WIDTH = 672;
const PAGE_GUTTER_BREAKPOINT = 640;

// Request a channel (§12), on its own path under Requests: the describe → review → approve flow,
// opened from the Guide's origination door. The flow's state is the route's search params (useOriginationFlow);
// this page's one PageHeader names the current step, and the panel nests no heading of its own.
// Approving creates the channel and the operator lands on it.
const RequestNewPage = ({ search }: RequestNewPageProps) => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  const flow = useOriginationFlow(search);
  const heading = originationHeading(flow.stage, isAdmin);
  const windowWidth = useWindowWidth();
  // The builder reached from a channel idea is "Edit before you request"; its own sentence under
  // the title says what the builder is, so the describe step's description would only contradict it.
  const editingIdea = flow.manual && search.ideaId !== undefined;
  useDocumentTitle(heading.title);

  const onCreated = (id: string) => {
    void queryClient.invalidateQueries({ queryKey: channelsApi.getListChannelsQueryKey() });
    void navigate({ to: "/channels/$id", params: { id } });
  };

  // Escape closes, as the Close button does: the Job stays remembered, an unsubmitted draft does not.
  // Not when something on top owns the key (a dialog or an open list), which closes itself.
  const close = flow.close;
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      if (event.target instanceof Element && event.target.closest(OWNS_ESCAPE)) return;
      if (document.querySelector(OWNS_ESCAPE)) return;
      close();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [close]);

  return (
    <Surface backgroundColor="transparent" borderWidth={0} height="100%">
      <PageHeader
        title={editingIdea ? "Edit before you request" : heading.title}
        description={flow.manual ? undefined : heading.description}
        actions={
          <Action icon="close" onPress={flow.close} variant="outline">
            Close
          </Action>
        }
      />
      {/* The panel sits in the centred form tier (#1817 Q10), inside the page-edge contract's
          24 px gutter (16 below the 640 px breakpoint). */}
      <ScrollFrame
        contentContainerStyle={{
          paddingHorizontal: windowWidth >= PAGE_GUTTER_BREAKPOINT ? semanticSpace.section : 16,
          paddingVertical: 18,
        }}
      >
        <Surface
          alignSelf="center"
          backgroundColor="transparent"
          borderWidth={0}
          maxWidth={FORM_WIDTH}
          width="100%"
        >
          <ChannelSuggestPanel flow={flow} onCreated={onCreated} />
        </Surface>
      </ScrollFrame>
    </Surface>
  );
};

export { RequestNewPage };
