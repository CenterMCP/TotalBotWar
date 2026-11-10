import { App } from '@modelcontextprotocol/ext-apps';
import { OpenAIExtensions } from '@openai/mcp-extensions/app';

type Point = {x:number;y:number};
type Unit = Point & {id:string;name:string;side:string;kind:string;hp:number;morale:number;guard:boolean;scouting:boolean};
type Order = {unit_id:string;action:string;x?:number;y?:number;target_id?:string};
type Cell = Point & {terrain:string;visible:boolean;explored:boolean};
type Battle = {battle_id:string;revision:number;turn:number;status:string;width:number;height:number;cells:Cell[];units:Unit[];orders:Record<string,Order>;reports:string[];scores:Record<string,number>;objective:Point;winning_score:number;max_turns:number};
const $ = <T extends HTMLElement = HTMLElement>(id:string) => document.getElementById(id) as T;
const app = new App({name:'TotalBotWar',version:'0.1.0'}, {availableDisplayModes:['fullscreen']}, {autoResize:false});
const extensions = new OpenAIExtensions(app);
let battle: Battle | undefined;
let selected: string | undefined;
let busy = false, connected = false, refreshing = false, disposed = false, initialResult = false;
let poll: ReturnType<typeof setTimeout> | undefined;
let generation=0;
let lastContext='';
const glyph: Record<string,string> = {infantry:'⚔',spears:'♜',archers:'➶',cavalry:'♞',scouts:'⌕'};
const movement = (u:Unit) => u.kind==='scouts'?4:u.kind==='cavalry'?3:2;
const coord = (p:Point) => `${String.fromCharCode(65+p.x)}${p.y+1}`;
const distance = (a:Point,b:Point) => Math.abs(a.x-b.x)+Math.abs(a.y-b.y);
const own = () => battle?.units.filter(u=>u.side==='player') ?? [];
function error(e:unknown) { $('status').textContent=e instanceof Error?e.message:String(e); $('status').dataset.error='true'; }
function notice(text:string) { $('status').textContent=text; $('status').dataset.error='false'; }
function remember(id:string) {try{localStorage.setItem('totalbotwar.battle',id)}catch{} }
function remembered():string|undefined {try{return localStorage.getItem('totalbotwar.battle')??undefined}catch{return undefined} }
function isBattle(v:unknown):v is Battle {if(!v||typeof v!=='object')return false;const b=v as Battle;return typeof b.battle_id==='string'&&Number.isInteger(b.revision)&&b.width===18&&b.height===12&&Array.isArray(b.cells)&&b.cells.length===216&&Array.isArray(b.units)&&Array.isArray(b.reports)&&typeof b.orders==='object'&&b.orders!==null&&typeof b.scores==='object'&&b.scores!==null;}
function accept(value:unknown, switchBattle=false) {
 if (!isBattle(value)) return;
 if (battle && battle.battle_id!==value.battle_id && !switchBattle) return;
 if (battle?.battle_id===value.battle_id && battle.revision>value.revision) return;
 const changed=!battle||battle.battle_id!==value.battle_id||battle.revision!==value.revision;
 battle=value; remember(value.battle_id);
 if(!selected||!own().some(u=>u.id===selected))selected=own()[0]?.id;
 if(changed) {render(); void context();}
}
async function context() {
 if(!connected||!battle||!app.getHostCapabilities()?.updateModelContext)return;
 const text=`TotalBotWar battle ${battle.battle_id}, turn ${battle.turn}, revision ${battle.revision}. Selected unit: ${selected??'none'}. The player commands the Dawn Legion. Use get_battle with this battle_id before planning; use only current visible intel. Queue with issue_orders, resolve with end_turn when requested. Current orders: ${JSON.stringify(battle.orders)}.`;
 if(text===lastContext)return;lastContext=text;
 try{await app.updateModelContext({content:[{type:'text',text}]})}catch{lastContext=''}
}
async function invoke(name:string,args:Record<string,unknown>) {
 const result=await app.callServerTool({name,arguments:args},{timeout:20_000});
 if(result.isError)throw new Error(result.content.filter(c=>c.type==='text').map(c=>c.text).join('\n')||'Command failed. Refresh and try again.');
 return result.structuredContent;
}
function controls() {
 const planning=connected&&!!battle&&battle.status==='planning'&&!busy;
 for(const id of ['start','resume','new-button'])$(id).toggleAttribute('disabled',!connected||busy);
 for(const id of ['defend','scout','cancel'])$(id).toggleAttribute('disabled',!planning||!selected);
 $('end-turn').toggleAttribute('disabled',!planning);
 $('send').toggleAttribute('disabled',!connected||busy||!extensions.message);
 $('end-turn').textContent=busy?'Sending orders…':`Resolve turn ${battle?.turn??''} →`;
 $('game').setAttribute('aria-busy',String(busy));
}
async function action(work:()=>Promise<void>) {
 if(busy||!connected)return;busy=true;controls();
 try {await work()}catch(e){error(e)}finally{busy=false;controls()}
}
async function refresh() {
 if(!connected||!battle||busy||refreshing||disposed)return;
 refreshing=true;const id=battle.battle_id, gen=generation;
 try {const value=await invoke('get_battle',{battle_id:id});if(gen===generation)accept(value)}catch(e){error(e)}finally{refreshing=false}
}
function schedule() {poll=setTimeout(async()=>{if(!document.hidden)await refresh();if(!disposed)schedule()},2500)}
function reachable(unit:Unit):Set<string> {
 if(!battle)return new Set();const out=new Set<string>(), visited=new Set([`${unit.x},${unit.y}`]);const queue=[{x:unit.x,y:unit.y,d:0}];
 while(queue.length){const p=queue.shift()!;if(p.d>=movement(unit))continue;for(const [dx,dy]of [[1,0],[-1,0],[0,1],[0,-1]]){const x=p.x+dx,y=p.y+dy,key=`${x},${y}`;if(x<0||x>=18||y<0||y>=12||visited.has(key)||battle.cells[y*18+x].terrain==='water')continue;visited.add(key);out.add(key);queue.push({x,y,d:p.d+1})}}return out;
}
function orderText(order:Order|undefined) {if(!order)return 'Awaiting orders';if(order.action==='move')return `March to ${coord({x:order.x!,y:order.y!})}`;if(order.action==='attack')return `Attack ${battle?.units.find(u=>u.id===order.target_id)?.name??order.target_id}`;return order.action==='scout'?'Scout the horizon':'Hold & defend';}
function terrainIcon(terrain:string) {
 const shapes:Record<string,string>={forest:'<path d="M10 3L3 16H7L2 25H18L13 16H17ZM24 7L18 19H21L17 28H31L27 19H30Z" fill="currentColor"/>',hill:'<path d="M1 25L12 7L23 25M14 25L24 13L32 25M8 16L12 20L16 16" stroke="currentColor" stroke-width="2" fill="none"/>',water:'<path d="M0 10Q8 2 16 10T32 10M0 22Q8 14 16 22T32 22" stroke="currentColor" stroke-width="2" fill="none"/>',tower:'<path d="M8 28V9H5V3H10V7H14V3H19V7H23V3H28V9H25V28ZM14 28V20H19V28" stroke="currentColor" stroke-width="2" fill="none"/>'};
 return shapes[terrain]?`<svg class="landmark" viewBox="0 0 34 34" aria-hidden="true">${shapes[terrain]}</svg>`:'';
}
function render() {
 if(!battle)return;
 $('lobby').hidden=true;$('game').hidden=false;$('new-button').hidden=false;
 $('turn').textContent=String(battle.turn).padStart(2,'0');$('player-score').textContent=String(battle.scores.player);$('enemy-score').textContent=String(battle.scores.enemy);
 $('progress').style.width=`${Math.min(100,battle.scores.player/6*100)}%`;$('progress-label').textContent=`${battle.scores.player} / 6 control points`;
 $('battle-id').textContent=`Battle ID: ${battle.battle_id}`;
 const enemies=battle.units.filter(u=>u.side==='enemy');$('visibility-label').textContent=`${enemies.length} enemy regiment${enemies.length===1?'':'s'} in sight`;
 $('unit-count').textContent=String(own().length).padStart(2,'0');
 const roster=$('roster');roster.replaceChildren();
 for(const u of own()) {
  const button=document.createElement('button');button.className='unit-card'+(selected===u.id?' active':'');button.setAttribute('aria-pressed',String(selected===u.id));button.setAttribute('aria-label',`${u.name}, ${u.kind}, ${u.hp} health, ${coord(u)}`);
  // Unit names come from this game's fixed roster; dynamic text is assigned with textContent below.
  button.innerHTML=`<span class="unit-glyph" aria-hidden="true">${glyph[u.kind]??'⚔'}</span><span><span class="unit-name"></span><span class="unit-meta"><span></span><span></span></span><span class="hp-track"><i></i></span><span class="queued"></span></span>`;
  button.querySelector('.unit-name')!.textContent=u.name;
  const meta=button.querySelectorAll('.unit-meta span');meta[0].textContent=`${u.kind} · ${coord(u)}`;meta[1].textContent=`${u.hp}%`;
  (button.querySelector('.hp-track i')as HTMLElement).style.width=`${Math.max(0,Math.min(100,u.hp))}%`;
  button.querySelector('.queued')!.textContent=orderText(battle.orders[u.id]);button.onclick=()=>select(u.id);roster.append(button);
 }
 renderMap();renderSelection();
 $('order-count').textContent=`${Object.keys(battle.orders).length} orders queued · remaining units defend`;
 const reports=$('reports');reports.replaceChildren();for(const text of battle.reports){const p=document.createElement('p');p.textContent=text;reports.append(p)}
 $('report-turn').textContent=battle.turn===1?'DAWN':`TURN ${Math.min(battle.turn-Number(battle.status==='planning'),24)}`;
 $('outcome').hidden=battle.status==='planning';$('outcome').textContent=({victory:'Victory. Ashen Vale is yours.',defeat:'Defeat. The Dawn Legion has fallen.',draw:'A hard-fought draw. Neither army prevailed.'}as Record<string,string>)[battle.status]??'';
 controls();
}
function renderSelection(){const u=own().find(u=>u.id===selected);$('selected-name').textContent=u?`${u.name} · ${u.id.toUpperCase()}`:'Choose a regiment';$('selected-detail').textContent=u?`${u.hp} health · ${u.morale} morale · move ${movement(u)} · range ${u.kind==='archers'?3:1} · ${orderText(battle?.orders[u.id])}`:'Select your forces on the map or from the roster.';controls()}
function renderMap() {
 if(!battle)return;const unit=own().find(u=>u.id===selected);const range=unit&&battle.status==='planning'?reachable(unit):new Set();
 const occupied=new Map(battle.units.map(u=>[`${u.x},${u.y}`,u]));const map=$('map');map.replaceChildren();
 for(const cell of battle.cells) {
  const key=`${cell.x},${cell.y}`,u=occupied.get(key);const tile=document.createElement('button');tile.className=`tile ${cell.terrain}${!cell.visible?' fog':''}${!cell.explored?' unexplored':''}${range.has(key)?' reachable':''}`;tile.dataset.x=String(cell.x);tile.dataset.y=String(cell.y);
  const label=`${coord(cell)}, ${cell.terrain}${!cell.visible?', outside vision':''}${u?`, ${u.side==='player'?'friendly':'enemy'} ${u.name}, ${u.hp} health`:''}`;tile.setAttribute('aria-label',label);tile.title=label;tile.innerHTML=terrainIcon(cell.terrain);
  if(u){const token=document.createElement('span');token.className=`token ${u.side}${u.id===selected?' selected':''}`;token.textContent=glyph[u.kind]??'⚔';const track=document.createElement('i'),bar=document.createElement('b');bar.style.width=`${Math.max(0,Math.min(100,u.hp))}%`;track.append(bar);token.append(track);if(battle.orders[u.id]){const dot=document.createElement('span');dot.className='order-dot';dot.textContent=battle.orders[u.id].action==='move'?'↗':battle.orders[u.id].action==='attack'?'⚔':'•';token.append(dot)};tile.append(token)}
  if(Object.values(battle.orders).some(o=>o.action==='move'&&o.x===cell.x&&o.y===cell.y)){const flag=document.createElement('span');flag.className='target-flag';tile.append(flag)}
  tile.onclick=()=>{if(u?.side==='player'){select(u.id);return};if(!unit||busy||battle?.status!=='planning')return;void queue(u?{unit_id:unit.id,action:'attack',target_id:u.id}:{unit_id:unit.id,action:'move',x:cell.x,y:cell.y})};map.append(tile);
 }
 const lines=$('order-lines');lines.replaceChildren();
 for(const o of Object.values(battle.orders)) {const u=own().find(u=>u.id===o.unit_id);if(!u||o.action!=='move')continue;const line=document.createElementNS('http://www.w3.org/2000/svg','line');for(const [k,v]of Object.entries({x1:u.x*100+50,y1:u.y*100+50,x2:o.x!*100+50,y2:o.y!*100+50}))line.setAttribute(k,String(v));line.setAttribute('stroke','#f3d79a');line.setAttribute('stroke-width','3');line.setAttribute('stroke-dasharray','9 7');line.setAttribute('marker-end','url(#arrowhead)');lines.append(line)}
}
function select(id:string){selected=id;render();void context()}
async function queue(order:Order){await action(async()=>{if(!battle)return;accept(await invoke('issue_orders',{battle_id:battle.battle_id,revision:battle.revision,orders:[order]}));notice('Order queued. Resolve the turn when your army is ready.')})}
async function start(){await action(async()=>{generation++;const value=await invoke('create_battle',{});if(!isBattle(value))throw new Error('Invalid battle response.');accept(value,true);notice('Your command begins. Select a regiment or give orders in ChatGPT.')})}
$('start').onclick=()=>void start();
$('new-button').onclick=()=>{if(busy)return;generation++;if(battle)$<HTMLInputElement>('resume-id').value=battle.battle_id;battle=undefined;selected=undefined;$('game').hidden=true;$('lobby').hidden=false;$('new-button').hidden=true;$('battle-id').textContent='';notice('Your previous battle is saved. Start another or resume with its battle ID.')};
$('resume-form').onsubmit=e=>{e.preventDefault();void action(async()=>{const id=$<HTMLInputElement>('resume-id').value.trim();if(!/^[a-f0-9]{48}$/.test(id))throw new Error('Enter the 48-character battle ID from your saved game.');generation++;const value=await invoke('get_battle',{battle_id:id});accept(value,true);notice('Battle resumed.')})};
for(const actionName of ['defend','scout','cancel'])$(actionName).onclick=()=>{if(selected)void queue({unit_id:selected,action:actionName})};
$('end-turn').onclick=()=>void action(async()=>{if(!battle)return;accept(await invoke('end_turn',{battle_id:battle.battle_id,revision:battle.revision}));notice(battle.status==='planning'?'Turn resolved. Review the dispatches and plan your next move.':`Battle finished: ${battle.status}.`)});
$('chat-form').onsubmit=e=>{e.preventDefault();void action(async()=>{const input=$<HTMLInputElement>('command'),command=input.value.trim();if(!command||!battle||!extensions.message)return;const active=!!app.getHostCapabilities()?.updateModelContext;const sent=await extensions.message.send({role:'user',content:[{type:'text',text:`In TotalBotWar battle ${battle.battle_id}: ${command}\nRead get_battle first for current revision and visible intel. Queue orders with issue_orders. Only end_turn if I requested it.`}],_meta:{'openai/message':{target:active?'active':'new',send:true}}});if(sent.isError)throw new Error('Chat could not accept the command. Use the ChatGPT composer.');input.value='';notice('Command sent to ChatGPT. The battlefield updates as orders arrive.')})};
$('help-button').onclick=()=>$<HTMLDialogElement>('manual').showModal();$('close-help').onclick=()=>$<HTMLDialogElement>('manual').close();
$('columns').replaceChildren(...Array.from({length:18},(_,i)=>{const s=document.createElement('span');s.textContent=String.fromCharCode(65+i);return s}));$('rows').replaceChildren(...Array.from({length:12},(_,i)=>{const s=document.createElement('span');s.textContent=String(i+1);return s}));
app.ontoolinput=({arguments:args})=>{if(args&&typeof args.battle_id==='string')$<HTMLInputElement>('resume-id').value=args.battle_id};
app.ontoolresult=result=>{initialResult=true;if(result.isError){error(result.content.filter(c=>c.type==='text').map(c=>c.text).join('\n'));return};if(isBattle(result.structuredContent)){generation++;accept(result.structuredContent,true)}};
app.onteardown=async()=>{disposed=true;connected=false;clearTimeout(poll);return {}};
document.addEventListener('visibilitychange',()=>{if(!document.hidden)void refresh()});
async function connect(){
 if(parent===window){$('connection').textContent='MCP host required';notice('Connect this MCP server in ChatGPT to play. This page runs inside the sidebar app.');return}
 try{await app.connect();connected=true;$('connection').textContent='Command link established';controls();
 // Declare supported display modes through the extension when supported by this host.
 if(app.getHostContext()?.displayMode!=='fullscreen'&&app.getHostContext()?.availableDisplayModes?.includes('fullscreen'))await app.requestDisplayMode({mode:'fullscreen'});
 setTimeout(()=>{if(initialResult||battle)return;const id=remembered();if(id)$<HTMLInputElement>('resume-id').value=id;notice(id?'Resume your saved battle, or begin a new campaign.':'Begin a campaign to take command.');},350);
 schedule();void context();
 }catch(e){error(e)}
}
void connect();
