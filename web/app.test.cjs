const {test}=require('node:test');
const assert=require('node:assert/strict');
const vm=require('node:vm');
const fs=require('node:fs');

// Minimal DOM fixture runs the actual app.js against JSON rows (cards are rendered
// client-side now). We record the HTML the ledger receives so order/content are testable.
function fixture(query='',rowCount=3,customRows,now=Date.now(),pms){
  const keys=['area','kind','q','provider','country','service','term','refund','sort'];
  const fields={};
  for(const k of keys) fields[k]={value:'',options:[{value:'',textContent:'全部'}],replacements:0,events:{},closest(){return {firstChild:{textContent:k}};},get selectedOptions(){return [{textContent:this.value}];},replaceChildren(fragment){this.options=[...fragment.children];this.value='';this.replacements++;},addEventListener(k,fn){this.events[k]=fn;},setAttribute(){}};
  // Build compact rows in the shape app.js expects.
  const base=[
    {p:'5SIM',k:'once',pn:'单次号码',c:'美国',s:'Google',t:'20 minutes',r:'conditional',u:'每次激活',cu:'USD',a:'0.20',pi:'fivesim',h:'https://5sim.net/',q:'google 谷歌 gmail',d:'x',rp:'x',rn:'x',ru:'x',ov:'2026',pv:'2026'},
    {p:'SMSPool',k:'rental',pn:'租赁',c:'英国',s:'Google',t:'1 month',r:'nonrefundable',u:'每次激活',cu:'USD',a:'0.10',pi:'smspool',h:'https://smspool.net/',q:'google 谷歌',d:'x',rp:'x',rn:'x',ru:'x',ov:'2026',pv:'2026'},
    {p:'Other',k:'free',pn:'免费',c:'美国',s:'Discord',t:'无固定期限',r:'na',u:'免费',cu:'',a:'',ki:'具体报价',pi:'other',h:'https://o.test/',q:'discord',d:'x',rp:'x',rn:'x',ru:'x',ov:'2026',pv:'2026'},
  ];
  const rows=customRows || Array.from({length:rowCount},(_,i)=>({...base[i%base.length],ok:i%3===2?'unknown':'starting'}));
  const events={},windowEvents={};let url=new URL('https://nav.example/'+query);const history=[];
  const form={hidden:true,elements:{namedItem:k=>fields[k]},addEventListener:(e,fn)=>events[e]=fn,reset(){keys.forEach(k=>fields[k].value='');if(events.reset)events.reset();}};
  let ledgerHTML='';
  let active;
  const makeNode=()=>({children:[],dataset:{},attrs:{},events:{},textContent:'',hidden:false,
    appendChild(n){this.children.push(n);if(this===ledger)ledgerHTML+=n._html||'';},
    replaceChildren(...nodes){this.children=nodes;if(this===ledger)ledgerHTML='';},
    querySelectorAll(selector){return this.children.flatMap(n=>[...(n.className==='obj-item'?[n]:[]),...(n.querySelectorAll?n.querySelectorAll(selector):[])]);},
    querySelector(selector){if(selector==='.obj-name')return this.nameNode||(this.nameNode={textContent:''});if(selector==='#clear-country')return clearCountry;return null;},
    addEventListener(name,fn){this.events[name]=fn;},setAttribute(k,v){this.attrs[k]=String(v);},
    fire(name,event={}){if(this.events[name])this.events[name](event);},focus(){active=this;},
    showModal(){this.open=true;},close(){this.open=false;this.fire('close');}});
  const ledger=makeNode();
  const countN=makeNode(),emptyN=makeNode(),selN=makeNode(),noticeN=makeNode(),moreN=makeNode(),clearCountry=makeNode();
  const objSearch=makeNode(),objList=makeNode(),objMore=makeNode(),modal=makeNode(),chanList=makeNode(),chanSearch=makeNode(),chanCount=makeNode(),chanTitle=makeNode(),qList=makeNode();
  const quickChips=[...fs.readFileSync(__dirname+'/index.gohtml','utf8').matchAll(/<button type="button" class="quick-chip" data-search="([^"]+)" aria-pressed="false">([^<]+)<\/button>/g)].map(([,search,label])=>{
    const button=makeNode();button.dataset.search=search;button.textContent=label;
    button.classList={toggle(name,on){button.active=on;}};return button;
  });
  objSearch.value='';chanSearch.value='';
  function elemFromHTML(html){ return {_html:html}; }
  const doc={
    get activeElement(){return active;},
    querySelector(k){
      if(k==='#filters')return form;
      if(k==='#count')return countN;
      if(k==='#empty')return emptyN;
      if(k==='#selected')return selN;
      if(k==='#notice')return noticeN;
      if(k==='#ledger')return ledger;
      if(k==='#ledger-more')return moreN;
      if(k==='#q-suggest')return qList;
      if(k==='#rows-data'){
        const pool=[''], idx=s=>{ if(!s)return 0; let i=pool.indexOf(s); if(i<0){pool.push(s);i=pool.length-1;} return i; };
        const products={};
        const out=rows.map(r=>{
          const pid=r.pid||r.pi;
          products[pid] ||= Object.fromEntries(['pn','d','rp','rn','ru','co','st','ar','rs','on','ps','m','pv'].map(k=>[k,r[k]]));
          const compact=Object.fromEntries(Object.entries(r).filter(([k])=>!['pn','d','rp','rn','ru','co','st','ar','rs','on','ps','m','pv'].includes(k)));
          return {...compact,pid,t:idx(r.t),n:idx(r.n),sc:idx(r.sc),h:idx(r.h),me:idx(r.me),os:idx(r.os),ov:idx(r.ov),ca:idx(r.ca)};
        });
        return {textContent:JSON.stringify({products,pool,rows:out,rate_date:'2026-11-06',channels:pms?{fivesim:{d:'最低充值 3 USD',pm:pms,v:'2026-11-08'}}:{fivesim:{d:'最低充值 3 USD',pm:['Visa','Mastercard'],v:'2026-11-08'}}})};
      }
      if(k==='#obj-data')return {textContent:JSON.stringify([...new Set(rows.map(r=>r.s))].map(Name=>({Name,Channels:1})))};
      return {'#obj-search':objSearch,'#obj-list':objList,'#obj-more':objMore,'#chan-modal':modal,'#chan-list':chanList,'#chan-search':chanSearch,'#chan-count':chanCount,'#chan-title':chanTitle}[k]||null;
    },
    querySelectorAll(sel){ if(sel==='.quick-chip')return quickChips; return []; },
    createDocumentFragment(){return makeNode();},
    createElement(tag){
      if(tag==='div'){ let html='',kids=[]; return { set innerHTML(v){html=v;kids=(v.match(/<article[\s\S]*?<\/article>/g)||[]).map(elemFromHTML);}, get innerHTML(){return html;}, get children(){return kids;}, get firstChild(){return kids[0]||null;} }; }
      return makeNode();
    },
  };
  const context={document:doc,URL,URLSearchParams,Date:class extends Date {static now(){return now;}},Option:function(text,value){this.textContent=text;this.value=value;},get location(){return url;},history:{pushState(a,b,next){url=new URL(next);history.push(url.href);}},window:{addEventListener:(e,fn)=>windowEvents[e]=fn},setTimeout:fn=>fn()};
  vm.runInNewContext(fs.readFileSync(__dirname+'/app.js','utf8'),context);
  return {fields,nodes:{'#filters':form,'#empty':emptyN,'#count':countN,'#notice':noticeN},ledger,moreN,objSearch,objList,objMore,modal,chanList,chanSearch,clearCountry,quickChips,qList,get active(){return active;},get html(){return ledgerHTML;},get matchedCount(){return parseInt(countN.textContent)||0;},history,
    openServices(){active=fields.service;fields.service.events.focus();},
    blurServices(){active=null;fields.service.events.blur();},
    change(k,v){active=fields[k];fields[k].value=v;events.input({target:{name:k}});},back(next){url=new URL('https://nav.example/'+next);windowEvents.popstate();},clear(){form.reset();}};
}

