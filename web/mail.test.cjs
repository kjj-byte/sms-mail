const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function fixture({local = true, attachDomains = ['catchmail.io','maildrop.cc'], saved = '', mobile = false} = {}) {
  function node() {
    return {hidden:false, disabled:false, checked:true, value:'', text:'', children:[], attrs:{}, events:{},dataset:{},
      get textContent(){return this.text+this.children.map(n=>n.textContent).join('');},
      set textContent(value){this.text=value;this.children=[];},
      setAttribute(k,v){this.attrs[k]=v;},
      scrollIntoView(){this.scrolled=true;},
      addEventListener(k,v){this.events[k]=v;},
      appendChild(n){this.children.push(n);}, append(...nodes){this.children.push(...nodes);},
      replaceChildren(...nodes){this.children=nodes;this.text='';}, add(n){this.children.push(n);},
      fire(k){return this.events[k]?.({preventDefault(){}});}
    };
  }
  const ids = ['sms-view','sms-tab',...new Set([...fs.readFileSync(__dirname+'/index.gohtml','utf8').matchAll(/id="(mail-[^"]+)"/g)].map(m=>m[1]))];
  const nodes = Object.fromEntries(ids.map(id => [id,node()]));
  const fields = Object.fromEntries(['for','provider','domain','address','password','imapHost'].map(id=>[id,node()]));
  fields.for.value='example.com';
  nodes['mail-create'].elements={namedItem:k=>fields[k]};
  nodes['mail-view'].hidden=true; nodes['mail-controls'].disabled=true;
  nodes['mail-attach-hint'].hidden=true;
  nodes['mail-group'].value='state';
  nodes['mail-filter'].value='all';
  const calls=[], timers=new Map(), windowEvents={}, documentEvents={}, storage=new Map([['identity.mail.selected',saved]]);
  let nextTimer=0, failMessages=false, detailDelay=null, pollDelay=null, codeError='', confirm=true;
  const expires=new Date(Date.now()+86400000).toISOString();
  const base={target_service:'example.com',created_at:'2026-11-08T00:00:00Z',expires_at:expires,status:'active'};
  let inboxes=[
    {...base,id:'seed',provider:'mailtm',address:'seed@example.com',state:'active'},
    {...base,id:'old',provider:'catchmail',address:'old@catchmail.io',state:'expired'},
    {...base,id:'gone',provider:'ghost',address:'gone@maildrop.cc',status:'closed',state:'closed'}
  ];
  const location={origin:local?'http://127.0.0.1:8787':'https://public.example',pathname:'/preview/',hash:''};
  const malicious='<img src=x onerror=alert(1)>';
  const fetch = async (url, options) => {
    calls.push({url,...options});
    let data={}; let status=200;
    const path=url.split('?')[0];
    if(path==='/mail-api/status') data={csrf:'csrf-test',attachDomains};
    else if(path==='/mail-api/providers') data={providers:[{name:'mailtm',displayName:'Mail.tm',tier:'free',retention:'~days',features:{customUsername:true,pollInbox:true,attachments:false},enabled:true},{name:'catchmail',displayName:'Catchmail.io',tier:'free',retention:'~days',features:{pollInbox:true},enabled:true},{name:'disabled',displayName:'Off',enabled:false,features:{pollInbox:true}}]};
    else if(path==='/mail-api/inboxes') data={inboxes,page:1,pageSize:100,total:inboxes.length};
    else if(path==='/mail-api/inbox' && options.method==='POST') {
      const body=JSON.parse(options.body);
      const address=body.address||'new@example.com', provider=body.address?'maildrop':'mailtm';
      data={id:'abc',address,provider,expiresAt:expires,features:{pollInbox:true}};
      inboxes=[{...base,id:'abc',provider,address,target_service:body.for,state:'active'}];
    }
    else if(/\/messages\/m[12]$/.test(path)) {
      if(detailDelay) await detailDelay.promise;
      data={id:path.split('/').pop(),from:'test@example.com',subject:malicious,text:path.endsWith('m2')?'second body':malicious,html:malicious,receivedAt:'2026-11-08T00:00:00Z'};
    }
    else if(path.endsWith('/messages')) {
      if(pollDelay) await pollDelay.promise;
      if(failMessages){status=502;data={error:'上游不可用'};}
      else data={status:'active',address:'new@example.com',provider:'mailtm',pollAfter:10,
        verification:{codes:[{type:'numeric',value:'123456',confidence:0.9}],email:{from:'test@example.com',subject:malicious},messageId:'m1',receivedAt:'2026-11-08T00:00:00Z',error:codeError},
        messages:[{id:'m1',from:'test@example.com',subject:malicious,excerpt:malicious,receivedAt:'2026-11-08T00:00:00Z'},{id:'m2',subject:'second'}]};
    } else if(path.endsWith('/code')) data={codes:[{type:'numeric',value:'123456',confidence:0.9,context:'...'}],email:{from:'test@example.com',subject:malicious},messageId:'m1',receivedAt:'2026-11-08T00:00:00Z'};
    else if(path==='/mail-api/dispatch/preview') data={service:'example.com',candidates:[{provider:'mailtm',displayName:'Mail.tm',eligible:true,reason:''}]};
    else if(options.method==='DELETE') {inboxes=[];data={ok:true};}
    return {ok:status<300,status,json:async()=>data};
  };
  const context={document:{querySelector:s=>nodes[s.slice(1)]||null,createElement:node,hidden:false,addEventListener:(k,v)=>documentEvents[k]=v},location,
    localStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v)},
    window:{addEventListener:(k,v)=>windowEvents[k]=v,confirm:()=>confirm,matchMedia:()=>({matches:mobile})},fetch,AbortController,Date,Option:function(text,value){this.textContent=text;this.value=value;},
    navigator:{clipboard:{writeText:async text=>{context.copied=text;}}},
    setTimeout:(fn,delay)=>{timers.set(++nextTimer,{fn,delay});return nextTimer;},clearTimeout:id=>timers.delete(id)};
  vm.runInNewContext(fs.readFileSync(__dirname+'/app.js','utf8'),context);
  const flush=async()=>{for(let i=0;i<6;i++)await new Promise(resolve=>setImmediate(resolve));};
  function deferred(){let resolve;const promise=new Promise(r=>{resolve=r;});return {promise,resolve};}
  return {nodes,fields,calls,timers,malicious,context,flush,storage,
    async switch(hash){location.hash=hash;windowEvents.hashchange();await flush();},
    async click(id){nodes[id].fire('click');await flush();},
    async create(){nodes['mail-create'].fire('submit');await flush();},
    fail(value=true){failMessages=value;},
    async visibility(hidden){context.document.hidden=hidden;documentEvents.visibilitychange();await flush();},
    delayDetail(){detailDelay=deferred();return detailDelay;},
    delayPoll(){pollDelay=deferred();return pollDelay;},
    detailError(value){codeError=value;},
    confirm(value){confirm=value;}
  };
}

