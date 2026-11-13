import { Stack, Table, TableBody, TableCell, TableRow, Text } from "@loomarr/design-system";
import { webTokenSet } from "@loomarr/design-system/web-theme";

// Tokens — the complete web token set, as data (§2.5, §5.1a).
//
// The Palette/Typography/Spacing pages EXPLAIN the system; this one is the reference you scan
// when you need the exact name of a thing. It reads `webTokenSet()` — the data theme.css is
// generated from — so a token that exists at build time appears here and one that does not,
// cannot.
//
// Deliberately dumb: no curation, no grouping by meaning, no commentary. The moment this page
// starts choosing what to show, it stops being a reliable index of what exists.

// A group is usually name → value, but `tintSteps` is a bare array of percentages and the type and
// motion groups nest (a text style holds its size, weight and leading). Every shape is flattened to
// one row per leaf, keyed by its dotted path, rather than any being skipped or printed as
// "[object Object]": an index that quietly omits a token is worse than no index, because it
// teaches you to trust it.
type TokenLeaf = string | number;
type TokenGroup = TokenLeaf | readonly TokenGroup[] | { readonly [key: string]: TokenGroup };

const asRows = (group: TokenGroup, prefix = ""): [string, TokenLeaf][] => {
  if (typeof group === "string" || typeof group === "number") return [[prefix, group]];
  const entries = Array.isArray(group)
    ? group.map((v, i) => [String(i), v] as const)
    : Object.entries(group as { readonly [key: string]: TokenGroup });
  return entries.flatMap(([key, value]) => asRows(value, prefix ? `${prefix}.${key}` : key));
};

const isColor = (v: TokenLeaf) => typeof v === "string" && v.startsWith("#");

const GroupTable = ({ name, group }: { name: string; group: TokenGroup }) => (
  <Stack gap="inline" render="section">
    <h2 className="font-mono text-base font-semibold">{name}</h2>
    <Table caption={`${name} tokens`} minWidth={384}>
      <TableBody>
        {asRows(group).map(([key, value]) => (
          <TableRow key={key}>
            <TableCell mono rowHeader>
              {key}
            </TableCell>
            <TableCell mono>{value}</TableCell>
            <TableCell width={48}>
              {isColor(value) && (
                <span
                  className="border-border block size-5 rounded border"
                  style={{ backgroundColor: String(value) }}
                />
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </Stack>
);

const Tokens = () => {
  const groups = Object.entries(webTokenSet()) as [string, TokenGroup][];
  const total = groups.reduce((n, [, g]) => n + asRows(g).length, 0);

  return (
    <Stack className="p-6" gap="screen">
      <Stack gap="1" render="header">
        <Text as="h1" textRole="title">
          Tokens
        </Text>
        <Text as="p" className="max-w-2xl" textRole="compact" tone="secondary">
          {`All ${total} generated tokens, read from the same data `}
          <code className="font-mono text-xs">theme.css</code>
          {" is built from. If it is not on this page, it does not exist at build time."}
        </Text>
      </Stack>
      {groups.map(([name, group]) => (
        <GroupTable key={name} name={name} group={group} />
      ))}
    </Stack>
  );
};

export { Tokens };
