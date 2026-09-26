// SPIKE runner: node run.mjs <chromium|firefox|webkit> — one browser, a handful of splices, no suites.
import http from 'node:http';import fs from 'node:fs';import path from 'node:path';import {createRequire} from 'node:module';
const require=createRequire(process.env.PW_FROM||'/home/fictional/Projects/loomarr/web/node_modules/.pnpm/playwright@1.62.1/node_modules/playwright/noop.js');
const pw=require('./index.js');const here=path.dirname(new URL(import.meta.url).pathname);
const srv=http.createServer(staticHandler(fs,path,here)).listen(0);
const port=srv.address().port;const browser=process.argv[2]||'chromium';
import {cases,summarise,staticHandler} from './cases.mjs';
const engines={chromium:pw.chromium,firefox:pw.firefox,webkit:pw.webkit};if(!Object.hasOwn(engines,browser))throw new Error('usage: node run.mjs chromium|firefox|webkit');
const b=await engines[browser].launch({args:browser==='chromium'?['--autoplay-policy=no-user-gesture-required']:[]});
for(const c of cases){const p=await (await b.newContext()).newPage();
  await p.goto(`http://localhost:${port}/`);
  const ok=await p.evaluate(()=>!!window.MediaSource);if(!ok){console.log(browser,'NO MSE');break}
  let r;try{r=await p.evaluate(x=>run(x),c.cfg)}catch(e){console.log(c.name,'THREW',String(e).slice(0,200));continue}
  console.log(JSON.stringify(summarise(browser,c,r)));
  await p.context().close()}
await b.close();srv.close();