test('mail workflow: lazy connection, list label, create, safe content, copy, dispatch and close', async()=>{
  const f=fixture(); assert.equal(f.calls.length,0);
  await f.switch('#mail');
  assert.equal(f.nodes['sms-view'].hidden,true);assert.equal(f.nodes['mail-tab'].attrs['aria-pressed'],'true');
  assert.equal(f.fields.provider.children.length,3);assert.equal(f.nodes['mail-controls'].disabled,false);
  const active=f.nodes['mail-inboxes'].children[0].children;
  assert.equal(active[0].textContent,'有效 · 1');
  assert.equal(active[1].children[0].textContent,'seed@example.com');
  assert.equal(f.nodes['mail-address'].textContent,'seed@example.com');
  await f.create();
  const create=f.calls.find(c=>c.method==='POST'&&c.url==='/mail-api/inbox');
  assert.equal(create.headers['X-CSRF-Token'],'csrf-test');
  assert.deepEqual(JSON.parse(create.body),{for:'example.com'});
  assert.equal(f.nodes['mail-address'].textContent,'new@example.com');
  assert.equal(f.nodes['mail-codes'].textContent,'123456');
  assert.equal(f.timers.size,1);
  const article=f.nodes['mail-messages'].children[0];
  assert.equal(article.children[1].textContent,f.malicious);
  article.fire('click');await f.flush();assert.equal(f.nodes['mail-detail'].textContent,f.malicious);
  await f.click('mail-copy');assert.equal(f.context.copied,'new@example.com');
  await f.click('mail-copy-code');assert.equal(f.context.copied,'123456');
  await f.click('mail-dispatch');assert.match(f.nodes['mail-dispatch-result'].textContent,/Mail.tm · 参与调度/);
  assert.equal(f.calls.filter(c=>c.url.endsWith('/code')).length,0);
  assert.equal([...f.timers.values()][0].delay,10000);
  await f.switch('#sms');assert.equal(f.timers.size,0);assert.equal(f.nodes['sms-view'].hidden,false);
  await f.switch('#mail');assert.equal(f.nodes['mail-address'].textContent,'new@example.com');assert.equal(f.timers.size,1);
  await f.click('mail-close');assert.equal(f.nodes['mail-actions'].hidden,true);assert.equal(f.timers.size,0);
  assert.ok(f.calls.some(c=>c.method==='DELETE'&&c.url.endsWith('/abc')));
});

