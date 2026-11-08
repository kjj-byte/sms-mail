// Capture logged-in LongSMS / Haozhu prices into data/web-prices.json.
// First run: node tools/web-prices.mjs --login   (log in manually; captcha stays human)
// Later runs: node tools/web-prices.mjs --auto     (reuses .cache/web-prices-profile)
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const args = new Map();
for (let i = 2; i < process.argv.length; i++) {
  const a = process.argv[i];
  if (a.startsWith('--')) {
    const next = process.argv[i + 1];
    args.set(a.slice(2), next && !next.startsWith('--') ? (i++, next) : true);
  }
}
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const out = path.resolve(args.get('out') || path.join(root, 'data', 'web-prices.json'));
const profile = path.resolve(args.get('profile') || path.join(root, '.cache', 'web-prices-profile'));
const login = args.has('login');
const withCountries = args.has('countries');
const waitSeconds = Number(args.get('wait') || (login ? 300 : 20));
const maxRequests = Number(args.get('max-requests') || 3000);
const port = 9300 + Math.floor(Math.random() * 400);

function findChrome() {
  const candidates = [
    process.env.CHROME_PATH,
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
    'C:/Program Files/Microsoft/Edge/Application/msedge.exe',
  ];
  return candidates.find((p) => p && fs.existsSync(p));
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function waitHTTP(url, ms) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    try { const r = await fetch(url); if (r.ok) return true; } catch {}
    await sleep(200);
  }
  return false;
}

class Page {
  constructor(wsUrl) { this.ws = new WebSocket(wsUrl); this.id = 0; this.pending = new Map(); this.closed = false; }
  async open() {
    await new Promise((r, j) => { this.ws.onopen = r; this.ws.onerror = j; });
    this.ws.onclose = () => {
      this.closed = true;
      for (const resolve of this.pending.values()) resolve({ error: { message: 'browser closed' } });
      this.pending.clear();
    };
    this.ws.onmessage = (e) => {
      const m = JSON.parse(e.data);
      if (m.id && this.pending.has(m.id)) { this.pending.get(m.id)(m); this.pending.delete(m.id); }
    };
    await this.send('Page.enable');
    await this.send('Runtime.enable');
  }
  send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = ++this.id;
      this.pending.set(id, (m) => m.error ? reject(new Error(m.error.message)) : resolve(m.result));
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }
  async eval(expression) {
    const r = await this.send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.text || 'page evaluation failed');
    return r.result.value;
  }
  async nav(url) { await this.send('Page.navigate', { url }); await sleep(2500); }
  close() { try { this.ws.close(); } catch {} }
}

async function newPage(url) {
  const t = await (await fetch(`http://127.0.0.1:${port}/json/new?${url}`, { method: 'PUT' })).json();
  const p = new Page(t.webSocketDebuggerUrl);
  await p.open();
  return p;
}

function findList(body, keys = ['data', 'list', 'items', 'rows', 'services', 'countries', 'projects']) {
  if (Array.isArray(body)) return body;
  if (!body || typeof body !== 'object') return [];
  for (const k of keys) {
    if (Array.isArray(body[k])) return body[k];
    const nested = findList(body[k], keys);
    if (nested.length) return nested;
  }
  return [];
}
function text(v) { return v == null ? '' : String(v).trim(); }
function truncBytes(s, max = 160) {
  const enc = new TextEncoder();
  if (enc.encode(s).length <= max) return s;
  let out = '';
  for (const ch of s) {
    if (enc.encode(out + ch).length > max) break;
    out += ch;
  }
  return out;
}
function decimal(v) {
  const s = text(v);
  return /^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$/.test(s) ? s : '';
}
function number(v) { const n = Number(v); return Number.isFinite(n) ? n : 0; }
function storage(page) {
  return page.eval(`(()=>{const o={};for(const k of Object.keys(localStorage))o[k]=String(localStorage.getItem(k)||'');for(const k of Object.keys(sessionStorage))o['session:'+k]=String(sessionStorage.getItem(k)||'');return o;})()`);
}
function pickToken(store) {
  for (const k of ['token', 'access_token', 'accessToken', 'auth_token', 'jwt', 'session:token']) {
    const v = store[k];
    if (v && v.length > 12 && !v.startsWith('{') && !v.startsWith('[')) return v;
  }
  for (const v of Object.values(store)) {
    if (v && v.startsWith('{')) {
      try {
        const o = JSON.parse(v);
        for (const k of ['token', 'access_token', 'accessToken', 'auth_token', 'jwt']) {
          if (o[k] && String(o[k]).length > 12) return String(o[k]);
        }
      } catch {}
    }
  }
  for (const v of Object.values(store)) {
    if (v && v.length > 20 && /^(eyJ|Bearer |[A-Za-z0-9._-]{24,})/.test(v)) return v.replace(/^Bearer /, '');
  }
  return '';
}
async function pageFetch(page, url, token) {
  const expr = `(async()=>{const h={'Accept':'application/json'};${token ? `h['Authorization']='Bearer '+${JSON.stringify(token)};` : ''}const r=await fetch(${JSON.stringify(url)},{credentials:'include',headers:h});const t=await r.text();let b;try{b=JSON.parse(t)}catch{b={raw:t.slice(0,500)}}return {status:r.status,body:b};})()`;
  return page.eval(expr);
}

