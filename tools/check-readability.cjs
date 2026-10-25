const fs=require('fs'),path=require('path'),{pathToFileURL}=require('url');
const {chromium}=require('playwright');
const root=path.resolve(__dirname,'..');
const out=path.join(process.env.COPIT_VISUAL_OUTPUT || path.join(root,'tmp','readability'),'screenshots');fs.mkdirSync(out,{recursive:true});
const assert=(v,m)=>{if(!v)throw Error(m)};
const profiles=[{id:'1080',width:1920,height:1080,dpr:1},{id:'1080-125',width:1536,height:864,dpr:1.25},{id:'1080-150',width:1280,height:720,dpr:1.5},{id:'portrait',width:1440,height:2560,dpr:1},{id:'portrait-125',width:1152,height:2048,dpr:1.25},{id:'portrait-150',width:960,height:1707,dpr:1.5},{id:'4k',width:3840,height:2160,dpr:1},{id:'window',width:800,height:650,dpr:1}];
(async()=>{const browser=await chromium.launch({headless:true,channel:'msedge'});const errors=[],external=[],results=[];try{
 const context=await browser.newContext();const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));await page.route(/^https?:/,route=>{external.push(route.request().url());return route.abort()});
 await page.addInitScript(fixtures=>{
  window.fixtures=fixtures;window.currentPage=0;window.prefs={...fixtures[0].Preferences,scanlines:true};window.writes=[];window.flightCalls=[];window.referenceWrites=[];
  window.go={main:{DesktopApp:{
   GetState:async()=>({...structuredClone(window.fixtures[window.currentPage]),Preferences:{...window.prefs}}),SelectPage:async p=>window.currentPage=p,SavePreferences:async p=>{window.prefs={...p};window.writes.push({...p})},
   SelectMaterial:async key=>window.fixtures[7].Engineering.Selected=key,FilterMaterials:async()=>{},SetMaterialTarget:async()=>{},PressControl:async id=>window.flightCalls.push(id),Disarm:async()=>{},
   SetFireGroupReference:async(key,group,trigger)=>{window.fixtures[8].PageView.Hardpoints.find(row=>row.Key===key).Groups[group]=trigger;window.referenceWrites.push({key,group,trigger})}
  }}};
 },JSON.parse(fs.readFileSync((process.env.COPIT_FRONTEND_FIXTURE || path.join(require('os').tmpdir(),'copit-readability-fixtures.json')),'utf8')));
 await page.goto(pathToFileURL(root+'/frontend/index.html').href);await page.locator('.cockpit-switch').last().waitFor();
 for(const profile of profiles){await page.setViewportSize({width:profile.width,height:profile.height});
  for(const scale of [100,125,150,175,200])for(const theme of ['command-deck','flight-console','retro-ops'])for(const id of [0,4,5,6,8,7,3,1,2]){
   await page.evaluate(async({id,scale,theme})=>{window.currentPage=id;window.prefs.uiScale=scale;window.prefs.theme=theme;await refresh();document.getElementById('main').scrollTop=0},{id,scale,theme});
   const m=await page.evaluate(()=>{const main=document.getElementById('main'),route=document.querySelector('.cockpit-route');const styles=getComputedStyle(document.documentElement);return{width:main.clientWidth,scroll:main.scrollWidth,height:main.clientHeight,scrollHeight:main.scrollHeight,unit:parseFloat(styles.fontSize),routeBottom:route.getBoundingClientRect().bottom,mainBottom:main.getBoundingClientRect().bottom-parseFloat(getComputedStyle(main).paddingBottom),font:parseFloat(getComputedStyle(document.querySelector('#navigation .navigation-label')).fontSize),tile:document.querySelector('.cockpit-switch')?.getBoundingClientRect().width}});
   assert(m.scroll<=m.width+2,'Horizontal overflow '+JSON.stringify({profile:profile.id,scale,theme,id,m}));
   assert(m.unit===16*scale/100,'UI scale did not apply');assert(Math.abs(m.font-18*scale/100)<.1,'Navigation counters scaling');
   if(id===0&&profile.id==='1080'&&scale===100)assert(m.routeBottom<=m.mainBottom+2,'Default cockpit route is below fold '+JSON.stringify(m));
   if(id===0)assert(Math.abs(m.tile-112*scale/100)<1,'Square control did not scale');
   results.push({profile:profile.id,scale,theme,page:id,...m});
   if(theme==='command-deck'&&[100,150,200].includes(scale)&&['1080','portrait','window'].includes(profile.id)&&[0,4,7,3].includes(id))await page.screenshot({path:path.join(out,profile.id+'-'+scale+'-page-'+id+'.png'),scale:'css'});
  }
  console.log(profile.id+' all pages/themes/scales passed');
 }
 // Preference save is real UI behavior, rather than only modifying fixture data.
 await page.setViewportSize({width:1920,height:1080});await page.evaluate(async()=>{window.currentPage=3;window.prefs.uiScale=100;await refresh()});
 await page.locator('#ui-scale').selectOption('150');await page.waitForFunction(()=>window.writes.at(-1)?.uiScale===150&&document.documentElement.style.fontSize==='24px');
 await page.locator('#reset-ui-scale').click();await page.waitForFunction(()=>window.writes.at(-1)?.uiScale===100&&document.documentElement.style.fontSize==='16px');
 // Editing fire groups stays explicit, retains focus and changes only a manual reference.
 await page.evaluate(async()=>{window.currentPage=8;await refresh()});assert(await page.locator('.firegroup-edit[open]').count()===0,'Reference editors not collapsed');
 await page.locator('.firegroup-edit summary').first().click();const select=page.locator('.firegroup-selectors select').first();await select.focus();await select.selectOption('1');
 await page.waitForFunction(()=>window.referenceWrites.length===1);await page.waitForFunction(()=>document.querySelector('.firegroup-selectors select').value==='1');
 assert(await page.locator('.firegroup-edit').first().getAttribute('open')!==null,'Save closed editor');assert(await page.locator('.firegroup-summary').first().textContent().then(s=>s.includes('A: Primary')),'Reference summary did not update');
 assert(await page.evaluate(()=>document.activeElement?.dataset.group)==='0','Reference save lost focus');
 // Both editor panels stay usable in portrait and at enlarged app text.
 for(const args of [{width:1440,height:2560,scale:100},{width:960,height:1707,scale:150},{width:1280,height:720,scale:200}]){
  await page.setViewportSize({width:args.width,height:args.height});await page.evaluate(async scale=>{window.currentPage=7;window.prefs.uiScale=scale;window.fixtures[7].Engineering.Selected=window.fixtures[7].Engineering.Rows[0].Key;await refresh()},args.scale);
  const details=page.locator('#material-details');assert(await details.isVisible(),'Details missing');await page.locator('#save-material-note').scrollIntoViewIfNeeded();assert(await page.locator('#save-material-note').isVisible(),'Save comment unreachable');
  await page.screenshot({path:path.join(out,'engineering-details-'+args.width+'-'+args.scale+'.png'),scale:'css'});
 }
 // Control picker and native form labels scale; Escape remains a safe close.
 await page.setViewportSize({width:1920,height:1080});await page.evaluate(async()=>{window.currentPage=0;window.prefs.uiScale=150;await refresh()});await page.locator('#open-control-picker').click();assert(await page.locator('#ship-control-picker').isVisible(),'Picker missing');
 await page.locator('#close-control-picker').focus();await page.keyboard.press('Escape');assert(!await page.locator('#ship-control-picker').isVisible(),'Escape did not close picker');
 // Warning + motion behavior is unchanged and decorations are below text.
 await page.evaluate(()=>renderShipHealth({Preferences:window.prefs,Health:{HullPercent:61,ShieldPercent:0,ShieldState:'OFFLINE',Offline:true}}));assert(await page.locator('#shield-offline-banner').isVisible(),'Shield warning missing');await page.emulateMedia({reducedMotion:'reduce'});
 assert(await page.evaluate(()=>getComputedStyle(document.querySelector('.shell'),'::after').animationName)==='none','Reduced motion warning still animated');
 assert(await page.evaluate(()=>getComputedStyle(document.querySelector('.panel'),'::after').zIndex)==='-1','Scan lines overlap text');
 assert(!external.length,'Unexpected network request');assert(!errors.length,'Browser error '+errors.join('\n'));assert((await page.evaluate(()=>window.flightCalls)).length===0,'Review sent flight controls');
 fs.writeFileSync(path.join(out,'..','results.json'),JSON.stringify({views:results.length,results,errors,external},null,2));console.log('PASS: saved scale/reset, explicit fire-group editor focus, Engineering save reachability, picker/Escape, reduced motion and no game/network calls.');
}finally{await browser.close()}})().catch(e=>{console.error(e);process.exitCode=1});