test('existing address attach and inbox grouping by state or provider', async()=>{
  const f=fixture();await f.switch('#mail');
  await f.click('mail-mode-attach');
  assert.equal(f.nodes['mail-attach-hint'].hidden,false);
  assert.match(f.nodes['mail-attach-hint'].textContent,/name@catchmail\.io/);
  assert.match(f.nodes['mail-attach-hint'].textContent,/邮箱授权码/);
  const titles=()=>f.nodes['mail-inboxes'].children.map(s=>s.children[0].textContent);
  assert.deepEqual(titles(),['有效 · 1','已过期 · 1','已关闭 · 1']);
  const requests=f.calls.length;
  f.nodes['mail-group'].value='provider';f.nodes['mail-group'].fire('change');await f.flush();
  assert.equal(f.calls.length,requests);
  assert.deepEqual(titles(),['Mail.tm · 1','Catchmail.io · 1','ghost · 1']);
  f.fields.address.value='mine@maildrop.cc';
  await f.create();
  const post=f.calls.find(c=>c.method==='POST'&&c.url==='/mail-api/inbox');
  assert.deepEqual(JSON.parse(post.body),{for:'example.com',address:'mine@maildrop.cc'});
  assert.equal(f.nodes['mail-address'].textContent,'mine@maildrop.cc');
});

test('custom domain mailbox attach sends IMAP credentials in the POST body only', async()=>{
  const f=fixture();await f.switch('#mail');
  await f.click('mail-mode-attach');
  f.fields.address.value='me@gmail.com';f.fields.password.value='app-secret-16';
  await f.create();
  const first=f.calls.filter(c=>c.method==='POST'&&c.url==='/mail-api/inbox').pop();
  assert.deepEqual(JSON.parse(first.body),{for:'example.com',address:'me@gmail.com',password:'app-secret-16'});
  assert.equal(f.fields.password.value,'');
  f.fields.password.value='app-secret-16';
  f.fields.imapHost.value='imap.gmail.com';
  await f.create();
  const second=f.calls.filter(c=>c.method==='POST'&&c.url==='/mail-api/inbox').pop();
  assert.deepEqual(JSON.parse(second.body),{for:'example.com',address:'me@gmail.com',password:'app-secret-16',imapHost:'imap.gmail.com'});
  assert.equal(f.nodes['mail-address'].textContent,'me@gmail.com');
  assert.ok(f.calls.every(c=>!String(c.url).includes('app-secret-16')));
});

test('attach hint keeps the IMAP guidance without attach domains', async()=>{
  const f=fixture({attachDomains:[]});await f.switch('#mail');
  await f.click('mail-mode-attach');
  assert.equal(f.nodes['mail-attach-hint'].hidden,false);
  assert.doesNotMatch(f.nodes['mail-attach-hint'].textContent,/可直接接入/);
  assert.match(f.nodes['mail-attach-hint'].textContent,/邮箱授权码/);
});