async function longsmsToken(page, navigate = false) {
  if (navigate) await page.nav('https://longsms.net/app/shop');
  return pickToken(await storage(page));
}
async function haozhumaToken(page, navigate = false) {
  if (navigate) await page.nav('https://h5.haozhuma.com/');
  return pickToken(await storage(page));
}

async function captureLongSMS(page, token, offers, warnings, budget) {
  const services = await pageFetch(page, '/api/user/short-term/browse/services?lang=en', token);
  if (services.status !== 200) { warnings.push(`LongSMS 短期服务目录 HTTP ${services.status}`); return; }
  const list = findList(services.body);
  if (!list.length) { warnings.push('LongSMS 短期服务目录为空'); return; }
  let serviceCount = 0, priceCount = 0;
  for (const svc of list) {
    if (budget.left-- <= 0) { warnings.push('LongSMS 短期服务达到请求上限，未完整'); return; }
    const code = text(svc.code || svc.service_code || svc.id);
    const name = truncBytes(text(svc.name || svc.name_en || svc.name_zh || svc.chn || code), 160);
    if (!code || !name) continue;
    serviceCount++;
    const amount = decimal(svc.price_usd ?? svc.price ?? svc.min_price);
    if (amount) {
      offers.push({
        provider: 'longsms', product: 'longsms-short', country: '未知', service: name,
        number_type: '未注明', term: '20 分钟', scope: `service_code=${code}`, amount,
        currency: 'USD', unit: '每次激活', kind: 'starting',
        source: 'https://longsms.net/api/user/short-term/browse/services?lang=en',
        evidence: JSON.stringify({ service: name, code, price_usd: svc.price_usd, qty: svc.qty }).slice(0, 1500),
      });
      priceCount++;
    }
    if (!withCountries) continue;
    if (budget.left-- <= 0) { warnings.push('LongSMS 短期国家目录达到请求上限，未完整'); return; }
    const countries = await pageFetch(page, `/api/user/short-term/browse/countries?service_code=${encodeURIComponent(code)}`, token);
    if (countries.status !== 200) { warnings.push(`LongSMS ${name} 国家目录 HTTP ${countries.status}`); continue; }
    for (const c of findList(countries.body)) {
      const from = decimal(c.from_price_usd ?? c.price ?? c.min_price);
      const cname = text(c.country_name || c.name || c.chn || c.country || c.code);
      if (!from || !cname) continue;
      offers.push({
        provider: 'longsms', product: 'longsms-short', country: cname, service: name,
        number_type: text(c.card_type || c.type) === 'mobile' ? '移动号' : text(c.card_type || c.type) === 'virtual' ? '虚拟号' : '未注明',
        term: '20 分钟', scope: `service_code=${code}`, amount: from, currency: 'USD', unit: '每次激活', kind: 'starting',
        source: 'https://longsms.net/api/user/short-term/browse/countries?service_code=' + encodeURIComponent(code),
        evidence: JSON.stringify({ service: name, code, country: cname, country_hint_id: c.country_hint_id, from_price_usd: c.from_price_usd, stock: c.stock }).slice(0, 1500),
      });
      priceCount++;
    }
  }
  if (!priceCount) warnings.push('LongSMS 短期目录已读取，但没有有效报价');
  console.log(`LongSMS: ${serviceCount} 个服务，${priceCount} 条短期起价`);
}

