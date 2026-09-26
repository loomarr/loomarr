const H264='video/mp4; codecs="avc1.640029,mp4a.40.2"';
export const cases=[
 {name:'A→B same format, drain (ahead 0.4 s)',cfg:{to:'B',mode:'queue',ahead:0.4,switchAt:4,playB:4,codecs:H264}},
 {name:'A→B same format, drain (ahead 2 s)',cfg:{to:'B',mode:'queue',ahead:2,switchAt:4,playB:4,codecs:H264}},
 {name:'A→B same format, flush future buffer',cfg:{to:'B',mode:'flush',ahead:2,switchAt:4,playB:4,codecs:H264}},
 {name:'A→C 480p (avcC/size change), drain',cfg:{to:'C',mode:'queue',ahead:0.4,switchAt:4,playB:4,codecs:H264}},
 {name:'A→B no timestampOffset (control)',cfg:{to:'B',mode:'naive',ahead:0.4,switchAt:4,playB:4,codecs:H264}}];

export const summarise=(browser,c,r)=>{
  const F=r.frames;const t0c=F[0].rgb;const isB=f=>Math.abs(f.rgb[0]-t0c[0])+Math.abs(f.rgb[1]-t0c[1])+Math.abs(f.rgb[2]-t0c[2])>60;
  // The splice window: 15 frames either side of the first NEW-channel frame presented after the control point.
  const k=F.findIndex(f=>f.w>=r.tCtl&&isB(f));
  let worst=0,base=0;const win=(i)=>k>=0&&i>=k-15&&i<=k+15;
  for(let i=1;i<F.length;i++){const ex=(F[i].w-F[i-1].w)-Math.max((F[i].mt-F[i-1].mt)*1000,0);
    if(win(i))worst=Math.max(worst,ex);else base=Math.max(base,ex)}
  return ({browser,case:c.name,frames:F.length,cut:+r.cut.toFixed(3),offset:+r.offset.toFixed(3),
    spliceWindowWorstExtraMs:Math.round(worst),elsewhereWorstExtraMs:Math.round(base),
    ctlToFirstNewFrameMs:k>=0?Math.round(F[k].w-r.tCtl):null,mediaTimeStep:k>0?+(F[k].mt-F[k-1].mt).toFixed(3):null,
    events:r.events.filter(e=>!['playing','waiting','resize'].includes(e.e)||(e.e!=='playing'&&e.at>500&&(k<0||e.at<F[k+15]?.w+50))).map(e=>e.e+'@'+Math.round(e.at)).join(',')||'none',
    buffered:r.finalBuffered.map(x=>x.map(y=>+y.toFixed(2))),quality:r.quality});
};
