// SPIKE: Playwright's Firefox build ships without H.264, so drive the SYSTEM Firefox over WebDriver BiDi.
// node run-firefox-bidi.mjs  (serves ./ and ./out, runs the same cases as run.mjs, prints the same JSON)
import http from 'node:http';import fs from 'node:fs';import path from 'node:path';import {spawn} from 'node:child_process';import os from 'node:os';
import {cases,summarise,staticHandler} from './cases.mjs';
const here=path.dirname(new URL(import.meta.url).pathname);
const srv=http.createServer(staticHandler(fs,path,here)).listen(0);
const port=srv.address().port,dbg=9333;const prof=fs.mkdtempSync(path.join(os.tmpdir(),'ffprof-'));
fs.writeFileSync(path.join(prof,'user.js'),'user_pref("media.autoplay.default",0);user_pref("media.autoplay.blocking_policy",0);user_pref("media.mediasource.enabled",true);\n');
const ff=spawn('firefox',['--headless','--no-remote','--profile',prof,'--remote-debugging-port='+dbg],{stdio:'ignore'});
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
let ws;for(let i=0;i<60;i++){try{ws=new WebSocket(`ws://127.0.0.1:${dbg}/session`);await new Promise((res,rej)=>{ws.onopen=res;ws.onerror=rej});break}catch{await sleep(500)}}
let id=0;const pend=new Map();ws.onmessage=m=>{const d=JSON.parse(m.data);const cb=typeof d.id==="number"?pend.get(d.id):undefined;if(typeof cb==="function")cb(d)};
const send=(method,params)=>new Promise(res=>{const i=++id;pend.set(i,res);ws.send(JSON.stringify({id:i,method,params}))});
await send('session.new',{capabilities:{}});
const {result:{context}}=await send('browsingContext.create',{type:'tab'});
for(const c of cases){
  await send('browsingContext.navigate',{context,url:`http://localhost:${port}/`,wait:'complete'});
  const e=await send('script.evaluate',{target:{context},awaitPromise:true,resultOwnership:'none',serializationOptions:{maxObjectDepth:12},
    expression:`run(${JSON.stringify(c.cfg)}).then(r=>JSON.stringify(r),e=>JSON.stringify({error:String(e)}))`});
  const v=e.result?.result?.value;if(!v){console.log(c.name,'FAILED',JSON.stringify(e).slice(0,300));continue}
  const r=JSON.parse(v);if(r.error){console.log(c.name,'THREW',r.error.slice(0,200));continue}
  console.log(JSON.stringify(summarise('firefox',c,r)));
}
ff.kill();srv.close();process.exit(0);
