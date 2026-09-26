// SPIKE runner: node run.mjs <chromium|firefox|webkit> — one browser, a handful of splices, no suites.
import http from 'node:http';import fs from 'node:fs';import path from 'node:path';import {createRequire} from 'node:module';
const require=createRequire(process.env.PW_FROM||'/home/fictional/Projects/loomarr/web/node_modules/.pnpm/playwright@1.62.1/node_modules/playwright/noop.js');
const pw=require('./index.js');const here=path.dirname(new URL(import.meta.url).pathname);
const srv=http.createServer((q,r)=>{const f=path.join(here,q.url==='/'?'splice.html':q.url.split('?')[0]);
  fs.readFile(f,(e,d)=>{if(e){r.writeHead(404);r.end();return}
    const s=f.endsWith('.html')?'text/html':'video/mp4';r.writeHead(200,{'content-type':s});r.end(d)})}).listen(0);
const port=srv.address().port;const browser=process.argv[2]||'chromium';
import {cases,summarise} from './cases.mjs';
const b=await pw[browser].launch({args:browser==='chromium'?['--autoplay-policy=no-user-gesture-required']:[]});
for(const c of cases){const p=await (await b.newContext()).newPage();
  await p.goto(`http://localhost:${port}/`);
  const ok=await p.evaluate(()=>!!window.MediaSource);if(!ok){console.log(browser,'NO MSE');break}
  let r;try{r=await p.evaluate(x=>run(x),c.cfg)}catch(e){console.log(c.name,'THREW',String(e).slice(0,200));continue}
  console.log(JSON.stringify(summarise(browser,c,r)));
  await p.context().close()}
await b.close();srv.close();