test('fixed quick links work through external app.js and track filters, history and reset',()=>{
  const targets=[['Google','Google'],['Telegram','Telegram'],['WeChat','WeChat'],['GPT','OpenAI'],['Claude','Claude']];
  const data=targets.map(([,name])=>quote({s:name,q:name.toLowerCase()}));
  const f=fixture('?service=Telegram&country=美国',0,data);
  assert.deepEqual(f.quickChips.map(b=>b.textContent),targets.map(([label])=>label));
  for(const [i,[,term]] of targets.entries()){
    f.qList.hidden=false;
    f.quickChips[i].fire('click');
    assert.equal(f.fields.q.value,term);assert.equal(f.fields.service.value,'');
    assert.equal(f.fields.country.value,'美国');assert.equal(f.matchedCount,1);
    assert.equal(f.objSearch.value,term);assert.equal(f.objList.children.length,1);
    assert.equal(f.qList.hidden,true);
    assert.equal(new URL(f.history.at(-1)).searchParams.get('q'),term);
    f.quickChips.forEach((b,j)=>{assert.equal(b.active,i===j);assert.equal(b.attrs['aria-pressed'],String(i===j));});
  }
  f.change('q','other');assert.ok(f.quickChips.every(b=>!b.active));
  f.back('?q=telegram');assert.equal(f.quickChips[1].attrs['aria-pressed'],'true');
  f.clear();assert.ok(f.quickChips.every(b=>b.attrs['aria-pressed']==='false'));assert.equal(f.matchedCount,5);
});

