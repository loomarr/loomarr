import { Card, Stack, Text } from "@loomarr/design-system";
import type { Review } from "../taxon-editor.type";

const TaxonImpactReview = ({ impact }: { impact: Review["impact"] }) => (
  <Card aria-label="Change impact" aria-live="polite" gap={12} padding={16} tone="signal">
    <Stack gap="1">
      <Text as="h4" textRole="heading">
        Review the impact
      </Text>
      <Text as="p" textRole="compact" tone="secondary">
        {impact.affectedStoredClips > 0
          ? `${impact.affectedStoredClips.toLocaleString()} stored clips may have different inherited classification (${impact.directStoredClips.toLocaleString()} direct, ${impact.descendantStoredClips.toLocaleString()} through descendants).`
          : "No stored clip classifications will change."}
      </Text>
      <Text as="p" textRole="compact" tone="secondary">
        {impact.affectedPlayableClips > 0
          ? `${impact.affectedPlayableClips.toLocaleString()} playable clips will be checked against channel filler rules after this change.`
          : "No playable clips need channel eligibility checks."}
      </Text>
    </Stack>
    {impact.descendants.length > 0 ? (
      <Text as="p" textRole="compact">
        Descendants kept: {impact.descendants.map((node) => node.label).join(", ")}
      </Text>
    ) : null}
    {impact.savedChannelSelections.length > 0 ? (
      <Stack gap="1">
        <Text as="p" textRole="cardLabel">
          Saved channel selections that reference this branch
        </Text>
        <Text as="ul" style={{ listStyleType: "disc", paddingLeft: 20 }} textRole="compact">
          {impact.savedChannelSelections.map((channel) => (
            <li key={channel.id}>
              Channel {channel.number}: {channel.name}
            </li>
          ))}
        </Text>
      </Stack>
    ) : (
      <Text as="p" textRole="compact" tone="secondary">
        No saved channel selections reference this branch.
      </Text>
    )}
    {impact.resolverTermsAdded.length > 0 || impact.resolverTermsRemoved.length > 0 ? (
      <Stack gap="1">
        <Text as="p" textRole="cardLabel">
          Classifier wording
        </Text>
        {impact.resolverTermsAdded.length > 0 ? (
          <Text as="p" textRole="compact">
            Starts resolving: {impact.resolverTermsAdded.join(", ")}
          </Text>
        ) : null}
        {impact.resolverTermsRemoved.length > 0 ? (
          <Text as="p" textRole="compact">
            Stops resolving: {impact.resolverTermsRemoved.join(", ")}
          </Text>
        ) : null}
      </Stack>
    ) : (
      <Text as="p" textRole="compact" tone="secondary">
        Classifier wording is unchanged.
      </Text>
    )}
    {impact.deleteBlocked ? (
      <Text as="p" textRole="compact" tone="warning">
        Retag {impact.directStoredClips.toLocaleString()} directly assigned stored clips before removing this
        tag. This includes clips in Incoming, removed clips, and compilations.
      </Text>
    ) : null}
  </Card>
);

export { TaxonImpactReview };
