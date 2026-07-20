import { anchorOf } from "@loomarr/core";
import type { ReactNode } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { cn } from "@/lib";
import type { DocViewProps } from "./doc-view.type";

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

// DocView renders one embedded help page. Styling is explicit per element rather than a
// prose plugin: the design system already defines type scale and color, and a generic
// typography reset would fight it.
const DocView = ({ markdown, className }: DocViewProps) => (
  <article className={cn("flex flex-col gap-4", className)}>
    <Markdown
      remarkPlugins={[remarkGfm]}
      components={{
        h1: ({ children }) => (
          <h2 id={headingId(children)} className="font-semibold text-xl">
            {children}
          </h2>
        ),
        h2: ({ children }) => (
          <h3 id={headingId(children)} className="mt-4 scroll-mt-20 font-semibold text-lg">
            {children}
          </h3>
        ),
        h3: ({ children }) => (
          <h4 id={headingId(children)} className="mt-3 scroll-mt-20 font-semibold">
            {children}
          </h4>
        ),
        p: ({ children }) => <p className="text-sm leading-relaxed">{children}</p>,
        ul: ({ children }) => <ul className="ml-5 flex list-disc flex-col gap-1.5 text-sm">{children}</ul>,
        ol: ({ children }) => <ol className="ml-5 flex list-decimal flex-col gap-1.5 text-sm">{children}</ol>,
        li: ({ children }) => <li className="leading-relaxed">{children}</li>,
        code: ({ children }) => (
          <code className="rounded-sm bg-static-800 px-1 py-0.5 font-mono text-[0.85em]">{children}</code>
        ),
        pre: ({ children }) => (
          <pre className="overflow-x-auto rounded-md border border-border bg-static-900 p-3 font-mono text-xs">
            {children}
          </pre>
        ),
        strong: ({ children }) => <strong className="font-semibold">{children}</strong>,
        blockquote: ({ children }) => (
          <blockquote className="border-static-700 border-l-2 pl-3 text-muted-foreground text-sm">
            {children}
          </blockquote>
        ),
        // Docs ship offline, so an external link is a deliberate exit. Marked as such
        // rather than silently stealing the tab.
        a: ({ children, href }) => (
          <a
            href={href}
            className="text-suggest-300 underline underline-offset-2"
            {...(href?.startsWith("http") ? { target: "_blank", rel: "noreferrer" } : {})}
          >
            {children}
          </a>
        ),
        table: ({ children }) => (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm">{children}</table>
          </div>
        ),
        th: ({ children }) => (
          <th className="border-border border-b p-2 text-left font-medium">{children}</th>
        ),
        td: ({ children }) => <td className="border-border/50 border-b p-2 align-top">{children}</td>,
      }}
    >
      {markdown}
    </Markdown>
  </article>
);

export { DocView };