test('quick link clears a narrower object selection before applying its keyword',()=>{
  const f=fixture('?q=google&service=GooglePay',0,[quote(),quote({s:'GooglePay',q:'googlepay'})]);
  assert.equal(f.matchedCount,1);
  f.quickChips[0].fire('click');assert.equal(f.matchedCount,2);assert.equal(f.fields.service.value,'');
});

test('target options mount once on restore and remain text-safe and selectable',()=>{
  const name='<img src=x onerror=alert(1)> & "quoted"';
  const f=fixture('?service='+encodeURIComponent(name),0,[quote({s:name})]);
  assert.equal(f.fields.service.replacements,1);
  assert.equal(f.fields.service.value,name);
  assert.equal(f.fields.service.options[0].value,'');
  assert.equal(f.fields.service.options[1].textContent,name+'（1 个渠道）');
  f.back('?service='+encodeURIComponent(name));
  assert.equal(f.fields.service.replacements,2);
  assert.equal(f.matchedCount,1);
});

test('URL restore, aliases, group filters, empty results and history',()=>{
  const f=fixture('?q=gmail&area=paid');assert.equal(f.nodes['#filters'].hidden,false);assert.equal(f.matchedCount,1);
  f.change('q','');f.change('term','30 days');assert.equal(f.matchedCount,0);assert.equal(f.fields.term.value,'30 days');assert.equal(f.nodes['#empty'].hidden,false);
  f.change('term','1 month');assert.equal(f.matchedCount,1);assert.match(f.history.at(-1),/term=1\+month/);
  f.back('?area=free');assert.equal(f.matchedCount,1);
  f.clear();assert.equal(f.matchedCount,3);
});
test('changing country preserves valid services and clears only invalid selections',()=>{
  const f=fixture();f.change('service','Google');f.change('country','美国');assert.equal(f.fields.service.value,'Google');
  f.change('country','英国');assert.equal(f.fields.service.value,'Google');
  f.change('country','美国');f.change('service','Discord');f.change('country','英国');assert.equal(f.fields.service.value,'');assert.match(f.nodes['#notice'].textContent,/已清除/);
});
test('keywords filter pulled target dropdown; provider and country are separate filters',()=>{
  const f=fixture('?q=Google');assert.equal(f.matchedCount,2);
  f.change('q','谷歌');assert.equal(f.matchedCount,2);
  f.openServices();
  assert.deepEqual(f.fields.service.options.map(o=>o.value),['','Google']);assert.match(f.fields.service.options[1].textContent,/2 个渠道/);
  f.change('provider','fivesim');f.openServices();assert.equal(f.matchedCount,1);assert.match(f.fields.service.options[1].textContent,/1 个渠道/);
  f.change('provider','');f.change('country','美国');assert.equal(f.matchedCount,1);
  f.change('q','gmail');assert.equal(f.matchedCount,1);
  f.change('q','5SIM');assert.equal(f.matchedCount,0);assert.deepEqual(f.fields.service.options.map(o=>o.value),['']);
  f.change('q','Discord');f.openServices();assert.deepEqual(f.fields.service.options.map(o=>o.value),['','Discord']);assert.equal(f.matchedCount,1);
});
test('1000-record fixture filters without changing matches',()=>{
  const f=fixture('',1000);const start=performance.now();f.change('q','google');const elapsed=performance.now()-start;assert.equal(f.matchedCount,667);console.log(`1000-record filter (data model): ${elapsed.toFixed(3)} ms (not a browser rendering measurement)`);
});

