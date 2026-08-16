import { readdir, readFile } from "node:fs/promises";
import path from "node:path";

// Where Starlight BELIEVES the docs collection lives. It derives this internally as
// `<srcDir>/content/<collection>` and strips exactly that prefix off each entry's `filePath`
// to build the sidebar tree, so entries must be reported relative to it even though nothing
// is actually stored there — this loader reads the repository's docs/ tree in place.
const DOCS_COLLECTION_ROOT = "src/content/docs";

/**
 * A content loader that reads the repository's `docs/` tree IN PLACE.
 *
 * WHY THIS EXISTS RATHER THAN A COPY STEP. The obvious way to give Starlight content is to put
 * markdown in `src/content/docs/`. That would mean every page existing twice — once as the file
 * the Go binary embeds and GitHub renders, once as the file the site renders — with a sync step
 * between them. A sync step that is occasionally forgotten produces two versions of one page,
 * which is precisely the drift this whole documentation effort exists to remove. So the site is
 * a renderer, not a second copy: one file, three consumers.
 *
 * WHY NOT JUST `glob({ base: '../docs' })`. Starlight requires a `title` in frontmatter, and
 * `docs/help/*.md` must have NO frontmatter at all — `docs/embed.go` derives a page's title from
 * its first H1, and the in-app viewer renders raw markdown, so a YAML block would print as
 * literal text to an operator mid-troubleshooting. Rather than bend the shipped files to suit
 * one consumer, this loader lifts the H1 into `data.title` and drops it from the body (Starlight
 * renders the title as the page's H1, so leaving it in would produce two).
 *
 * Frontmatter, where a file happens to have it, still wins — the design docs use it.
 */
export const repoDocs = ({ base, include }) => ({
  name: "loomarr-repo-docs",

  async load({ store, parseData, generateDigest, renderMarkdown, logger, watcher }) {
    const root = path.resolve(base);
    store.clear();

    const files = [];
    for (const entry of include) {
      const target = path.join(root, entry);
      files.push(...(await collectMarkdown(target, root)));
    }

    if (files.length === 0) {
      // A silent empty build would publish a site with no pages and report success — the
      // vacuous-green failure this repo keeps relearning. Fail loudly instead.
      throw new Error(
        `repo-docs: no markdown found under ${root}. Check the 'include' list in astro.config.mjs.`,
      );
    }

    for (const filePath of files) {
      const raw = await readFile(filePath, "utf8");
      // `install/index.md` addresses as `install`, matching how Astro's own glob loader and
      // every static site treat an index file. Without this the sidebar has to link
      // `install/index`, which is both ugly and a 404 waiting for someone to tidy it.
      const id = path
        .relative(root, filePath)
        .replace(/\.md$/, "")
        .replace(/\\/g, "/")
        .replace(/(^|\/)index$/, "");

      const { frontmatter, content } = splitFrontmatter(raw);
      const { title, body } = liftTitle(content);

      const resolvedTitle = frontmatter.title ?? title;
      if (!resolvedTitle) {
        // Starlight cannot render a page with no title, and guessing from the filename would
        // hide the real problem: a documentation page with no H1.
        throw new Error(`repo-docs: ${id} has neither frontmatter title nor an H1 heading.`);
      }

      const data = await parseData({
        id,
        data: { ...frontmatter, title: resolvedTitle },
      });

      // ⚠ `filePath` USED to be omitted here, on the reasoning that it is optional metadata
      // (edit links, error messages) and that anything honest would start with `../`, since
      // these files live outside this project by design.
      //
      // Starlight 0.39+ made it mandatory in practice: sidebar autogeneration calls
      // `route.entry.filePath.replace(...)` with no guard, so an omitted value is a build
      // crash ("Cannot read properties of undefined") pointing at Starlight internals rather
      // than at this loader.
      //
      // It is set to the collection-relative path — the shape Starlight parses to derive the
      // sidebar tree — rather than a real on-disk path. A `../` prefix would survive the
      // `replace` and put every page at the wrong depth in the tree.
      store.set({
        id,
        data,
        body,
        filePath: `${DOCS_COLLECTION_ROOT}/${path.relative(root, filePath)}`,
        digest: generateDigest(body),
        rendered: await renderMarkdown(body),
      });
    }

    logger.info(`Loaded ${files.length} pages from ${path.relative(process.cwd(), root)}/`);

    // `docs/` sits outside this project, so Astro's own dev-server watcher does not cover it.
    // Without this, editing a page in dev looks like the site is ignoring you.
    if (watcher) {
      watcher.add(root);
    }
  },
});

const collectMarkdown = async (target, root) => {
  const out = [];
  let entries;
  try {
    entries = await readdir(target, { withFileTypes: true });
  } catch {
    // A single file rather than a directory (docs/configuration.md).
    return target.endsWith(".md") ? [target] : [];
  }
  for (const entry of entries) {
    const full = path.join(target, entry.name);
    if (entry.isDirectory()) {
      out.push(...(await collectMarkdown(full, root)));
    } else if (entry.name.endsWith(".md")) {
      out.push(full);
    }
  }
  return out;
};

/**
 * Minimal YAML frontmatter split. Deliberately not a YAML dependency: the only frontmatter in
 * this tree is flat `key: value`, and pulling in a parser to read it would be a dependency
 * bought for a case that does not exist.
 */
const splitFrontmatter = (raw) => {
  const match = raw.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?/);
  if (!match) return { frontmatter: {}, content: raw };

  const frontmatter = {};
  for (const line of match[1].split(/\r?\n/)) {
    const kv = line.match(/^([A-Za-z0-9_-]+):\s*(.*)$/);
    if (kv) frontmatter[kv[1]] = kv[2].replace(/^["']|["']$/g, "").trim();
  }
  return { frontmatter, content: raw.slice(match[0].length) };
};

/** Lift the first H1 out of the body and return it as the title. */
const liftTitle = (content) => {
  const match = content.match(/^#\s+(.+?)\s*$/m);
  if (!match) return { title: null, body: content };
  return {
    title: match[1].replace(/`/g, "").trim(),
    body: content.slice(0, match.index) + content.slice(match.index + match[0].length),
  };
};
