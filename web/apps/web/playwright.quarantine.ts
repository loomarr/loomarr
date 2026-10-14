import { readFileSync } from "node:fs";

// Flake quarantine (#1570 step 3). scripts/flake-quarantine.sh resolves which listed tests have an
// OPEN issue into PLAYWRIGHT_QUARANTINE_FILE ("project<TAB>spec<TAB>title<TAB>issue" lines), and
// PLAYWRIGHT_QUARANTINE picks the run:
//   unset or "off"  every test runs (local runs, and the manual dispatch that measures a flake)
//   "exclude"       the required job: every test except the quarantined ones
//   "only"          the non-blocking quarantine job: only the quarantined ones
// Anything unexpected throws, so a typo can never quietly run less than the required set.

type Project = { name: string; grep?: RegExp | RegExp[]; grepInvert?: RegExp | RegExp[] };

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Playwright matches grep against "<project> <spec> <title>", optionally followed by tags. Anchoring
// the whole string makes a quarantine line match exactly one test in one project.
function testsPattern(project: string, tests: string[]): RegExp {
  const alternatives = tests.map(escapeRegExp).join("|");
  return new RegExp(`^${escapeRegExp(project)} (?:${alternatives})(?: @\\S+)*$`);
}

function readQuarantine(file: string | undefined, projects: Set<string>): Map<string, string[]> {
  if (!file) throw new Error("PLAYWRIGHT_QUARANTINE is set but PLAYWRIGHT_QUARANTINE_FILE is not");
  const byProject = new Map<string, string[]>();
  for (const line of readFileSync(file, "utf8").split("\n")) {
    if (line === "") continue;
    const [project, spec, title, issue, ...extra] = line.split("\t");
    if (!project || !spec || !title || !/^[1-9][0-9]*$/.test(issue ?? "") || extra.length > 0) {
      throw new Error(`malformed quarantine line: ${JSON.stringify(line)}`);
    }
    if (!projects.has(project)) throw new Error(`quarantine names unknown project ${project}`);
    console.log(`flake quarantine: ${project} ${spec} "${title}" (#${issue})`);
    byProject.set(project, [...(byProject.get(project) ?? []), `${spec} ${title}`]);
  }
  return byProject;
}

export function applyFlakeQuarantine<P extends Project>(
  projects: P[],
  env: NodeJS.ProcessEnv = process.env,
): P[] {
  const mode = env.PLAYWRIGHT_QUARANTINE ?? "off";
  if (mode === "off") return projects;
  if (mode !== "exclude" && mode !== "only") {
    throw new Error(`PLAYWRIGHT_QUARANTINE=${mode}: want off, exclude, or only`);
  }
  const quarantined = readQuarantine(env.PLAYWRIGHT_QUARANTINE_FILE, new Set(projects.map((p) => p.name)));
  if (mode === "exclude") {
    return projects.map((project) => {
      const tests = quarantined.get(project.name);
      return tests ? { ...project, grepInvert: testsPattern(project.name, tests) } : project;
    });
  }
  const selected = projects.flatMap((project) => {
    const tests = quarantined.get(project.name);
    return tests ? [{ ...project, grep: testsPattern(project.name, tests) }] : [];
  });
  if (selected.length === 0) throw new Error("PLAYWRIGHT_QUARANTINE=only but no test is quarantined");
  return selected;
}
