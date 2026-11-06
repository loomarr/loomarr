// Writes generated/theme.css from the web theme tokens: `pnpm --filter @loomarr/design-system generate`
// (`make fe-tokens`). CI fails when the committed file drifts from the tokens.

import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { webThemeCss } from "../web-theme";

const out = join(dirname(fileURLToPath(import.meta.url)), "..", "generated");
mkdirSync(out, { recursive: true });
writeFileSync(join(out, "theme.css"), webThemeCss());
console.log("design-system: wrote generated/theme.css");