test('23117-record keyword filtering preserves case, whitespace and AND semantics',()=>{
  const f=fixture('',23117),times=[];
  for(let i=0;i<7;i++){
    const start=performance.now();f.change('q',i%2?'google 谷歌':'  GoOgLe   谷歌  ');times.push(performance.now()-start);
    assert.equal(f.matchedCount,15412);
    assert.deepEqual(f.fields.service.options.map(o=>o.value),['']);
  }
  console.log(`23117-record keyword filter median: ${times.sort((a,b)=>a-b)[3].toFixed(3)} ms (data model, not browser rendering)`);
  f.change('q','google discord');assert.equal(f.matchedCount,0);
  f.change('q','   ');assert.equal(f.matchedCount,23117);
});

test('large target dropdown mounts on focus and releases options while preserving selection',()=>{
  const data=Array.from({length:2000},(_,i)=>quote({s:`Object${i}`,q:`object${i}`}));
  const f=fixture('',0,data);
  assert.equal(f.fields.service.options.length,1);
  f.openServices();assert.equal(f.fields.service.options.length,2001);
  f.change('service','Object1999');assert.equal(f.matchedCount,1);
  f.blurServices();assert.deepEqual(f.fields.service.options.map(o=>o.value),['','Object1999']);
  assert.equal(f.fields.service.value,'Object1999');
  f.change('q','object1');assert.equal(f.fields.service.value,'Object1999');
  f.openServices();assert.equal(f.fields.service.options.length,1112);
  f.change('q','object2');assert.equal(f.fields.service.value,'');
  assert.match(f.nodes['#notice'].textContent,/已清除/);
  f.back('?service=Object1999');assert.equal(f.fields.service.value,'Object1999');assert.equal(f.matchedCount,1);
  f.clear();assert.equal(f.matchedCount,2000);assert.equal(f.fields.service.options.length,1);
});
test('price sort orders matched cards ascending, unpriced last',()=>{
  const f=fixture('?q=Google',3,[
    {p:'5SIM',pi:'fivesim',k:'once',s:'Google',q:'google',c:'美国',t:'single activation',cu:'USD',u:'次',a:'0.20',ok:'starting'},
    {p:'SMSPool',pi:'smspool',k:'once',s:'Google',q:'google',c:'美国',t:'single activation',cu:'USD',u:'次',a:'0.10',ok:'starting'},
    {p:'No price',pi:'none',k:'once',s:'Google',q:'google',c:'美国',t:'single activation',cu:'USD',u:'次',ok:'login'}
  ]);
  f.change('sort','price_asc');
  assert.ok(f.html.indexOf('0.10')>=0 && f.html.indexOf('0.10')<f.html.indexOf('0.20'),'asc order');
  f.change('sort','price_desc');
  assert.ok(f.html.indexOf('0.20')>=0 && f.html.indexOf('0.20')<f.html.indexOf('0.10'),'desc order');
  f.change('sort','group');
  assert.ok(f.html.indexOf('5SIM')>=0 && f.html.indexOf('5SIM')<f.html.indexOf('SMSPool'),'group order');
});

const quote = changes => ({p:'Channel',pi:'channel',pid:'product',k:'once',ok:'starting',c:'美国',s:'Google',q:'google',t:'single activation',u:'activation',cu:'USD',a:'0.20',n:'mobile',sc:'one service',h:'https://example.test/',pn:'单次号码',d:'20 分钟',rp:'支持',rn:'不支持',ru:'不支持',pv:'2026-11-06',ov:'2026-11-06',ca:'2026-11-06T12:00:00+08:00',os:'https://example.test/prices',me:'官方公开 API',...changes});

test('invited free listings label the registration destination and retain official sources',()=>{
  const link='https://premium.smsonline.cloud/zh?ref=fixture';
  const row=quote({k:'free',iv:true,h:link,ps:['https://www.smsonline.cloud/']});
  const f=fixture('',0,[row]);
  assert.match(f.html,/前往邀请注册/);
  assert.ok(f.html.includes(link));
  assert.ok(f.html.includes('https://www.smsonline.cloud/'));
  assert.match(f.html,/noopener noreferrer sponsored/);
  assert.match(fixture('',0,[{...row,iv:false}]).html,/查看免费号码/);
});

test('recharge facts moved off cards into the header dropdown',()=>{
  // Cards no longer carry per-card deposit small print (it was too long); the
  // per-channel minimum/payment facts now render once in the #deposit-dropdown.
  const priced=fixture('',1);
  assert.ok(!priced.html.includes('c-deposit'));
  assert.ok(!/最低充值 3 USD/.test(priced.html));
});