test('static site disables mail without disturbing SMS', async()=>{
  const publicSite=fixture({local:false});await publicSite.switch('#mail');
  assert.equal(publicSite.calls.length,0);assert.equal(publicSite.nodes['mail-controls'].disabled,true);
  assert.match(publicSite.nodes['mail-status'].textContent,/本机管理服务的 \/preview\//);
  await publicSite.switch('#sms');assert.equal(publicSite.nodes['sms-view'].hidden,false);
});

test('index template keeps the authorization code field masked', ()=>{
  const html=fs.readFileSync(__dirname+'/index.gohtml','utf8');
  assert.match(html,/<input name="password" type="password"[^>]*autocomplete="off"/);
  assert.match(html,/<input name="imapHost"[^>]*autocomplete="off"/);
});

test('poll failure stops automatic requests and keeps the inbox available for retry', async()=>{
  const f=fixture();await f.switch('#mail');await f.create();f.fail();await f.click('mail-refresh');
  assert.equal(f.nodes['mail-auto'].checked,false);assert.equal(f.timers.size,0);
  assert.match(f.nodes['mail-status'].textContent,/上游不可用/);assert.equal(f.nodes['mail-address'].textContent,'new@example.com');
  f.fail(false);await f.click('mail-refresh');
  assert.equal(f.nodes['mail-codes'].textContent,'123456');assert.equal(f.nodes['mail-address'].textContent,'new@example.com');
});

test('search and status filters do not request the server; expired and closed selections never poll', async()=>{
  const f=fixture();await f.switch('#mail');
  const count=f.calls.length;
  f.nodes['mail-search'].value='catchmail';f.nodes['mail-search'].fire('input');
  assert.equal(f.nodes['mail-inboxes'].children.length,1);
  f.nodes['mail-inboxes'].children[0].children[1].fire('click');await f.flush();
  assert.equal(f.calls.length,count);assert.equal(f.timers.size,0);
  assert.equal(f.nodes['mail-refresh'].disabled,true);assert.match(f.nodes['mail-meta'].textContent,/已过期/);
  f.nodes['mail-search'].value='';f.nodes['mail-filter'].value='closed';f.nodes['mail-filter'].fire('change');
  f.nodes['mail-inboxes'].children[0].children[1].fire('click');await f.flush();
  assert.equal(f.nodes['mail-close'].disabled,true);assert.equal(f.calls.length,count);
});

test('selection restores from storage and polling pauses while the browser is hidden', async()=>{
  const f=fixture({saved:'old'});await f.switch('#mail');
  assert.equal(f.nodes['mail-address'].textContent,'old@catchmail.io');assert.equal(f.timers.size,0);
  f.nodes['mail-inboxes'].children[0].children[1].fire('click');await f.flush();
  assert.equal(f.storage.get('identity.mail.selected'),'seed');assert.equal(f.timers.size,1);
  await f.visibility(true);assert.equal(f.timers.size,0);
  const count=f.calls.length;await f.visibility(false);assert.equal(f.calls.length,count+1);assert.equal(f.timers.size,1);
});

test('mode isolation ignores stale credentials and closing can be cancelled', async()=>{
  const f=fixture();await f.switch('#mail');await f.click('mail-mode-attach');
  assert.equal(f.fields.address.required,true);assert.equal(f.fields.domain.disabled,true);
  f.fields.address.value='private@gmail.com';f.fields.password.value='secret';
  await f.click('mail-mode-create');await f.create();
  const post=f.calls.find(c=>c.method==='POST');assert.deepEqual(JSON.parse(post.body),{for:'example.com'});
  f.confirm(false);await f.click('mail-close');assert.ok(!f.calls.some(c=>c.method==='DELETE'));
  assert.equal(f.timers.size,1);
});

test('a late message detail cannot replace another message or another inbox', async()=>{
  const f=fixture();await f.switch('#mail');const delay=f.delayDetail();
  f.nodes['mail-messages'].children[0].fire('click');await f.flush();
  f.nodes['mail-messages'].children[1].fire('click');await f.flush();delay.resolve();await f.flush();
  assert.equal(f.nodes['mail-detail'].textContent,'second body');
  const next=f.delayDetail();f.nodes['mail-messages'].children[0].fire('click');await f.flush();
  f.nodes['mail-inboxes'].children[1].children[1].fire('click');await f.flush();next.resolve();await f.flush();
  assert.equal(f.nodes['mail-detail'].hidden,true);assert.equal(f.nodes['mail-detail'].textContent,'');
});

test('an aborted old poll cannot stop automatic polling for the next selection', async()=>{
  const f=fixture();await f.switch('#mail');const delay=f.delayPoll();await f.click('mail-refresh');
  await f.switch('#sms');await f.switch('#mail');delay.resolve();await f.flush();
  assert.equal(f.nodes['mail-auto'].checked,true);assert.equal(f.timers.size,1);
  f.detailError('验证码正文读取失败');await f.click('mail-refresh');
  assert.match(f.nodes['mail-code-source'].textContent,/验证码正文读取失败/);
});

test('mobile selections reveal the message list and reader', async()=>{
  const f=fixture({mobile:true});await f.switch('#mail');
  f.nodes['mail-inboxes'].children[0].children[1].fire('click');await f.flush();
  assert.equal(f.nodes['mail-messages'].scrolled,true);
  f.nodes['mail-messages'].children[0].fire('click');await f.flush();
  assert.equal(f.nodes['mail-reader-content'].scrolled,true);
});
