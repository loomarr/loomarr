import { anchorOf, parseDocHref } from "@loomarr/core/anchor";
import {
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  Text,
  TextLink,
} from "@loomarr/design-system";
import type { ReactNode } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { DocViewProps } from "./doc-view.type";

// An internal doc link is a relative one: not an absolute URL, and not a bare same-page
// "#anchor" (which the browser scrolls on its own). Those are the cross-page links the
// help set uses ("concepts", "member-guide#section") and must be ROUTED, not followed as
// a raw href — see parseDocHref.
const isInternal = (href?: string): href is string =>
  !!href && !/^[a-z]+:\/\//i.test(href) && !href.startsWith("#") && !href.startsWith("/");

// A page shows a diagram or screenshot by its repo path ("../diagrams/generated/architecture.svg",
// "../images/screenshots/guide-dark.webp") so it renders on GitHub and the docs site too. In the
// app the same file ships inside the binary, at /v1/docs/diagrams/<name> or
// /v1/docs/screenshots/<name>, so Help's images work air-gapped. docs/embed_test.go checks
// every image a page shows is embedded.
const embeddedSrc = (src?: string): string | undefined => {
  const diagram = src?.match(/(?:^|\/)diagrams\/generated\/([a-z0-9-]+\.svg)$/)?.[1];
  if (diagram) return `/v1/docs/diagrams/${diagram}`;
  const screenshot = src?.match(/(?:^|\/)images\/screenshots\/([a-z0-9-]+-dark\.webp)$/)?.[1];
  return screenshot ? `/v1/docs/screenshots/${screenshot}` : undefined;
};

// Heading ids are a CONTRACT, not decoration. The API emits `troubleshooting#tunarr`-style
// deep-links on every failed setup check (§13: "every red check deep-links to its
// section"), and a Go test asserts each anchor resolves to a real heading. That guarantee
// only holds if the rendered ids use the same slug transform, so both sides call the one
// in packages/core rather than each rolling their own.
const headingId = (children: ReactNode): string =>
  anchorOf(
    Array.isArray(children)
      ? children.map((c) => (typeof c === "string" ? c : "")).join("")
      : String(children ?? ""),
  );

// A page may carry an editor note in an HTML comment: GitHub and the docs site hide it. Without
// rehype-raw, react-markdown prints raw HTML as text, so the comment would show as body copy.
// This drops comment nodes (block or inline) from the tree and leaves any other HTML alone.
interface MarkdownNode {
  type: string;
  value?: string;
  children?: MarkdownNode[];
}
const isHtmlComment = (node: MarkdownNode): boolean =>
  node.type === "html" && /^<!--[\s\S]*-->$/.test(node.value?.trim() ?? "");
const dropComments = (node: MarkdownNode): void => {
  if (!node.children) return;
  node.children = node.children.filter((child) => !isHtmlComment(child));
  node.children.forEach(dropComments);
};
const remarkDropHtmlComments = () => dropComments;

// DocView renders one embedded help page. Styling is explicit per element rather than a
// prose plugin: the design system already defines type scale and color, and a generic
// typography reset would fight it.
const DocView = ({ markdown, className, onNavigate }: DocViewProps) => (
  <Stack className={className} gap="4" render="article">
    <Markdown
      remarkPlugins={[remarkGfm, remarkDropHtmlComments]}
      components={{
        h1: ({ children }) => (
          <Text as="h2" id={headingId(children)} textRole="title">
            {children}
          </Text>
        ),
        h2: ({ children }) => (
          <Text as="h3" className="mt-4 scroll-mt-20" id={headingId(children)} strong textRole="headline">
            {children}
          </Text>
        ),
        h3: ({ children }) => (
          <Text as="h4" className="mt-3 scroll-mt-20" id={headingId(children)} textRole="heading">
            {children}
          </Text>
        ),
        p: ({ children }) => (
          <Text as="p" textRole="compact">
            {children}
          </Text>
        ),
        ul: ({ children }) => (
          <Stack className="ml-5 list-disc" gap="1.5" render="ul">
            {children}
          </Stack>
        ),
        ol: ({ children }) => (
          <Stack className="ml-5 list-decimal" gap="1.5" render="ol">
            {children}
          </Stack>
        ),
        li: ({ children }) => (
          <Text as="li" textRole="compact">
            {children}
          </Text>
        ),
        code: ({ children }) => (
          <code className="bg-static-800 rounded-sm px-1 py-0.5 font-mono text-[0.85em]">{children}</code>
        ),
        pre: ({ children }) => (
          <Text
            as="pre"
            className="border-border bg-static-900 overflow-x-auto rounded-md border p-3"
            textRole="mono"
            tone="primary"
          >
            {children}
          </Text>
        ),
        strong: ({ children }) => <strong className="font-semibold">{children}</strong>,
        blockquote: ({ children }) => (
          <Text
            as="blockquote"
            className="border-static-700 border-l-2 pl-3"
            textRole="compact"
            tone="secondary"
          >
            {children}
          </Text>
        ),
        // Three link kinds: an INTERNAL cross-page link ("concepts",
        // "member-guide#section") is routed into the Help center (§13 — a raw href would
        // resolve relative to the current path and break); an external URL opens in a new
        // tab (a deliberate exit from the offline docs); a same-page "#anchor" falls
        // through to the browser.
        a: ({ children, href }) =>
          onNavigate && isInternal(href) ? (
            <TextLink
              onPress={() => {
                const { page, section } = parseDocHref(href);
                if (page) onNavigate(page, section);
              }}
            >
              {children}
            </TextLink>
          ) : (
            <TextLink external={href?.startsWith("http")} href={href}>
              {children}
            </TextLink>
          ),
        // Diagrams and screenshots load only through <img>, where an SVG can't run script (the
        // route also serves diagrams sandboxed). Any other image would be a fetch out of the
        // household's network, so it shows as its alt text instead.
        img: ({ src, alt }) => {
          const embedded = embeddedSrc(src);
          return embedded ? (
            <img src={embedded} alt={alt ?? ""} className="my-2 h-auto max-w-full" loading="lazy" />
          ) : (
            <span className="text-muted-foreground">{alt}</span>
          );
        },
        // A page's table is the design system's, as the mock draws it. A Markdown table has no
        // caption of its own, so it is named for what it is.
        table: ({ children }) => <Table caption="Table">{children}</Table>,
        thead: ({ children }) => <TableHeader>{children}</TableHeader>,
        tbody: ({ children }) => <TableBody>{children}</TableBody>,
        tr: ({ children }) => <TableRow>{children}</TableRow>,
        th: ({ children }) => <TableHeaderCell>{children}</TableHeaderCell>,
        td: ({ children }) => <TableCell verticalAlign="top">{children}</TableCell>,
      }}
    >
      {markdown}
    </Markdown>
  </Stack>
);

export { DocView };
