// SPIKE: Playwright's Firefox build ships without H.264, so drive the SYSTEM Firefox over WebDriver BiDi.
// node run-firefox-bidi.mjs  (serves ./ and ./out, runs the same cases as run.mjs, prints the same JSON)
import http from 'node:http';import fs from 'node:fs';import path from 'node:path';import {spawn} from 'node:child_process';import os from 'node:os';
import {cases,summarise,staticHandler} from './cases.mjs';
const here=path.dirname(new URL(import.meta.url).pathname);
