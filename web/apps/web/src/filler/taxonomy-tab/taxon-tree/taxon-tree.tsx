import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import { Action, Badge, Details, Icon, icons, Stack, Text, TextLink } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import type { TaxonomyAxis as Axis } from "../../taxonomy-axes";
import { AXIS_COPY } from "../taxonomy-axis-copy";

type TaxonTreeNode = { taxon: TaxonDTO; children: TaxonTreeNode[] };

const treeForAxis = (taxa: TaxonDTO[], axis: Axis): TaxonTreeNode[] => {
  const nodes = taxa.filter((taxon) => taxon.axis === axis);
  const slugs = new Set(nodes.map((taxon) => taxon.slug));
  const children = new Map<string, TaxonDTO[]>();
  for (const taxon of nodes) {
    const parent = taxon.parent && slugs.has(taxon.parent) ? taxon.parent : "";
    children.set(parent, [...(children.get(parent) ?? []), taxon]);
  }
  for (const rows of children.values()) rows.sort((a, b) => a.label.localeCompare(b.label));
  const build = (parent: string): TaxonTreeNode[] =>
    (children.get(parent) ?? []).map((taxon) => ({ taxon, children: build(taxon.slug) }));
  return build("");
};

const TaxonTree = ({
  nodes,
  axis,
  isAdmin,
  onEdit,
  nested = false,
}: {
  nodes: TaxonTreeNode[];
  axis: Axis;
  isAdmin: boolean;
  onEdit: (taxon: TaxonDTO) => void;
  nested?: boolean;
}) => (
  <Stack
    borderColor="borderDecorative"
    borderLeftWidth={nested ? 1 : 0}
    divided={!nested}
    marginLeft={nested ? 20 : 0}
    render="ul"
    {...(!nested ? { "aria-label": `${AXIS_COPY[axis].label} vocabulary` } : {})}
  >
    {nodes.map(({ taxon, children }) => {
      const synonyms = taxon.synonyms ?? [];
      const aliases = taxon.retiredAliases ?? [];
      const resolverTermCount = synonyms.length + aliases.length;
      return (
        <li key={taxon.slug}>
          <Stack align="start" direction="row" gap="inline" paddingHorizontal={16} paddingVertical={12}>
            {nested ? (
              <Stack direction="row" marginTop={4} render="span">
                <Icon decorative glyph={icons.rowExpand} size="compact" tone="muted" />
              </Stack>
            ) : null}
            <Stack flexBasis="0%" flexGrow={1} minWidth={0}>
              <Action
                disabled={!isAdmin}
                layout="text"
                onPress={() => onEdit(taxon)}
                style={{ alignSelf: "flex-start" }}
                variant="ghost"
              >
                {taxon.label}
              </Action>
              <Stack align="center" direction="row" gap="inline" marginTop={2} wrap>
                <Text as="span" textRole="mono" wordBreak="break-all">
                  {taxon.slug}
                </Text>
                {resolverTermCount > 0 ? (
                  <Details
                    summary={`${resolverTermCount} classifier ${resolverTermCount === 1 ? "term" : "terms"}`}
                    textRole="hint"
                    tone="secondary"
                  >
                    <Stack gap="1" marginTop={4} maxWidth={384} style={{ overflowWrap: "break-word" }}>
                      {synonyms.length > 0 ? (
                        <Text as="p" textRole="hint" tone="secondary">
                          Synonyms: {synonyms.join(", ")}
                        </Text>
                      ) : null}
                      {aliases.length > 0 ? (
                        <Text as="p" textRole="hint" tone="secondary">
                          Retired slugs: {aliases.join(", ")}
                        </Text>
                      ) : null}
                    </Stack>
                  </Details>
                ) : null}
              </Stack>
            </Stack>
            <Stack align="center" direction="row" flexShrink={0} gap="1.5" justify="end" wrap>
              {(taxon.assertedClips ?? 0) > 0 && taxon.assertedClips !== taxon.matchedClips ? (
                <Badge shape="square" title="Clips directly assigned this tag" tone="neutral">
                  {taxon.assertedClips.toLocaleString()} direct
                </Badge>
              ) : null}
              <Text as="span" textRole="mono">
                <TextLink
                  render={
                    <Link
                      to="/filler/library"
                      search={{ taxon: taxon.slug }}
                      title={`Browse clips matching ${taxon.label}, including descendants`}
                    />
                  }
                  tone="primary"
                  underline="hover"
                >
                  {taxon.matchedClips.toLocaleString()} {taxon.matchedClips === 1 ? "clip" : "clips"}
                </TextLink>
              </Text>
            </Stack>
          </Stack>
          {children.length > 0 ? (
            <TaxonTree nodes={children} axis={axis} isAdmin={isAdmin} onEdit={onEdit} nested />
          ) : null}
        </li>
      );
    })}
  </Stack>
);

export type { TaxonTreeNode };
export { TaxonTree, treeForAxis };
