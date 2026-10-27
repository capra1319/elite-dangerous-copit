const fs=require('fs'),path=require('path'),{pathToFileURL}=require('url');const {chromium}=require('playwright');
const root=path.resolve(__dirname,'..'),assert=(v,m)=>{if(!v)throw Error(m)};
fs.mkdirSync(path.join(process.env.COPIT_VISUAL_OUTPUT || path.join(root,'tmp','readability'),'screenshots'),{recursive:true});
(async()=>{const browser=await chromium.launch({headless:true,channel:'msedge'});try{
const page=await browser.newPage({viewport:{width:1920,height:1080}}),errors=[],requests=[];page.on('pageerror',e=>errors.push(e.message));await page.route(/^https?:/,r=>{requests.push(r.request().url());return r.abort()});
await page.addInitScript(fixtures=>{window.fixtures=fixtures;window.outfitBaseline=structuredClone(fixtures[9].Outfitting);window.current=0;window.prefs={...fixtures[0].Preferences,uiScale:100,scanlines:true};window.go={main:{DesktopApp:{GetState:async()=>({...structuredClone(window.fixtures[window.current]),Preferences:window.prefs}),SelectPage:async p=>window.current=p,SavePreferences:async p=>window.prefs=p,ShowOutfitting:async()=>{},Disarm:async()=>{}}}}},JSON.parse(fs.readFileSync((process.env.COPIT_FRONTEND_FIXTURE || path.join(require('os').tmpdir(),'copit-readability-fixtures.json')),'utf8')));
await page.goto(pathToFileURL(root+'/app/frontend/index.html').href);await page.locator('.cockpit-switch').last().waitFor();
for(const size of [{width:1920,height:1080},{width:1440,height:2560},{width:1280,height:720},{width:960,height:1707},{width:800,height:650}])for(const scale of [100,150,200]){
 await page.setViewportSize(size);
 for(const stage of ['populated','loading','error','empty']){
  await page.evaluate(async({scale,stage})=>{window.current=9;window.prefs.uiScale=scale;const original=window.outfitBaseline;window.fixtures[9]={...window.fixtures[9],Outfitting:{...original,Loading:stage==='loading',Error:stage==='error'?'Test report unavailable. Retry deliberately.':'',Modules:stage==='empty'?[]:original.Modules,Message:stage==='empty'?'No results for these filters.':original.Message}};await refresh()}, {scale,stage});
  assert(await page.locator('#outfitting-view').isVisible(),'Outfitting missing');
  if(stage==='populated')assert(await page.locator('#outfit-modules tr').count()>0,'Populated fixture lost modules');
  if(stage==='loading')assert(await page.locator('#outfitting-cancel').isVisible(),'Cancel hidden while loading');
  if(stage==='error')assert(await page.locator('#outfitting-error').isVisible(),'Lookup error hidden');
  const measure=await page.evaluate(()=>{const main=document.getElementById('main');return {width:main.clientWidth,scroll:main.scrollWidth}});assert(measure.scroll<=measure.width+2,'Outfitting overflow '+JSON.stringify({size,scale,stage,measure}));
 }
}
await page.setViewportSize({width:1920,height:1080});await page.evaluate(async()=>{window.current=0;window.prefs.uiScale=100;await refresh()});
await page.evaluate(async()=>{window.current=5;await refresh()});await page.locator('#station-search-tools summary').click();assert(await page.locator('#station-search-kind').isVisible(),'External search details missing');await page.evaluate(async()=>{window.current=0;await refresh()});
for(const scenario of ['connected','armed','paused','unknown','missing-control','offline']){
 await page.evaluate(scenario=>{const d=structuredClone(window.fixtures[0]);d.Mode='control';d.Preferences=window.prefs;d.Ready=['connected','armed','paused','offline'].includes(scenario);d.Armed=scenario==='armed';d.Paused=scenario==='paused';d.Status=scenario==='unknown'?'SEARCHING · DISARMED':scenario.toUpperCase();d.Health={HullPercent:61,ShieldPercent:scenario==='offline'?0:null,ShieldState:scenario==='offline'?'OFFLINE':scenario==='unknown'?'UNKNOWN':'UP',Offline:scenario==='offline',Source:'Shields: up/down only; percentage not reported. Hull: last reported snapshot.'};if(scenario==='missing-control'){d.Controls.at(-1).Availability='NOT IN LAST LOADOUT';d.Controls.at(-1).Enabled=false}render(d)},scenario);
 const overflow=await page.evaluate(()=>{const main=document.getElementById('main');return main.scrollWidth>main.clientWidth+2});assert(!overflow,'State overflow '+scenario);
 if(scenario==='connected')assert(await page.locator('[data-health-value="shield"]').first().textContent()==='UP','Real shield invented percentage');
 if(scenario==='offline')assert(await page.locator('#shield-offline-banner').isVisible(),'Offline missing');
 if(scenario==='unknown')assert(!await page.locator('#shield-offline-banner').isVisible(),'Unknown retained warning');
 if(scenario==='missing-control')assert(await page.locator('.cockpit-switch[data-availability="unavailable"]').isDisabled(),'Missing utility clickable');
 if(['armed','offline'].includes(scenario))await page.screenshot({path:path.join(process.env.COPIT_VISUAL_OUTPUT || path.join(root,'tmp','readability'),'screenshots','state-'+scenario+'.png'),scale:'css'});
}
await page.evaluate(async()=>{window.current=0;window.prefs.uiScale=200;await refresh()});await page.locator('#open-control-picker').click();await page.locator('#close-control-picker').scrollIntoViewIfNeeded();assert(await page.locator('#close-control-picker').isVisible(),'Picker close unreachable');await page.keyboard.press('Escape');
// Text palette contrast against every shared surface, plus warning/gauge colors.
const contrast=await page.evaluate(()=>{
 const hex=s=>{const canvas=document.createElement('canvas'),ctx=canvas.getContext('2d');ctx.fillStyle=s;ctx.fillRect(0,0,1,1);return [...ctx.getImageData(0,0,1,1).data].slice(0,3)};
 const luminance=s=>hex(s).map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((v,n,i)=>v+n*[.2126,.7152,.0722][i],0);
 const results=[];for(const theme of ['command-deck','flight-console','retro-ops']){document.documentElement.dataset.theme=theme;const css=getComputedStyle(document.documentElement);for(const token of ['--text','--muted','--accent','--green','--danger','--caution'])for(const bg of ['--bg','--panel','--panel-hi']){const a=luminance(css.getPropertyValue(token)),b=luminance(css.getPropertyValue(bg));results.push({theme,token,bg,ratio:(Math.max(a,b)+.05)/(Math.min(a,b)+.05)})}}return results;
});assert(contrast.every(c=>c.ratio>=4.5),'Text contrast below target '+JSON.stringify(contrast.filter(c=>c.ratio<4.5)));fs.writeFileSync(path.join(process.env.COPIT_VISUAL_OUTPUT || path.join(root,'tmp','readability'),'contrast.json'),JSON.stringify(contrast,null,2));
assert(!errors.length,'JS errors '+errors);assert(!requests.length,'Unexpected external request');console.log('PASS: outfitting populated/loading/error/empty states at five sizes and three UI scales; connected/armed/paused/unknown/missing/offline states; scaled picker; all shared text colors >=4.5:1 on all shared theme surfaces.');
}finally{await browser.close()}})().catch(e=>{console.error(e);process.exitCode=1});