test('cards no longer claim payment methods (moved to header dropdown)',()=>{
  // Payment methods used to be printed on each card; they now live in the
  // #deposit-dropdown only, so a card must not assert a payment method.
  const withPay=fixture('',0,[quote({pi:'fivesim'})]).html;
  assert.ok(!withPay.includes(' · 可用 '));
  assert.ok(!withPay.includes('支付方式以官网注册登录后的充值页为准'));
});

test('static quotes expire on later visits at the 24-hour and seven-day boundaries',()=>{
  const row=quote({w:false});
  const warning='资料已过维护周期';
  const checked=Date.parse(row.ca);
  assert.ok(!fixture('',0,[row],checked+86400000).html.includes(warning));
  assert.ok(fixture('',0,[row],checked+86400001).html.includes(warning));
  for(const key of ['ov','pv']){
    const manual=quote({ca:'',ov:'2026-11-07',pv:'2026-11-07',[key]:'2026-11-06'});
    const boundary=Date.parse('2026-11-13T00:00:00Z');
    assert.ok(!fixture('',0,[manual],boundary).html.includes(warning));
    assert.ok(fixture('',0,[manual],boundary+1).html.includes(warning));
  }
  assert.ok(fixture('',0,[quote({w:true})],checked).html.includes(warning));
  assert.ok(fixture('',0,[quote({lf:true,w:false})],checked+86400001).html.includes(warning));
});

test('reused sort orders preserve filtered results, ties and pagination across sort switches',()=>{
  const rows=Array.from({length:180},(_,i)=>quote({p:`Channel ${i%2}`,pi:`channel${i%2}`,s:i%3?'Google':'Discord',q:i%3?'google':'discord',c:i%4?'美国':'英国',a:i%7?`${i%9}.000000000000000001`:''}));
  const f=fixture('',0,rows);
  for(const sort of ['price_desc','group','price_asc','price_desc','group']){
    f.change('q','');f.change('country','');f.change('provider','');f.change('sort',sort);
    for(const [key,value] of [['q','google'],['country','美国'],['provider','channel1']]){
      f.change(key,value);
      const params=new URLSearchParams(Object.entries(f.fields).map(([k,v])=>[k,v.value]));
      const fresh=fixture(`?${params}`,0,rows);
      assert.equal(f.matchedCount,fresh.matchedCount);assert.equal(f.html,fresh.html);
      f.moreN.fire('click');fresh.moreN.fire('click');assert.equal(f.html,fresh.html);
    }
  }
});

test('pooled and product fields render with per-offer kinds, original prices and warnings',()=>{
  const f=fixture('',0,[quote({oa:'1000',oc:'sats',a:'0.75854',ora:'2000',ra:'1.51708',r:'conflicting',co:'<unsafe>'}),quote({a:'',ok:'login'}),quote({a:'',ok:'unknown'}),quote({lf:true,a:'',ok:'unknown',sm:'来源上次访问失败'})]);
  for(const text of ['20 分钟','官方公开 API','mobile','one service','https://example.test/prices','2026-11-06T12:00:00+08:00','原价 1000 sats','2026-11-06 汇率折算','续费原价 2000 sats','登录官网查看价格','价格待核实','退款：资料冲突，官网确认','来源上次访问失败','对象采集'])assert.ok(f.html.includes(text),text);
  assert.ok(f.html.includes('≈ '));assert.ok(f.html.includes('&lt;unsafe&gt;'));assert.ok(!f.html.includes('<unsafe>'));
});

test('decimal ascending and descending preserve all 18 digits and zero',()=>{
  const f=fixture('',0,[quote({p:'Smaller',a:'0.100000000000000001'}),quote({p:'Larger',a:'0.100000000000000002'}),quote({p:'Zero',a:'0'}),quote({p:'Unknown',a:'',ok:'unknown'})]);
  assert.ok(f.html.indexOf('Zero')<f.html.indexOf('Smaller'));assert.ok(f.html.indexOf('Smaller')<f.html.indexOf('Larger'));
  f.change('sort','price_desc');assert.ok(f.html.indexOf('Larger')<f.html.indexOf('Smaller'));assert.ok(f.html.indexOf('Smaller')<f.html.indexOf('Zero'));assert.ok(f.html.indexOf('Zero')<f.html.indexOf('Unknown'));
});

