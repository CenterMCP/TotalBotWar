import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import http from 'node:http';
import net from 'node:net';
import {spawn,spawnSync} from 'node:child_process';
import {mkdtemp,rm,mkdir} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('..',import.meta.url));
const temp=await mkdtemp(join(tmpdir(),'totalbotwar-test-'));
let child,browser,host;
try {
 const binary=join(temp,'TotalBotWar');
 const built=spawnSync('go',['build','-o',binary,'./cmd/server'],{cwd:root,encoding:'utf8'});assert.equal(built.status,0,built.stderr);
 const listener=net.createServer();await new Promise(resolve=>listener.listen(0,'127.0.0.1',resolve));const port=listener.address().port;await new Promise(resolve=>listener.close(resolve));
 child=spawn(binary,['--address',`127.0.0.1:${port}`],{stdio:'ignore',env:{...process.env,DATA_DIR:join(temp,'data')}});
 for(let attempt=0;;attempt++){try{if((await fetch(`http://127.0.0.1:${port}/health`)).ok)break}catch{};if(attempt>100)throw Error('MCP server failed to start');await new Promise(r=>setTimeout(r,50))}
 let session,sequence=0;
 async function rpc(method,params){const headers={'content-type':'application/json',accept:'application/json, text/event-stream'};if(session)headers['Mcp-Session-Id']=session;const response=await fetch(`http://127.0.0.1:${port}/mcp`,{method:'POST',headers,body:JSON.stringify({jsonrpc:'2.0',id:++sequence,method,params})});session=response.headers.get('mcp-session-id')??session;let raw=await response.text();if(response.headers.get('content-type')?.includes('text/event-stream'))raw=raw.split('\n').find(l=>l.startsWith('data:')).slice(5);const data=JSON.parse(raw);if(!response.ok||data.error)throw Error(JSON.stringify(data));return data.result}
 await rpc('initialize',{protocolVersion:'2025-11-25',capabilities:{},clientInfo:{name:'TotalBotWar-test-host',version:'1'}});
 const toolList=await rpc('tools/list',{});assert.equal(toolList.tools.length,5);const open=toolList.tools.find(t=>t.name==='open_battlefield');assert.deepEqual(open._meta['openai/ui'].entrypoints,[{type:'global'},{type:'thread'}]);assert.ok(!toolList.tools.find(t=>t.name==='end_turn')._meta.ui.resourceUri);
 const resource=await rpc('resources/read',{uri:open._meta.ui.resourceUri});assert.deepEqual(resource.contents[0]._meta['openai/ui'].availableDisplayModes,['fullscreen']);
 const calls=[],messages=[],contexts=[],initializations=[];let failNext=false;
 const shell=`<!doctype html><html style="height:100%;background:#101713"><head><title>TotalBotWar · local MCP host</title></head><body style="margin:0;height:100%"><iframe id="app" src="/widget" style="width:100%;height:100%;border:0" title="TotalBotWar"></iframe><script>
 const frame=document.getElementById('app');window.addEventListener('message',async e=>{if(e.source!==frame.contentWindow)return;const m=e.data;if(m.method==='ui/notifications/initialized'){frame.contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/tool-result',params:{content:[],structuredContent:{lobby:true}}},'*');return}if(m.id===undefined)return;try{let result;if(m.method==='ui/initialize'){await fetch('/init',{method:'POST',body:JSON.stringify(m.params)});result={protocolVersion:m.params.protocolVersion,hostInfo:{name:'local-test-host',version:'1'},hostCapabilities:{serverTools:{},updateModelContext:{text:{}},message:{text:{}},experimental:{'openai/message':{},'openai/model-context':{}}},hostContext:{theme:'dark',platform:'web',displayMode:'fullscreen',availableDisplayModes:['fullscreen']}}}
 else if(m.method==='tools/call')result=await fetch('/call',{method:'POST',body:JSON.stringify(m.params)}).then(r=>r.json());
 else if(m.method==='ui/message')result=await fetch('/message',{method:'POST',body:JSON.stringify(m.params)}).then(r=>r.json());
 else if(m.method==='ui/update-model-context')result=await fetch('/context',{method:'POST',body:JSON.stringify(m.params)}).then(r=>r.json());
 else if(m.method==='ping')result={};else throw Error('Unexpected bridge request '+m.method);
 frame.contentWindow.postMessage({jsonrpc:'2.0',id:m.id,result},'*')}catch(error){frame.contentWindow.postMessage({jsonrpc:'2.0',id:m.id,error:{code:-32603,message:error.message}},'*')}});</script></body></html>`;
 host=http.createServer(async(req,res)=>{try{if(req.url==='/widget'){res.setHeader('content-type','text/html');res.end(resource.contents[0].text);return}if(req.method==='POST'){let raw='';for await(const chunk of req)raw+=chunk;const data=JSON.parse(raw);let result={};if(req.url==='/call'){calls.push(data);result=failNext?{isError:true,content:[{type:'text',text:'Simulated connection failure. Retry your command.'}]}:await rpc('tools/call',data);failNext=false}else if(req.url==='/message')messages.push(data);else if(req.url==='/context')contexts.push(data);else if(req.url==='/init')initializations.push(data);res.setHeader('content-type','application/json');res.end(JSON.stringify(result));return}res.setHeader('content-type','text/html');res.end(shell)}catch(e){res.statusCode=500;res.end(JSON.stringify({error:e.message}))}});
 await new Promise(resolve=>host.listen(0,'127.0.0.1',resolve));const url=`http://127.0.0.1:${host.address().port}`;
 if(process.argv.includes('--preview')){console.log(`TotalBotWar local MCP host: ${url}\nThis host emulates ChatGPT for development. Chat messages are captured, not answered.`);await new Promise(resolve=>{process.on('SIGINT',resolve);process.on('SIGTERM',resolve)});}
 else {
 browser=await chromium.launch({headless:true,...(process.env.BROWSER_EXECUTABLE?{executablePath:process.env.BROWSER_EXECUTABLE}:{})});const page=await browser.newPage({viewport:{width:1560,height:1060}});const errors=[];page.on('pageerror',e=>errors.push(e.message));await page.goto(url);const frame=page.frameLocator('#app');
 await frame.locator('#start:enabled').waitFor();assert.deepEqual(initializations[0].appCapabilities.availableDisplayModes,['fullscreen']);await frame.locator('#start').click();await frame.locator('#game:not([hidden])').waitFor();assert.equal(await frame.locator('.tile').count(),216);assert.equal(await frame.locator('.unit-card').count(),5);assert.ok((await frame.locator('.hp-track i').first().boundingBox()).height<=4,'health bar must remain compact');assert.equal(await frame.locator('.token.enemy').count(),0);assert.equal(await frame.locator('.tile.fog').count()>0,true);
 const created=calls.find(c=>c.name==='create_battle');assert.ok(created);const state=contexts.at(-1)?.content[0].text;assert.ok(state?.includes('TotalBotWar battle'));
 const id=(await frame.locator('#battle-id').textContent()).replace('Battle ID: ','');
 // Move from C3 to D3, queue a second order and verify that the turn has not advanced.
 await frame.locator('.tile[data-x="3"][data-y="2"]').click();await frame.locator('#order-count').getByText('1 orders queued',{exact:false}).waitFor();assert.equal(await frame.locator('#order-lines line').count(),1);
 await frame.getByRole('button',{name:/Grey Watch, scouts/}).click();await frame.locator('#scout').click();await frame.locator('#order-count').getByText('2 orders queued',{exact:false}).waitFor();assert.equal((await rpc('tools/call',{name:'get_battle',arguments:{battle_id:id}})).structuredContent.turn,1);
 await frame.locator('#end-turn').click();await frame.locator('#turn').getByText('02',{exact:true}).waitFor();await frame.locator('#order-count').getByText('0 orders queued',{exact:false}).waitFor();
 const current=(await rpc('tools/call',{name:'get_battle',arguments:{battle_id:id}})).structuredContent;assert.equal(current.units.find(u=>u.id==='p1').x,3);
 // External (chat-side) tool call is reflected by polling, without remounting the iframe.
 const ordered=await rpc('tools/call',{name:'issue_orders',arguments:{battle_id:id,revision:current.revision,orders:[{unit_id:'p1',action:'defend'}]}});assert.ok(!ordered.isError);await frame.locator('#order-count').getByText('1 orders queued',{exact:false}).waitFor();
 // Stale requests cannot advance the game twice.
 const stale=await rpc('tools/call',{name:'end_turn',arguments:{battle_id:id,revision:1}});assert.equal(stale.isError,true);
 const invalid=await rpc('tools/call',{name:'issue_orders',arguments:{battle_id:id,revision:ordered.structuredContent.revision,orders:[{unit_id:'p1',action:'move',x:4}]}});assert.equal(invalid.isError,true);
 const badNumber=await rpc('tools/call',{name:'issue_orders',arguments:{battle_id:id,revision:ordered.structuredContent.revision,orders:[{unit_id:'p1',action:'move',x:4.5,y:2}]}});assert.equal(badNumber.isError,true);
 failNext=true;await frame.locator('#defend').click();await frame.locator('#status[data-error=true]').getByText(/Simulated connection failure/).waitFor();await frame.locator('#defend').click();await frame.locator('#status[data-error=false]').getByText(/Order queued/).waitFor();
 await frame.locator('#command').fill('Hold the line and scout ahead.');await frame.locator('#send').click();await frame.locator('#status').getByText(/Command sent to ChatGPT/).waitFor();assert.equal(messages.length,1);assert.ok(messages[0].content[0].text.includes(id));assert.equal(messages[0]._meta['openai/message'].target,'active');
 await frame.locator('#help-button').click();await frame.locator('#manual[open]').waitFor();await frame.locator('#close-help').click();
 if(process.env.SCREENSHOT_PATH){await mkdir(join(root,'build'),{recursive:true});await page.screenshot({path:process.env.SCREENSHOT_PATH,fullPage:true})}
 await page.setViewportSize({width:390,height:844});assert.equal(await frame.locator('.tile').count(),216);const overflow=await page.evaluate(()=>{const d=document.getElementById('app').contentDocument;return d.documentElement.scrollWidth>d.documentElement.clientWidth});assert.equal(overflow,false,'mobile page overflows');
 await frame.locator('#new-button').click();await frame.locator('#lobby:not([hidden])').waitFor();await frame.locator('#resume-id').fill(id);await frame.locator('#resume').click();await frame.locator('#game:not([hidden])').waitFor();assert.equal(await frame.locator('#turn').textContent(),'02');
 assert.deepEqual(errors,[]);console.log('PASS: sidebar metadata, fullscreen handshake, real MCP HTTP, fog, movement orders, turn resolution, chat-side sync, stale revisions, validation, retry, chat messages, resume, and responsive layout.');
 }
} finally {if(browser)await browser.close();if(host)await new Promise(r=>host.close(r));if(child&&child.exitCode===null){const exited=new Promise(r=>child.once('exit',r));child.kill('SIGTERM');await exited}await rm(temp,{recursive:true,force:true})}
