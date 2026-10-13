import {
  type ChannelIdeaDTO,
  type Intent,
  ideaAvailability,
  ideaCount,
  ideaReason,
} from "@loomarr/core/requests";
import { intentSchema } from "@loomarr/core/schemas";
import { CHANNEL_TEMPLATES } from "@loomarr/core/templates";
import { Action, Badge, Field, ScrollFrame, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";

import { StatePanel } from "../../state-panel";
import { RequestsBack } from "../requests-back";
import type { RequestChannelProps } from "./request-channel.type";

const PAGE = 3;

// Web's wording for a brief the server can't take yet. Both keep the draft; a member is never told
// to change a setting only an administrator can reach.
const unavailableCopy = (reason: "ai" | "grounding", isAdmin: boolean) =>
  reason === "grounding"
    ? {
        description: isAdmin
          ? "AI is connected. Loomarr also needs TMDB to match your description to real titles. Your draft is saved."
          : "AI is connected, but an administrator needs to connect TMDB before Loomarr can match your description to real titles. Your draft is saved.",
        title: "Connect TMDB to build this channel",
      }
    : {
        description: isAdmin
          ? "Connect an AI service and choose a model for channel suggestions. Your draft is saved."
          : "An administrator needs to finish AI setup before Loomarr can build this channel. Your draft is saved.",
        title: "Finish AI setup",
      };

const IdeaCard = ({
  busy,
  idea,
  nowMs,
  onHide,
  onRequest,
}: {
  busy: boolean;
  idea: ChannelIdeaDTO;
  nowMs: number;
  onHide: () => void;
  onRequest: () => void;
}) => (
  <Surface gap="$inline" level="raised" padding="$control" role="group">
    <Text density="touch" textRole="caption" tone="secondary">
      {ideaReason(idea, false, nowMs)}
    </Text>
    <Text density="touch" textRole="title">
      {idea.name}
    </Text>
    <Text density="touch" textRole="body" tone="secondary">
      {idea.pitch}
    </Text>
    <Text density="touch" textRole="caption">
      {`${ideaCount(idea)} · ${ideaAvailability(idea)}`}
    </Text>
    {idea.requested ? (
      <Badge density="touch" tone="info">
        Waiting for an admin
      </Badge>
    ) : (
      <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$inline">
        <Action
          accessibilityLabel={`Not for me: ${idea.name}`}
          accessibilityRole="button"
          density="touch"
          disabled={busy}
          onPress={onHide}
          style={{ flex: 1 }}
          tone="secondary"
        >
          Not for me
        </Action>
        <Action
          accessibilityLabel={`Request channel: ${idea.name}`}
          accessibilityRole="button"
          density="touch"
          disabled={busy}
          onPress={onRequest}
          style={{ flex: 2 }}
          tone="primary"
        >
          {busy ? "Requesting…" : "Request channel"}
        </Action>
      </Surface>
    )}
  </Surface>
);

// "Request a channel" on the phone, with both of Web's ways in. Channel ideas are the server's cards
// and work with AI off; the typed brief goes to the suggester and, when AI or TMDB isn't set up, keeps
// the draft and says who can fix it. Either way the request then waits for an admin.
const RequestChannel = ({
  brief,
  hiddenIdea,
  ideas,
  ideasStatus,
  initialIntent,
  nowMs,
  onBack,
  onHideIdea,
  onRequestIdea,
  onRetryIdeas,
  onSubmitBrief,
  onUndoHide,
  requestingIdeaId,
  viewer,
}: RequestChannelProps) => {
  const [page, setPage] = useState(0);
  const [description, setDescription] = useState(initialIntent?.description ?? "");
  const [problem, setProblem] = useState<string>();
  const sending = brief.kind === "sending";

  const start = ideas.length > 0 ? (page * PAGE) % ideas.length : 0;
  const shown = [...ideas.slice(start), ...ideas.slice(0, start)].slice(0, PAGE);

  const submit = () => {
    // The same schema Web's form validates with; a resumed request keeps its constraints.
    const parsed = intentSchema.safeParse({ ...initialIntent, description });
    if (!parsed.success) {
      setProblem(parsed.error.issues[0]?.message ?? "Check the channel request and try again.");
      return;
    }
    setProblem(undefined);
    onSubmitBrief(parsed.data as Intent);
  };

  return (
    <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap="$control">
      <RequestsBack onPress={onBack} />
      <ScrollFrame density="touch">
        <Text density="touch" textRole="display">
          Request a channel
        </Text>
        <Text density="touch" textRole="body" tone="secondary">
          Describe what you want to watch. You'll choose the final titles before anything is created.
        </Text>

        <Text density="touch" textRole="headline">
          Channel ideas
        </Text>
        {hiddenIdea ? (
          <Surface
            alignItems="center"
            flexDirection="row"
            gap="$control"
            justifyContent="space-between"
            padding="$control"
          >
            <Text density="touch" textRole="caption" tone="secondary">{`Hid ${hiddenIdea.name}`}</Text>
            <Action accessibilityRole="button" density="touch" onPress={onUndoHide} tone="secondary">
              Undo
            </Action>
          </Surface>
        ) : null}
        {ideasStatus === "loading" ? (
          <StatePanel density="touch" kind="loading" title="Finding ideas from your library" />
        ) : ideasStatus === "error" ? (
          <StatePanel
            action={{ label: "Try again", onPress: onRetryIdeas }}
            density="touch"
            kind="error"
            title="Couldn't load channel ideas"
          />
        ) : ideas.length === 0 ? (
          <Text density="touch" textRole="body" tone="secondary">
            That's every idea for now. New ones appear as your library grows.
          </Text>
        ) : (
          <>
            {shown.map((idea) => (
              <IdeaCard
                busy={requestingIdeaId === idea.id}
                idea={idea}
                key={idea.id}
                nowMs={nowMs}
                onHide={() => onHideIdea(idea.id)}
                onRequest={() => onRequestIdea(idea.id)}
              />
            ))}
            {ideas.length > PAGE ? (
              <Action
                accessibilityRole="button"
                density="touch"
                icon="channels"
                onPress={() => setPage((current) => current + 1)}
                tone="secondary"
              >
                Different ideas
              </Action>
            ) : null}
          </>
        )}

        <Text density="touch" textRole="headline">
          Describe your own
        </Text>
        <Field
          density="touch"
          disabled={sending}
          error={problem}
          label="Describe the channel"
          multiline
          onChangeText={(text) => {
            setDescription(text);
            setProblem(undefined);
          }}
          placeholder="Saturday-morning cartoons like I watched as a kid — bright, silly, kid-safe"
          value={description}
        />
        <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline" role="group">
          <Text density="touch" textRole="caption" tone="secondary">
            Or start from one of these
          </Text>
          {CHANNEL_TEMPLATES.map((template) => (
            <Action
              accessibilityRole="button"
              density="touch"
              disabled={sending}
              key={template.id}
              onPress={() => {
                setDescription(template.description);
                setProblem(undefined);
              }}
              tone="secondary"
            >
              {template.label}
            </Action>
          ))}
        </Surface>
        {brief.kind === "unavailable" ? (
          <StatePanel
            density="touch"
            kind="permission"
            {...unavailableCopy(brief.reason, viewer === "admin")}
          />
        ) : null}
        {brief.kind === "failed" ? (
          <StatePanel
            density="touch"
            description={brief.message}
            kind="error"
            title="Couldn't send that request"
          />
        ) : null}
        <Action accessibilityRole="button" density="touch" disabled={sending} onPress={submit} tone="primary">
          {sending ? "Sending…" : "Suggest titles"}
        </Action>
      </ScrollFrame>
    </Surface>
  );
};

export { RequestChannel };