async function captureHaozhuma(page, token, offers, warnings) {
  const api = 'https://api.zsnixp.cn/v1';
  const me = await pageFetch(page, api + '/me', token);
  if (me.status !== 200 || me.body?.code !== 0) { warnings.push(`豪猪登录态无效：${me.status}`); return; }
  // shortcut: keyword sweep covers names containing two ASCII letters/digits; Chinese-only names may be absent.
  const projects = await page.eval(`(async()=>{const token=${JSON.stringify(token)};const chars='abcdefghijklmnopqrstuvwxyz0123456789';const pairs=[];for(const a of chars)for(const b of chars)pairs.push(a+b);const map=new Map();let i=0;async function worker(){while(i<pairs.length){const k=pairs[i++];try{const r=await fetch('https://api.zsnixp.cn/v1/projects?keyword='+k,{credentials:'include',headers:{'Authorization':'Bearer '+token}});const b=await r.json();if(b.code===0&&Array.isArray(b.data))for(const p of b.data)map.set(String(p.id),{id:String(p.id),name:String(p.name||''),price:String(p.price??'')});}catch{}}}await Promise.all(Array.from({length:12},worker));return [...map.values()];})()`);
  let count = 0;
  for (const p of projects) {
    const price = decimal(p.price);
    const service = truncBytes(text(p.name).replace(/[\u0000-\u001f\u007f]+/g, ' '), 160);
    if (!price || !service) continue;
    offers.push({
      provider: 'haozhuma', product: 'haozhuma-once', country: '中国', service,
      number_type: '未注明', term: 'single activation', scope: 'project_id=' + p.id,
      amount: price, currency: 'CNY', unit: '每次激活', kind: 'exact',
      source: api + '/me', evidence: JSON.stringify(p).slice(0, 1500),
    });
    count++;
  }
  if (!count) warnings.push('豪猪项目目录已读取，但没有带价格的字段');
  console.log(`豪猪: ${count} 条项目报价`);
}

async function main() {
  const chrome = findChrome();
  if (!chrome) { console.error('未找到 Chrome/Edge；网页价格抓取跳过'); process.exit(0); }
  fs.mkdirSync(profile, { recursive: true });
  const chromeArgs = [
    login ? '--window-size=1200,900' : '--headless=new',
    '--disable-gpu', '--no-first-run', '--no-default-browser-check',
    `--user-data-dir=${profile}`, `--remote-debugging-port=${port}`, 'about:blank',
  ];
  const child = spawn(chrome, chromeArgs, { stdio: 'ignore' });
  try {
    if (!(await waitHTTP(`http://127.0.0.1:${port}/json/version`, 15000))) throw new Error('浏览器启动失败');
    const lsPage = await newPage('https://longsms.net/en/login');
    const hzPage = await newPage('https://h5.haozhuma.com/login.php');
    let lsToken = '';
    let hzToken = '';
    if (login) {
      console.log('请在弹出的 Chrome 中登录 LongSMS 和豪猪（含验证码）。脚本只保存登录态，不保存密码。');
      const end = Date.now() + waitSeconds * 1000;
      while (Date.now() < end) {
        lsToken = await longsmsToken(lsPage).catch(() => '');
        hzToken = await haozhumaToken(hzPage).catch(() => '');
        if (lsToken && hzToken) break;
        if (lsPage.closed && hzPage.closed) {
          console.log('Chrome 已关闭。登录态已写入 profile；稍后运行 --auto 即可复用。');
          break;
        }
        await sleep(4000);
      }
    } else {
      lsToken = await longsmsToken(lsPage, true).catch(() => '');
      hzToken = await haozhumaToken(hzPage, true).catch(() => '');
    }
    if (!lsToken && !hzToken) {
      console.log('没有可复用的网页登录态；跳过网页价格抓取。先运行 --login 登录一次。');
      process.exitCode = 0;
      return;
    }
    const offers = [], warnings = [], budget = { left: maxRequests };
    if (lsToken) await captureLongSMS(lsPage, lsToken, offers, warnings, budget).catch((e) => warnings.push('LongSMS：' + e.message));
    else warnings.push('LongSMS 未登录，未读取短期报价');
    if (hzToken) await captureHaozhuma(hzPage, hzToken, offers, warnings).catch((e) => warnings.push('豪猪：' + e.message));
    else warnings.push('豪猪未登录，未读取项目报价');
    if (!offers.length) {
      console.error(warnings.join('；') || '没有取得网页报价');
      process.exitCode = login ? 1 : 0;
      return;
    }
    const capturedAt = new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
    fs.mkdirSync(path.dirname(out), { recursive: true });
    fs.writeFileSync(out, JSON.stringify({ schema_version: 1, captured_at: capturedAt, offers, warnings }, null, 2) + '\n', { mode: 0o600 });
    console.log(`已写入 ${offers.length} 条网页报价：${out}`);
    if (warnings.length) console.error('警告：' + warnings.join('；'));
  } finally {
    try { child.kill(); } catch {}
  }
}

main().catch((e) => { console.error(e.message); process.exit(1); });