test('price_asc is a global price order; group mode still groups by channel/term',()=>{
  const f=fixture('',0,[quote({p:'B exact high',pi:'b',a:'0.9',ok:'exact'}),quote({p:'A starting low',pi:'a',a:'0.01'}),quote({p:'A exact high',pi:'a',a:'0.8',ok:'exact'}),quote({p:'B starting low',pi:'b',a:'0.02'}),quote({p:'Later term',pi:'b',a:'0.001',t:'z'})]);
  // price_asc: strictly ascending by amount regardless of offer kind or term.
  const order=['Later term','A starting low','B starting low','A exact high','B exact high'];
  for(let i=0;i<order.length-1;i++) assert.ok(f.html.indexOf(order[i])<f.html.indexOf(order[i+1]),`asc ${order[i]}<${order[i+1]}`);
  f.change('sort','price_desc');
  for(let i=order.length-1;i>0;i--) assert.ok(f.html.indexOf(order[i])<f.html.indexOf(order[i-1]),`desc ${order[i]}<${order[i-1]}`);
  f.change('sort','group');assert.ok(f.html.indexOf('A starting low')<f.html.indexOf('B exact high'));assert.ok(f.html.indexOf('A exact high')<f.html.indexOf('B exact high'));
});

test('load more appends once and filters reset the visible page',()=>{
  const f=fixture('',125);assert.equal(f.ledger.children.length,60);
  f.moreN.fire('click');assert.equal(f.ledger.children.length,120);
  f.moreN.fire('click');assert.equal(f.ledger.children.length,125);assert.equal(f.moreN.hidden,true);
  f.change('q','google');assert.equal(f.matchedCount,84);assert.equal(f.ledger.children.length,60);assert.equal(f.moreN.hidden,false);
  f.moreN.fire('click');assert.equal(f.ledger.children.length,84);
});

test('object buttons select, search and paginate; new pages retain selection',()=>{
  const data=Array.from({length:65},(_,i)=>quote({s:`Object${String(i).padStart(2,'0')}`,q:`object${i}`}));
  const f=fixture('',0,data);assert.equal(f.objList.children.length,60);
  f.objMore.fire('click');assert.equal(f.objList.children.length,65);
  const button=f.objList.children[64].children[0];assert.equal(button.type,'button');button.fire('click');assert.equal(f.fields.service.value,'Object64');assert.equal(f.matchedCount,1);assert.equal(button.attrs['aria-pressed'],'true');
  f.objSearch.value='Object64';f.objSearch.fire('input');assert.equal(f.objList.children.length,1);assert.equal(f.objList.children[0].children[0].attrs['aria-pressed'],'true');
});

test('channel preview filters, opens modally and restores trigger focus on close',()=>{
  const f=fixture('',0,[quote({a:'0.1'}),quote({c:'英国',a:'0.2'})]);
  let focused=false;const trigger={dataset:{provider:'channel',service:'Google'},focus(){focused=true;}};
  f.ledger.fire('click',{target:{closest:()=>trigger},preventDefault(){}});assert.equal(f.modal.open,true);assert.equal(f.active,f.chanSearch);assert.match(f.chanList.innerHTML,/美国/);assert.match(f.chanList.innerHTML,/英国/);
  f.chanSearch.value='英国';f.chanSearch.fire('input');assert.ok(!f.chanList.innerHTML.includes('美国'));
  f.modal.querySelectorAll=()=>[f.objMore,f.chanSearch,f.moreN];
  f.objMore.focus();let prevented=false;f.modal.fire('keydown',{key:'Tab',shiftKey:true,preventDefault(){prevented=true;}});assert.equal(f.active,f.moreN);assert.equal(prevented,true);
  f.modal.fire('keydown',{key:'Tab',shiftKey:false,preventDefault(){}});assert.equal(f.active,f.objMore);
  f.modal.fire('click',{target:{closest:()=>true}});assert.equal(f.modal.open,false);assert.equal(focused,true);
});

test('empty country hint describes site coverage and clears only country',()=>{
  const f=fixture('?q=google&country=英国',0,[quote({c:'未知'}),quote({c:'英国',s:'Discord',q:'discord'})]);
  assert.equal(f.matchedCount,0);assert.match(f.nodes['#empty'].innerHTML,/本站未收录/);assert.match(f.nodes['#empty'].innerHTML,/不代表该国可用/);
  f.clearCountry.fire('click');assert.equal(f.fields.country.value,'');assert.equal(f.fields.q.value,'google');assert.equal(f.matchedCount,1);
});



