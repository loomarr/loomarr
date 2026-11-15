import * as fillerApi from "@loomarr/api/endpoints/filler";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, Card, Grid, GridItem, Stack, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { TAXONOMY_AXES as AXES, type TaxonomyAxis as Axis } from "../taxonomy-axes";
import { type Editor, TaxonEditor } from "./taxon-editor";
import { TaxonTree, type TaxonTreeNode, treeForAxis } from "./taxon-tree";
import { AXIS_COPY } from "./taxonomy-axis-copy";

const TaxonomyTab = ({ isAdmin }: { isAdmin: boolean }) => {
  const query = fillerApi.useListTaxonomy();
  const data = unwrap(query.data, (body) => body);
  const taxa = data?.taxa ?? [];
  const [editor, setEditor] = useState<Editor>();
  const byAxis = useMemo(
    () =>
      Object.fromEntries(AXES.map((axis) => [axis, treeForAxis(taxa, axis)])) as Record<
        Axis,
        TaxonTreeNode[]
      >,
    [taxa],
  );

  if (query.error) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  if (!data) {
    return (
      <Text as="p" textRole="compact" tone="secondary">
        Loading the taxonomy…
      </Text>
    );
  }

  return (
    <Stack gap="section">
      <Grid columns={{ base: 1, sm: 3 }} gap="3">
        <Card gap={4} padding={16}>
          <Text as="p" textRole="hint" tone="secondary">
            Library coverage
          </Text>
          {data.totalClips === 0 ? (
            <>
              <Text as="p" textRole="title">
                No playable clips yet
              </Text>
              <Text as="p" textRole="compact" tone="secondary">
                Coverage begins after clips are ready.
              </Text>
            </>
          ) : (
            <>
              <Text as="p" textRole="display">
                {data.taggedClips.toLocaleString()} / {data.totalClips.toLocaleString()}
              </Text>
              <Text as="p" textRole="compact" tone="secondary">
                playable clips have at least one classification signal
              </Text>
            </>
          )}
        </Card>
        <GridItem span={{ base: 1, sm: 2 }}>
          <Card gap={4} padding={16}>
            <Text as="p" textRole="heading">
              What belongs here
            </Text>
            <Text as="p" textRole="compact" tone="secondary">
              Classification describes what a clip contains. Kind is the closed playout role; format and
              presentation are optional browsing details. Era, audience, and grounded brand remain separate
              facts too.
            </Text>
            <Stack align="start" marginTop={4}>
              {data.totalClips === 0 ? (
                <Text as="p" textRole="compact" tone="secondary">
                  Nothing to classify yet.
                </Text>
              ) : data.unclassifiedClips > 0 ? (
                <Link
                  to="/filler/library"
                  search={{ unclassified: true }}
                  className="inline-flex underline-offset-2 hover:underline"
                >
                  <Text as="span" textRole="compact" tone="signal">
                    Review {data.unclassifiedClips.toLocaleString()} unclassified{" "}
                    {data.unclassifiedClips === 1 ? "clip" : "clips"}
                  </Text>
                </Link>
              ) : (
                <Text as="p" textRole="compact" tone="success">
                  Every playable clip has at least one taxonomy tag.
                </Text>
              )}
            </Stack>
          </Card>
        </GridItem>
      </Grid>

      {editor ? (
        <TaxonEditor
          key={`${editor.mode}:${editor.taxon?.slug ?? editor.axis}`}
          editor={editor}
          taxa={taxa}
          onClose={() => setEditor(undefined)}
        />
      ) : null}

      <section aria-labelledby="classification-coverage-heading">
        <Stack gap="1">
          <Text as="h2" id="classification-coverage-heading" textRole="headline" strong>
            Classification coverage
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            These are independent signals. Missing presentation, seasonal, or audience cues can be perfectly
            normal.
          </Text>
        </Stack>
        {data.totalClips === 0 ? (
          <Card marginTop={12} padding={16}>
            <Text as="p" textRole="compact" tone="secondary">
              Coverage will appear after clips are ready. The vocabulary below is ready when they are.
            </Text>
          </Card>
        ) : (
          <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            {AXES.map((axis) => {
              const coverage = data.axisCoverage?.find((item) => item.axis === axis);
              return (
                <Card gap={4} key={axis} padding={16}>
                  <Text as="h3" textRole="heading">
                    {AXIS_COPY[axis].label}
                  </Text>
                  <Text as="p" textRole="compact" tone="secondary">
                    {AXIS_COPY[axis].help}
                  </Text>
                  <Stack marginTop={4}>
                    <Text as="p" textRole="hint" tone="secondary">
                      {AXIS_COPY[axis].example}
                    </Text>
                  </Stack>
                  {coverage ? (
                    <Stack
                      align="start"
                      borderColor="borderDecorative"
                      borderTopWidth={1}
                      gap="1"
                      marginTop={8}
                      paddingTop={12}
                    >
                      <Text as="p" textRole="compact">
                        <Text as="span" textRole="cardLabel">
                          {coverage.taggedClips.toLocaleString()}
                        </Text>{" "}
                        of {data.totalClips.toLocaleString()} playable clips
                      </Text>
                      {coverage.untaggedClips > 0 ? (
                        <Link
                          to="/filler/library"
                          search={{ withoutAxis: axis }}
                          className="inline-flex underline-offset-2 hover:underline"
                        >
                          <Text as="span" textRole="hint" tone="signal">
                            Browse {coverage.untaggedClips.toLocaleString()} without this signal
                          </Text>
                        </Link>
                      ) : (
                        <Text as="p" textRole="hint" tone="success">
                          All playable clips have this signal.
                        </Text>
                      )}
                    </Stack>
                  ) : null}
                </Card>
              );
            })}
          </div>
        )}
      </section>

      <details className="border-border bg-panel rounded-lg border">
        <Text
          as="summary"
          className="focus-visible:ring-ring cursor-pointer px-4 py-3 focus-visible:ring-2 focus-visible:outline-none"
          textRole="heading"
        >
          {isAdmin ? "Manage vocabulary" : "Browse vocabulary"}
        </Text>
        <Stack borderColor="borderDecorative" borderTopWidth={1} gap="4" padding={16}>
          <Text as="p" textRole="compact" tone="secondary">
            {isAdmin
              ? "Advanced: change the hierarchy and the words classifiers resolve. Review the impact before any edit is applied."
              : "This hierarchy is read-only. Administrators can change classifier vocabulary and relationships."}
          </Text>
          <div className="grid gap-4 xl:grid-cols-2">
            {AXES.map((axis) => {
              return (
                <Card key={axis} overflow="hidden">
                  <Stack
                    align="start"
                    borderBottomWidth={1}
                    borderColor="borderDecorative"
                    direction="row"
                    gap="control"
                    padding={16}
                  >
                    <Stack flexBasis="0%" flexGrow={1} minWidth={0}>
                      <Text as="h3" textRole="heading">
                        {AXIS_COPY[axis].label}
                      </Text>
                      <Stack marginTop={2}>
                        <Text as="p" textRole="compact" tone="secondary">
                          {AXIS_COPY[axis].help}
                        </Text>
                      </Stack>
                      <Stack marginTop={4}>
                        <Text as="p" textRole="hint" tone="secondary">
                          {AXIS_COPY[axis].example}
                        </Text>
                      </Stack>
                    </Stack>
                    {isAdmin ? (
                      <Action
                        icon="plus"
                        onPress={() => setEditor({ mode: "create", axis })}
                        variant="outline"
                      >
                        Add
                      </Action>
                    ) : null}
                  </Stack>
                  {byAxis[axis].length === 0 ? (
                    <Stack paddingHorizontal={16} paddingVertical={12}>
                      <Text as="p" textRole="compact" tone="secondary">
                        No terms yet.
                      </Text>
                    </Stack>
                  ) : (
                    <TaxonTree
                      nodes={byAxis[axis]}
                      axis={axis}
                      isAdmin={isAdmin}
                      onEdit={(taxon) => isAdmin && setEditor({ mode: "edit", axis, taxon })}
                    />
                  )}
                </Card>
              );
            })}
          </div>
        </Stack>
      </details>
    </Stack>
  );
};

export { TaxonomyTab };
