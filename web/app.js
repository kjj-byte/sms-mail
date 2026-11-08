(() => {
  'use strict';
  const form = document.querySelector('#filters');
  if (!form) return;
  const ledger = document.querySelector('#ledger');
  const ledgerMore = document.querySelector('#ledger-more');
  const keys = ['area','kind','q','provider','country','service','term','refund','sort'];
  const filterKeys = ['area','kind','q','provider','country','service','term','refund'];
  const count = document.querySelector('#count');
  const notice = document.querySelector('#notice');
  const field = name => form.elements.namedItem(name);
  const compareNames = new Intl.Collator().compare;
  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const countryLabel = r => r.c && r.c !== '未知' ? r.c : (r.ok === 'starting' ? '跨国起价（国家未指定）' : '国家未注明');
  const offerLabel = k => ({exact:'具体报价',starting:'公开起价',unknown:'价格待核实',login:'登录官网查看价格'}[k] || '价格待核实');
  // Channel-level recharge facts, shown once in a dropdown next to the results
  // header (not on every card). Amount is c.d (already hidden upstream when it
  // only says "needs login"); empty means the site has no public amount.
  const depositRow = (name, c, url) => {
    const hasAmt = !!c.d;
    const amount = hasAmt ? esc(c.d) : '官网未公开';
    const pay = c.pm && c.pm.length
      ? c.pm.map(m => `<span class="deposit-chip">${esc(m)}</span>`).join('')
      : `官网注册登录后充值页查看${url ? ` <a href="${esc(url)}" target="_blank" rel="noopener noreferrer">官网↗</a>` : ''}`;
    return `<div class="deposit-item${hasAmt ? '' : ' is-unknown'}">` +
      `<div class="deposit-item-head"><span class="deposit-item-name">${esc(name)}</span>` +
      (c.v ? `<span class="deposit-ver">${esc(c.v)} 核对</span>` : '') + `</div>` +
      `<div class="deposit-amt-row"><span class="deposit-amt-label">起充</span>` +
      `<span class="deposit-amt${hasAmt ? '' : ' muted'}">${amount}</span></div>` +
      `<div class="deposit-pay-row"><span class="deposit-pay-label">支付</span>` +
      `<span class="deposit-chips">${pay}</span></div>` +
      `</div>`;
  };
  function renderDepositDropdown() {
    const box = document.querySelector('#deposit-list');
    if (!box) return;
    const seen = new Set();
    const parts = [];
    for (const r of rows) {
      if (seen.has(r.pi)) continue;
      seen.add(r.pi);
      const c = channels[r.pi];
      if (!c) continue;
      parts.push(depositRow(r.p, c, channelURL(r.pi)));
    }
    box.innerHTML = parts.join('');
    const cnt = document.querySelector('#deposit-count');
    if (cnt) cnt.textContent = parts.length ? parts.length + ' 个渠道' : '';
    positionDepositPanel();
  }
  // The dropdown can sit anywhere along the header, so clamp its panel inside
  // the viewport instead of letting it overflow (CSS alone can't clamp both sides).
  function positionDepositPanel() {
    const dd = document.querySelector('#deposit-dropdown');
    if (!dd) return;
    const panel = dd.querySelector('.deposit-panel');
    if (!panel) return;
    panel.style.left = '';
    const r = panel.getBoundingClientRect();
    if (!r.width) return; // hidden while <details> closed
    const pad = 12;
    if (r.right > window.innerWidth - pad) {
      panel.style.left = (window.innerWidth - pad - r.width - r.left) + 'px';
    }
    if (parseFloat(panel.style.left || '0') + r.left < pad) {
      panel.style.left = (pad - r.left) + 'px';
    }
  }
  const ddEl = document.querySelector('#deposit-dropdown');
  if (ddEl) ddEl.addEventListener('toggle', () => { if (ddEl.open) positionDepositPanel(); });
  window.addEventListener('resize', () => { if (ddEl && ddEl.open) positionDepositPanel(); });
  const channelURL = pi => (rows.find(row => row.pi === pi) || {}).h || '';

  // ---- Data source: compact rows loaded asynchronously from rows.json (keeps
  // index.html small so the page opens fast even on slow connections).
  let rows = [];
  let products = {};
  let channels = {};
  let pool = [''];
  let rateDate = '';
  function applyPayload(payload) {
    products = payload.products || {};
    channels = payload.channels || {};
    pool = payload.pool || [''];
    rateDate = payload.rate_date || '';
    rows = (payload.rows || []).map((r, i) => {
      const p = products[r.pid] || {};
      r.order = i;
      r.priced = r.a != null && r.a !== '';
      r.area = r.k === 'free' ? 'free' : (r.k === 'pending' ? 'unknown' : 'paid');
      // expand pooled string indexes back to their values
      r.t = pool[r.t] || ''; r.h = pool[r.h] || ''; r.me = pool[r.me] || ''; r.os = pool[r.os] || '';
      r.ov = pool[r.ov] || ''; r.ca = pool[r.ca] || '';
      r.n = pool[r.n] || ''; r.sc = pool[r.sc] || '';
      // pull product-level fields up for the renderer
      r.pn = p.pn || ''; r.d = p.d || ''; r.rp = p.rp || ''; r.rn = p.rn || ''; r.ru = p.ru || '';
      r.co = p.co || ''; r.st = p.st || ''; r.ar = p.ar || ''; r.rs = p.rs || ''; r.on = p.on || '';
      r.ps = p.ps || []; r.m = p.m || ''; r.pv = p.pv || '';
      return r;
    });
    byChannel = {};
    for (const r of rows) {
      if (!r.s) continue;
      const k = r.pi + '|' + r.s;
      (byChannel[k] || (byChannel[k] = [])).push(r);
    }
    renderDepositDropdown();
  }
  const PAGE = 60;
  let shown = 0, matched = [];

  function options(name, values, labels = {}) {
    const select = field(name), current = select.value;
    const unique = [...new Set(values)].sort(compareNames);
    const fragment = document.createDocumentFragment();
    fragment.appendChild(new Option(select.options[0].textContent, ''));
    unique.forEach(value => fragment.appendChild(new Option(labels[value] || value, value)));
    select.replaceChildren(fragment);
    select.value = unique.includes(current) ? current : '';
  }
  options('country', rows.map(r => r.c), { '未知': '国家未指定（含跨国起价）' });
  options('provider', rows.map(r => r.pi), Object.fromEntries(rows.map(r => [r.pi, r.p])));
  const serviceSelect = field('service');
  let serviceChannels = new Map();
  // Keep thousands of native options out of typing/layout work until the dropdown is used.
  // shortcut: focus still mounts all matches; use a paged chooser if dropdown opening becomes a bottleneck.
  function serviceOptions(current = serviceSelect.value) {
    const names = document.activeElement === serviceSelect ? [...serviceChannels.keys()] : serviceChannels.has(current) ? [current] : [];
    options('service', names, Object.fromEntries(names.map(name => [name, `${name}（${serviceChannels.get(name).size} 个渠道）`])));
    serviceSelect.value = serviceChannels.has(current) ? current : '';
  }
  serviceSelect.addEventListener('focus', () => serviceOptions());
  serviceSelect.addEventListener('blur', () => serviceOptions());
  function services(current = serviceSelect.value) {
    const words = field('q').value.trim().toLowerCase().split(/\s+/);
    const country = field('country').value, provider = field('provider').value;
    serviceChannels = new Map();
    for (const r of rows) {
      if (!r.s || (country && r.c !== country) || (provider && r.pi !== provider) || !words.every(word => r.q.includes(word))) continue;
      if (!serviceChannels.has(r.s)) serviceChannels.set(r.s, new Set());
      serviceChannels.get(r.s).add(r.pi);
    }
    serviceOptions(current);
  }

  const cmp = (a, b) => a === b ? 0 : a < b ? -1 : 1;
  const fieldsCmp = (a, b, names) => {
    for (const name of names) { const c = cmp(a[name] || '', b[name] || ''); if (c) return c; }
    return 0;
  };
  function decimalCmp(a, b) {
    const [ai, af = ''] = a.split('.'), [bi, bf = ''] = b.split('.');
    return cmp(ai.length, bi.length) || cmp(ai, bi) || cmp(af.padEnd(Math.max(af.length, bf.length), '0'), bf.padEnd(Math.max(af.length, bf.length), '0'));
  }
  const curRank = c => ['USD','$','US$'].includes((c || '').toUpperCase()) ? 0 : 1;
  function priceCmp(a, b, dir) {
    const kind = cmp(a.k, b.k);
    if (kind) return kind;
    if (a.priced !== b.priced) return a.priced ? -1 : 1;
    if (!a.priced) return a.order - b.order;
    const ca = curRank(a.cu), cb = curRank(b.cu);
    if (ca !== cb) return ca - cb;
    // price_asc/price_desc are a true global price order: do not group by term/unit/kind first.
    const price = decimalCmp(a.a, b.a);
    if (price) return dir * price;
    return a.order - b.order;
  }

  // Group offers by provider+service so each card can show the channel's full list.
  // Include cross-country starting prices and listings without a known country.
  // Populated in applyPayload after rows load.
  let byChannel = {};

  // Build one card element from a compact row.
  function cardHTML(r) {
    const now = Date.now();
    // Date-only verification stamps use UTC midnight, matching Go's date parser.
    const stale = r.w || [r.ov, r.pv].some(date => now - Date.parse(date + 'T00:00:00Z') > 7 * 86400000) || (r.ca && now - Date.parse(r.ca) > 86400000);
    const buyLabel = r.k === 'free' ? (r.iv ? '前往邀请注册' : '查看免费号码') : (r.lf ? '前往渠道官网' : '前往官网购买');
    const sponsored = r.iv ? ' sponsored' : '';
    if (r.lf) {
      return `<article class="card compact">` +
        `<div class="c-head"><div class="c-name"><h3>${esc(r.p)}</h3><p>${esc(r.s)} · ${esc(r.pn)}</p></div>` +
        `<span class="badge muted-badge">已列出此接码对象，价格待获取</span></div>` +
        (r.sm ? `<p class="badge warn">${esc(r.sm)}</p>` : '') +
        `<p class="muted">价格与库存以官网为准。${stale ? '资料已过维护周期，待更新。' : ''}</p>` +
        `<p class="stamp">对象采集 ${esc(r.ca)} · <a href="${esc(r.os)}" target="_blank" rel="noopener noreferrer">对象来源</a></p>` +
        `<a class="btn" data-purchase="${esc(r.pi)}" href="${esc(r.h)}" target="_blank" rel="noopener noreferrer${sponsored}">前往渠道官网</a>` +
        `</article>`;
    }
    const price = r.priced
      ? `<strong>${r.oc ? '≈ ' : ''}${esc(r.a)}</strong><span class="cur">${esc(r.cu)}</span>`
      : `<strong class="muted-price">${esc(offerLabel(r.ok))}</strong>`;
    const tags = [
      `<span class="badge kind-${esc(r.k)}">${esc(kindLabel(r.k))}</span>`,
      `<span class="badge">${esc(offerLabel(r.ok))}</span>`,
      `<span class="badge">${esc(r.t)}</span>`,
      `<span class="badge refund-${esc(r.r)}">退款：${esc(r.r === 'refundable' ? '未收码可退' : r.r === 'nonrefundable' ? '不可退' : r.r === 'conditional' ? '有条件' : r.r === 'conflicting' ? '资料冲突，官网确认' : r.r === 'na' ? '不适用' : '未知')}</span>`
    ];
    if (stale) tags.push(`<span class="badge warn" title="资料已过维护周期，待更新">资料已过维护周期，待更新</span>`);
    if (r.sm) tags.push(`<span class="badge warn">${esc(r.sm)}</span>`);
    const manage = r.m ? `<a class="btn ghost" href="${esc(r.m)}" target="_blank" rel="noopener noreferrer">官网续费 / 管理</a>` : '';
    let stamp = `规则核验 ${esc(r.pv || '')} · 报价核验 ${esc(r.ov || '')}`;
    if (r.ca) stamp += ` · ${r.lf ? '对象采集' : '价格采集'} ${esc(r.ca)}`;
    const group = r.s ? (byChannel[r.pi + '|' + r.s] || []) : [];
    const previewBtn = group.length > 1
      ? `<button type="button" class="btn ghost c-preview" data-provider="${esc(r.pi)}" data-service="${esc(r.s)}">本渠道全部 ${group.length} 个报价</button>`
      : '';
    return `<article class="card">` +
      `<div class="c-head"><div class="c-name"><h3>${esc(r.p)}</h3><p>${esc(r.s)} · ${esc(countryLabel(r))}</p></div>` +
      `<div class="c-price">${price}<small>${esc(r.u)}</small>${r.oc ? `<small class="original-price">原价 ${esc(r.oa)} ${esc(r.oc)} · ${esc(rateDate)} 汇率折算</small>` : ''}</div></div>` +
      `<div class="c-tags">${tags.join('')}</div>` +
      `<div class="c-meta"><span>可用时长 <b>${esc(r.d)}</b></span><span>期限内再收码 <b>${esc(r.rp)}</b></span><span>同号续费 <b>${esc(r.rn)}</b></span><span>结束后复用 <b>${esc(r.ru)}</b></span></div>` +
      `<div class="c-actions"><a class="btn" data-purchase="${esc(r.pi)}" href="${esc(r.h)}" target="_blank" rel="noopener noreferrer${sponsored}">${buyLabel}</a>${manage}${previewBtn}<small class="stamp">${stamp}</small></div>` +
      `<details class="c-more"><summary>限制、续费条件与官方来源</summary>` +
      `<p>${esc(r.co || '')}</p>` +
      (r.me ? `<p>读取方式：${esc(r.me)}</p>` : '') +
      `<p>号码类型：${esc(r.n)}；适用范围：${esc(r.sc)}</p>` +
      `<dl><dt>起算点</dt><dd>${esc(r.st || '')}</dd><dt>自动续费</dt><dd>${esc(r.ar || '')}</dd><dt>到期恢复原号</dt><dd>${esc(r.rs || '')}</dd><dt>在线方式</dt><dd>${esc(r.on || '')}</dd><dt>续费价格</dt><dd>${r.ra ? esc(r.cu) + ' ' + esc(r.ra) : '未知，官网确认'}</dd></dl>` +
      `<p>官网可能需要重新选择服务、国家和租期。${r.iv ? '本购买入口含管理员邀请信息。' : ''}</p>` +
      (r.ora ? `<p>续费原价 ${esc(r.ora)} ${esc(r.oc)}；美元金额按 ${esc(rateDate)} 汇率折算。</p>` : '') +
      `<ul>${(r.ps || []).map(u => `<li><a href="${esc(u)}" target="_blank" rel="noopener noreferrer">产品规则来源：${esc(u)}</a></li>`).join('')}${r.os ? `<li><a href="${esc(r.os)}" target="_blank" rel="noopener noreferrer">报价来源</a></li>` : ''}</ul></details>` +
      `</article>`;
  }
  function kindLabel(k) { return k === 'free' ? '免费公共号码' : k === 'once' ? '一次性接码' : k === 'rental' ? '可续费号码' : '产品类型待核实'; }

  function appendPage() {
    const slice = matched.slice(shown, shown + PAGE);
    const frag = document.createElement('div');
    frag.innerHTML = slice.map(cardHTML).join('');
    for (const node of Array.from(frag.children)) ledger.appendChild(node);
    shown += slice.length;
    ledgerMore.hidden = shown >= matched.length;
    ledgerMore.textContent = matched.length ? `已显示 ${shown} / ${matched.length} 条，点击加载更多` : '';
  }
  // Object browser: full searchable, paged list of every target.
  const objDataEl = document.querySelector('#obj-data');
  const allObjects = (objDataEl ? JSON.parse(objDataEl.textContent) : [])
    .sort((a, b) => (b.Channels - a.Channels) || compareNames(a.Name, b.Name));
  const objSearch = document.querySelector('#obj-search');
  const objList = document.querySelector('#obj-list');
  const objMore = document.querySelector('#obj-more');
  const explorer = document.querySelector('.target-explorer');
  if (explorer) {
    let resizeFrame;
    function sizeExplorer() {
      resizeFrame = 0;
      if (window.innerWidth <= 1100) return;
      const top = Math.max(30, explorer.getBoundingClientRect().top);
      explorer.style.setProperty('--explorer-height', `${Math.max(0, window.innerHeight - top)}px`);
    }
    const scheduleSize = () => { if (!resizeFrame) resizeFrame = requestAnimationFrame(sizeExplorer); };
    window.addEventListener('scroll', scheduleSize, {passive: true});
    window.addEventListener('resize', scheduleSize);
    new ResizeObserver(scheduleSize).observe(document.querySelector('.app-main-content'));
    sizeExplorer();
  }
  const OBJ_PAGE = 60;
  let objShown = 0, objFiltered = allObjects;
  function selectObject(name) {
    const el = field('service');
    const same = el.value === name;
    field('q').value = ''; services(same ? '' : name);
    notice.textContent = '';
    render(true);
  }
  function renderObjects(reset) {
    const term = (objSearch.value || '').trim().toLowerCase();
    if (reset || !objShown) { objShown = 0; objList.replaceChildren(); objFiltered = term ? allObjects.filter(o => o.Name.toLowerCase().includes(term)) : allObjects; }
    const next = objFiltered.slice(objShown, objShown + OBJ_PAGE);
    for (const o of next) {
      const li = document.createElement('li');
      const button = document.createElement('button');
      button.type = 'button'; button.className = 'obj-item'; button.dataset.name = o.Name;
      button.setAttribute('aria-pressed', String(field('service').value === o.Name));
      button.innerHTML = `<span class="obj-name"></span><span class="obj-ch">${o.Channels} 渠道</span>`;
      button.querySelector('.obj-name').textContent = o.Name;
      button.addEventListener('click', () => selectObject(o.Name));
      li.appendChild(button);
      objList.appendChild(li);
    }
    objShown += next.length;
    objMore.hidden = objShown >= objFiltered.length;
    objMore.textContent = objFiltered.length ? `显示 ${objShown} / ${objFiltered.length} 个对象，点击加载更多` : '没有匹配的接码对象';
  }
  if (objSearch) {
    renderObjects(true);
    objSearch.addEventListener('input', () => renderObjects(true));
    objMore.addEventListener('click', () => renderObjects(false));
  }

  // Keyword autocomplete (from object names, popularity-ranked).
  const suggestions = allObjects.map(o => o.Name);
  const qInput = field('q');
  const qList = document.querySelector('#q-suggest');
  let activeIndex = -1;
  function closeSuggest() { qList.hidden = true; qList.replaceChildren(); qInput.setAttribute('aria-expanded', 'false'); activeIndex = -1; }
  function pickSuggest(name) {
    qInput.value = name; closeSuggest();
    const old = field('service').value; services();
    if (old && old !== field('service').value) notice.textContent = '原接码对象不在当前列表中，已清除该对象选择。';
    render(true);
  }
  function showSuggest() {
    const term = qInput.value.trim().toLowerCase();
    if (term.length < 1) { closeSuggest(); return; }
    const prefixed = [], wordStart = [], substring = [];
    for (const name of suggestions) {
      const low = name.toLowerCase(); const idx = low.indexOf(term);
      if (idx === 0) prefixed.push(name);
      else if (idx > 0 && /[\s\-_/]/.test(low[idx - 1])) wordStart.push(name);
      else if (idx > 0) substring.push(name);
    }
    const m = [...prefixed, ...wordStart, ...substring].slice(0, 8);
    if (!m.length) { closeSuggest(); return; }
    qList.replaceChildren(...m.map(name => {
      const li = document.createElement('li'); li.setAttribute('role', 'option'); li.textContent = name;
      li.addEventListener('mousedown', e => { e.preventDefault(); pickSuggest(name); });
      return li;
    }));
    qList.hidden = false; qInput.setAttribute('aria-expanded', 'true'); activeIndex = -1;
  }
  if (qList) {
    qInput.addEventListener('input', showSuggest);
    qInput.addEventListener('focus', showSuggest);
    qInput.addEventListener('blur', () => setTimeout(closeSuggest, 120));
    qInput.addEventListener('keydown', e => {
      const items = [...qList.children];
      if (e.key === 'ArrowDown' && !qList.hidden) { e.preventDefault(); activeIndex = Math.min(activeIndex + 1, items.length - 1); items.forEach((it, i) => it.setAttribute('aria-selected', i === activeIndex)); }
      else if (e.key === 'ArrowUp' && !qList.hidden) { e.preventDefault(); activeIndex = Math.max(activeIndex - 1, 0); items.forEach((it, i) => it.setAttribute('aria-selected', i === activeIndex)); }
      else if (e.key === 'Enter' && !qList.hidden && activeIndex >= 0) { e.preventDefault(); pickSuggest(items[activeIndex].textContent); }
      else if (e.key === 'Escape') { closeSuggest(); }
    });
  }

  const quickChips = [...document.querySelectorAll('.quick-chip')];
  quickChips.forEach(button => button.addEventListener('click', () => {
    field('service').value = '';
    notice.textContent = '';
    pickSuggest(button.dataset.search);
    if (objSearch) { objSearch.value = button.dataset.search; renderObjects(true); }
  }));

  // Rows stay immutable after loading, so each sort order can be reused across filters.
  const sortedRows = new Map();
  function render(push = false) {
    const state = Object.fromEntries(keys.map(k => [k, field(k).value]));
    quickChips.forEach(button => {
      const active = state.q.trim().toLowerCase() === button.dataset.search.toLowerCase();
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', String(active));
    });
    const words = state.q.trim().toLowerCase().split(/\s+/);
    const sort = ['price_asc', 'price_desc'].includes(state.sort) ? state.sort : 'group';
    if (!sortedRows.has(sort)) {
      const ordered = rows.slice();
      if (sort === 'price_desc') ordered.sort((a, b) => priceCmp(a, b, -1));
      else if (sort === 'price_asc') ordered.sort((a, b) => priceCmp(a, b, 1));
      else ordered.sort((a, b) => fieldsCmp(a, b, ['pi','k','c','s','n','t','cu','u','sc','ok']) || priceCmp(a, b, 1));
      sortedRows.set(sort, ordered);
    }
    matched = sortedRows.get(sort).filter(row => {
      for (const k of filterKeys) {
        if (!state[k]) continue;
        if (k === 'q') { if (!words.every(word => row.q.includes(word))) return false; }
        else if (k === 'area') { if (row.area !== state[k]) return false; }
        else if (k === 'kind') { if (row.k !== state[k]) return false; }
        else if (k === 'provider') { if (row.pi !== state[k]) return false; }
        else if (k === 'country') { if (row.c !== state[k]) return false; }
        else if (k === 'service') { if (row.s !== state[k]) return false; }
        else if (k === 'term') { if (row.t !== state[k]) return false; }
        else if (k === 'refund') { if (row.r !== state[k]) return false; }
      }
      return true;
    });
    if (objList) objList.querySelectorAll('.obj-item').forEach(it => it.setAttribute('aria-pressed', String(it.dataset.name === state.service)));
    // Render only the first page of the current view (not tens of thousands of nodes).
    ledger.replaceChildren(); shown = 0; appendPage();
    count.textContent = `${matched.length} 条匹配渠道记录`;
    const emptyEl = document.querySelector('#empty');
    emptyEl.hidden = matched.length !== 0;
    emptyEl.textContent = '没有匹配的渠道记录。可清空关键词或更换接码对象；未成功拉取的来源不代表不支持该对象。';
    // Explain our collected coverage without making claims about upstream availability.
    if (matched.length === 0 && state.country) {
      const withoutCountry = rows.filter(row => {
        for (const k of filterKeys) {
          if (!state[k] || k === 'country') continue;
          if (k === 'q') { if (!words.every(word => row.q.includes(word))) return false; }
          else if (k === 'area') { if (row.area !== state[k]) return false; }
          else if (k === 'kind') { if (row.k !== state[k]) return false; }
          else if (k === 'provider') { if (row.pi !== state[k]) return false; }
          else if (k === 'service') { if (row.s !== state[k]) return false; }
          else if (k === 'term') { if (row.t !== state[k]) return false; }
          else if (k === 'refund') { if (row.r !== state[k]) return false; }
        }
        return true;
      });
      if (withoutCountry.length > 0) {
        emptyEl.innerHTML = `本站未收录所选国家「${esc(field('country').selectedOptions[0].textContent)}」下的匹配记录。其他国家或跨国起价不代表该国可用。<button type="button" class="btn ghost" id="clear-country">清除国家筛选，查看 ${withoutCountry.length} 条</button>`;
        const cb = emptyEl.querySelector('#clear-country');
        if (cb) cb.addEventListener('click', () => { field('country').value = ''; render(true); });
      }
    }
    document.querySelector('#selected').textContent = keys.filter(k => state[k] && k !== 'sort').map(k => {
      const el = field(k);
      return k === 'q' ? `搜索：${state[k]}` : `${el.closest('label').querySelector('.cell-label, span').textContent.trim()}：${el.selectedOptions[0].textContent}`;
    }).join('；');
    if (push) {
      const url = new URL(location.href);
      keys.forEach(k => state[k] ? url.searchParams.set(k, state[k]) : url.searchParams.delete(k));
      if (url.href !== location.href) history.pushState(null, '', url);
    }
  }
  function restore() {
    const params = new URLSearchParams(location.search);
    filterKeys.filter(k => k !== 'service').forEach(k => { field(k).value = params.get(k) || ''; });
    field('sort').value = params.get('sort') || 'price_asc';
    services(params.get('service') || '');
    render();
  }
  ledgerMore.addEventListener('click', appendPage);

  // Cards per row: segmented buttons drive a hidden <select> (the state source,
  // remembered in localStorage). The ledger grid reads the data-per-row attr.
  const perRowSel = document.querySelector('#per-row');
  const perRowSeg = document.querySelector('#per-row-seg');
  const applyPerRow = n => {
    if (ledger) ledger.dataset.perRow = String(n);
    if (perRowSeg) perRowSeg.querySelectorAll('button').forEach(b =>
      b.setAttribute('aria-pressed', b.dataset.n === String(n) ? 'true' : 'false'));
  };
  if (perRowSel) {
    let saved = '3';
    try { saved = localStorage.getItem('perRow') || '3'; } catch { }
    if (![...perRowSel.options].some(o => o.value === saved)) saved = '3';
    perRowSel.value = saved;
    applyPerRow(saved);
    const commit = n => {
      perRowSel.value = String(n);
      applyPerRow(n);
      try { localStorage.setItem('perRow', String(n)); } catch { }
    };
    perRowSel.addEventListener('change', () => commit(perRowSel.value));
    if (perRowSeg) perRowSeg.addEventListener('click', e => {
      const btn = e.target.closest('button');
      if (btn) commit(btn.dataset.n);
    });
  }

  // Channel offer preview: open a searchable list of every offer for one provider+service.
  const modal = document.querySelector('#chan-modal');
  const chanList = document.querySelector('#chan-list');
  const chanSearch = document.querySelector('#chan-search');
  const chanCount = document.querySelector('#chan-count');
  const chanTitle = document.querySelector('#chan-title');
  let chanRows = [];
  let chanTrigger;
  function rowLineHTML(x) {
    const priceText = x.priced ? `${x.oc ? '≈ ' : ''}${esc(x.a)} ${esc(x.cu)} · ${esc(offerLabel(x.ok))}` : esc(offerLabel(x.ok));
    return `<a class="chan-row" href="${esc(x.h)}" target="_blank" rel="noopener noreferrer">` +
      `<span class="chan-country">${esc(countryLabel(x))}${x.sm ? `<small>${esc(x.sm)}</small>` : ''}</span>` +
      `<span class="chan-price">${priceText}${x.oc ? `<small>原价 ${esc(x.oa)} ${esc(x.oc)} · ${esc(rateDate)} 汇率折算</small>` : ''}</span>` +
      `<span class="chan-term">${esc(x.t || '')}</span>` +
      `<span class="chan-go">前往</span></a>`;
  }
  function renderChan() {
    const term = (chanSearch.value || '').trim().toLowerCase();
    const list = term
      ? chanRows.filter(x => (x.c || '').toLowerCase().includes(term) || (x.a || '').includes(term) || (x.oa || '').includes(term))
      : chanRows;
    chanList.innerHTML = list.map(rowLineHTML).join('');
    chanCount.textContent = `${list.length} 个报价` + (term ? `（含「${term}」）` : '') + ' · 点击行前往该渠道官网';
  }
  function openChan(provider, service, trigger) {
    chanRows = (byChannel[provider + '|' + service] || []).slice().sort((a, b) => priceCmp(a, b, 1));
    chanTitle.textContent = `${chanRows[0] ? chanRows[0].p : provider} · ${service} 全部报价`;
    chanSearch.value = '';
    renderChan();
    chanTrigger = trigger;
    modal.showModal();
    chanSearch.focus();
  }
  function closeChan() { modal.close(); }
  if (modal) {
    ledger.addEventListener('click', e => {
      const b = e.target.closest('.c-preview');
      if (b) { e.preventDefault(); openChan(b.dataset.provider, b.dataset.service, b); }
    });
    modal.addEventListener('click', e => { if (e.target.closest('[data-close]')) closeChan(); });
    modal.addEventListener('keydown', e => {
      if (e.key !== 'Tab') return;
      const controls = [...modal.querySelectorAll('button, input, a[href]')];
      const first = controls[0], last = controls[controls.length - 1];
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
    });
    chanSearch.addEventListener('input', renderChan);
    modal.addEventListener('close', () => { if (chanTrigger) chanTrigger.focus(); });
  }

  form.addEventListener('submit', e => e.preventDefault());
  form.addEventListener('input', e => {
    notice.textContent = '';
    if (['country', 'provider', 'q'].includes(e.target.name)) {
      const old = field('service').value;
      services();
      if (old && old !== field('service').value) notice.textContent = '原接码对象不在当前列表中，已清除该对象选择。';
    }
    render(true);
  });
  form.addEventListener('reset', () => setTimeout(() => { field('sort').value = 'price_asc'; services(); notice.textContent = ''; render(true); }, 0));
  window.addEventListener('popstate', restore);
  form.hidden = false;
  // Load the price rows asynchronously, then run the initial render. The ?v=
  // asset version (from the meta tag) busts caches so a rebuilt site is picked
  // up without a manual hard refresh.
  const assetVer = (document.querySelector('meta[name="asset-ver"]') || {}).content || '';
  fetch('rows.json' + (assetVer ? '?v=' + encodeURIComponent(assetVer) : ''), {cache: 'no-cache'})
    .then(r => r.ok ? r.json() : {})
    .then(data => { applyPayload(data || {}); })
    .catch(() => { /* keep empty defaults; page still renders filters */ })
    .finally(() => { restore(); });
})();

(() => {
  'use strict';
  const $ = id => document.querySelector('#' + id);
  if (!$('mail-view')) return;
  const form = $('mail-create'), controls = $('mail-controls');
  const field = name => form.elements.namedItem(name);
  const local = location.protocol === 'http:' && location.hostname === '127.0.0.1' && location.pathname.startsWith('/preview/');
  const labels = {active: '有效', expired: '已过期', closed: '已关闭'};
  let csrf = '', ready = false, connecting = false, selected = null, timer, polling, controller, reading;
  let providers = [], inboxes = [], mode = 'create', selectedMessage = '', codeValue = '', pollAfter = 5000;
  const status = text => { $('mail-status').textContent = text; };
  function remember(value) { try { localStorage.setItem('identity.mail.selected', value); } catch {} }
  function recalled() { try { return localStorage.getItem('identity.mail.selected'); } catch { return ''; } }
  function state(inbox) {
    if (inbox.status === 'closed' || inbox.state === 'closed') return 'closed';
    const expires = inbox.expiresAt || inbox.expires_at;
    return inbox.state === 'expired' || (expires && new Date(expires).getTime() <= Date.now()) ? 'expired' : 'active';
  }
  const providerName = name => providers.find(p => p.name === name)?.displayName || name;
  const date = value => value && Number.isFinite(new Date(value).getTime()) ? new Date(value).toLocaleString() : '时间未知';
  async function api(path, method = 'GET', body, signal) {
    let response;
    try {
      response = await fetch('/mail-api' + path, {
        method, signal, cache: 'no-store', credentials: 'same-origin',
        headers: {'X-CSRF-Token': csrf, 'Content-Type': 'application/json'},
        body: body === undefined ? undefined : JSON.stringify(body)
      });
    } catch (e) {
      if (e.name === 'AbortError') throw e;
      throw new Error('网络连接失败，请检查本机邮箱服务后重试。', {cause: e});
    }
    const text = await response.text();
    let data;
    try { data = text ? JSON.parse(text) : {}; }
    catch {
      const error = new Error(response.ok ? '邮箱服务返回了无法识别的数据，请重试。' : `邮箱请求失败（HTTP ${response.status}），请重试。`);
      error.status = response.status; throw error;
    }
    if (!response.ok) { const error = new Error(`邮箱请求失败（HTTP ${response.status}）${data?.error ? `：${data.error}` : ''}`); error.status = response.status; throw error; }
    return data;
  }
  function stop() {
    clearTimeout(timer);
    if (controller) controller.abort();
    if (reading) reading.abort();
    controller = polling = reading = null;
  }
  function active() { return !$('mail-view').hidden && !document.hidden; }
  function schedule() {
    clearTimeout(timer);
    if (active() && selected && state(selected) === 'active' && $('mail-auto').checked) timer = setTimeout(() => run(refresh), pollAfter);
  }
  async function run(action, button) {
    if (button) button.disabled = true;
    try { await action(); }
    catch (e) { if (e.name !== 'AbortError') status(e.message); }
    finally { if (button) button.disabled = false; }
  }
  function inboxPath(inbox = selected) { return '/inbox/' + encodeURIComponent(inbox.id); }
  function empty(container, title, text) {
    const box = document.createElement('div'); box.className = 'mail-empty';
    const heading = document.createElement('h3'); heading.textContent = title;
    const hint = document.createElement('p'); hint.textContent = text;
    box.append(heading, hint); container.replaceChildren(box);
  }
  function clearInbox() {
    stop(); selected = null; selectedMessage = ''; codeValue = '';
    $('mail-address').textContent = '选择或创建邮箱'; $('mail-meta').textContent = '从左侧选择邮箱开始收件';
    $('mail-actions').hidden = true; $('mail-refresh').disabled = true;
    $('mail-code-panel').hidden = true; $('mail-codes').textContent = ''; $('mail-copy-code').disabled = true;
    $('mail-message-count').textContent = '0';
    empty($('mail-messages'), '等待选择邮箱', '创建临时邮箱，或从左侧选择已有邮箱。');
    $('mail-reader-empty').hidden = false; $('mail-reader-content').hidden = true;
    $('mail-detail').hidden = true; $('mail-detail').textContent = '';
  }
  function renderInboxes() {
    const container = $('mail-inboxes'), query = $('mail-search').value.trim().toLowerCase(), filter = $('mail-filter').value;
    container.replaceChildren();
    const visible = inboxes.filter(i => (!filter || filter === 'all' || state(i) === filter) && `${i.address} ${i.provider} ${providerName(i.provider)} ${i.target_service || ''}`.toLowerCase().includes(query));
    $('mail-inbox-count').textContent = String(inboxes.length);
    $('mail-overview').textContent = `${inboxes.filter(i => state(i) === 'active').length} 个有效邮箱 / ${providers.filter(p => p.enabled).length} 个内置渠道`;
    const groups = $('mail-group').value === 'provider'
      ? [...new Set(visible.map(i => i.provider))].map(name => ({label: providerName(name), items: visible.filter(i => i.provider === name)}))
      : Object.keys(labels).map(s => ({label: labels[s], items: visible.filter(i => state(i) === s)}));
    for (const group of groups.filter(g => g.items.length)) {
      const section = document.createElement('section'); section.className = 'mail-group';
      const title = document.createElement('h3'); title.textContent = `${group.label} · ${group.items.length}`; section.append(title);
      for (const inbox of group.items) {
        const button = document.createElement('button'); button.type = 'button'; button.className = 'mail-inbox-item';
        const address = document.createElement('strong'); address.textContent = inbox.address;
        const meta = document.createElement('span'); meta.textContent = `${providerName(inbox.provider)} · ${labels[state(inbox)]}${inbox.target_service ? ' · ' + inbox.target_service : ''}`;
        button.append(address, meta); button.setAttribute('aria-pressed', String(selected?.id === inbox.id));
        button.addEventListener('click', () => {
          run(() => select(inbox));
          if (window.matchMedia?.('(max-width:760px)').matches) $('mail-messages').scrollIntoView({block: 'start'});
        }); section.append(button);
      }
      container.append(section);
    }
    if (!visible.length) empty(container, inboxes.length ? '没有匹配的邮箱' : '还没有邮箱', inboxes.length ? '试试其他关键词或状态。' : '点击上方「新建 / 接入邮箱」开始。');
  }
  async function list() {
    inboxes = (await api('/inboxes?pageSize=100')).inboxes || [];
    if (selected) {
      const latest = inboxes.find(i => i.id === selected.id);
      if (latest) Object.assign(selected, latest);
      else { clearInbox(); remember(''); }
    }
    renderInboxes();
  }
  function showIdentity() {
    const current = state(selected), expires = selected.expiresAt || selected.expires_at;
    $('mail-address').textContent = selected.address;
    $('mail-meta').textContent = `${providerName(selected.provider)} · ${labels[current]} · ${expires ? '到期 ' + date(expires) : '长期接入'}${selected.target_service ? ' · ' + selected.target_service : ''}`;
    $('mail-actions').hidden = false; $('mail-close').disabled = current === 'closed'; $('mail-refresh').disabled = current !== 'active';
    return current;
  }
  async function select(inbox) {
    clearInbox(); selected = inbox; remember(inbox.id); renderInboxes();
    const current = showIdentity();
    if (current !== 'active') {
      empty($('mail-messages'), `邮箱${labels[current]}`, current === 'expired' ? '有效期已结束，可以关闭记录或新建邮箱。' : '此记录已停止收件，请新建邮箱。');
      status(`当前邮箱${labels[current]}，已停止收件。`); return;
    }
    $('mail-code-panel').hidden = false; $('mail-codes').textContent = '等待验证码';
    empty($('mail-messages'), '正在收件…', '正在从渠道获取邮件。'); await refresh();
  }
  function showCodes(verification = {}) {
    const codes = verification.codes || [];
    codeValue = codes[0]?.value || '';
    $('mail-codes').replaceChildren();
    for (const code of codes) {
      const button = document.createElement('button'); button.type = 'button'; button.className = code.type === 'link' ? 'mail-code-link' : 'mail-code';
      button.textContent = code.value; button.setAttribute('aria-label', `复制${code.type === 'link' ? '验证链接' : '验证码'} ${code.value}`);
      button.addEventListener('click', () => run(() => copy(code.value, '已复制，请核对邮件来源。'))); $('mail-codes').append(button);
    }
    if (!codes.length) $('mail-codes').textContent = verification.messageId ? '未识别到验证码' : '等待验证码';
    $('mail-copy-code').disabled = !codeValue;
    $('mail-code-source').textContent = verification.error || (verification.email ? `来自 ${verification.email.from || '未知发件人'} · ${date(verification.receivedAt)}` : '新邮件到达后自动识别，可点击候选值复制。');
  }
  async function openMessage(inbox, message) {
    if (reading) reading.abort();
    const current = new AbortController(); reading = current; selectedMessage = message.id;
    for (const row of $('mail-messages').children) row.setAttribute('aria-pressed', String(row.dataset.messageId === message.id));
    $('mail-reader-empty').hidden = true; $('mail-reader-content').hidden = false;
    $('mail-detail-subject').textContent = message.subject || '（无主题）';
    $('mail-detail-from').textContent = `${message.from || '未知发件人'} · ${date(message.receivedAt)}`;
    $('mail-detail').hidden = false; $('mail-detail').textContent = '正在读取正文…';
    if (window.matchMedia?.('(max-width:1200px)').matches) $('mail-reader-content').scrollIntoView({block: 'start'});
    try {
      const detail = await api(inboxPath(inbox) + '/messages/' + encodeURIComponent(message.id), 'GET', undefined, current.signal);
      if (selected !== inbox || reading !== current || current.signal.aborted) return;
      $('mail-detail').textContent = detail.text || detail.excerpt || '无正文';
    } catch (e) {
      if (selected === inbox && reading === current && e.name !== 'AbortError') $('mail-detail').textContent = '正文读取失败，请再次点击该邮件重试。';
      throw e;
    }
  }
  async function refresh() {
    if (!selected || polling || !active()) return;
    if (showIdentity() !== 'active') { stop(); renderInboxes(); status('邮箱已过期或关闭，已停止收件。'); return; }
    const inbox = selected, current = new AbortController(); controller = polling = current; clearTimeout(timer);
    try {
      const data = await api(inboxPath(inbox) + '/messages?includeCode=true', 'GET', undefined, current.signal);
      if (selected !== inbox || current.signal.aborted) return;
      if (data.status === 'closed') { inbox.status = 'closed'; await select(inbox); return; }
      pollAfter = Math.max(5000, Math.min(60000, (Number(data.pollAfter) || 5) * 1000));
      showCodes(data.verification);
      const container = $('mail-messages'); container.replaceChildren();
      const messages = data.messages || []; $('mail-message-count').textContent = String(messages.length);
      for (const message of messages) {
        const button = document.createElement('button'); button.type = 'button'; button.className = 'mail-message'; button.dataset.messageId = message.id;
        button.setAttribute('aria-pressed', String(selectedMessage === message.id));
        const from = document.createElement('span'); from.className = 'mail-message-from'; from.textContent = message.from || '未知发件人';
        const title = document.createElement('strong'); title.textContent = message.subject || '（无主题）';
        const excerpt = document.createElement('span'); excerpt.textContent = message.excerpt || '点击阅读邮件正文';
        const time = document.createElement('time'); time.textContent = date(message.receivedAt);
        button.append(from, title, excerpt, time); button.addEventListener('click', () => run(() => openMessage(inbox, message))); container.appendChild(button);
      }
      if (!messages.length) empty(container, '等待新邮件', '将邮箱地址填入目标服务，邮件到达后会出现在这里。');
      status(`收件完成 · ${new Date().toLocaleTimeString()}${$('mail-auto').checked ? ' · 每 ' + pollAfter / 1000 + ' 秒自动检查' : ' · 自动收件已暂停'}`); schedule();
    } catch (e) {
      if (selected === inbox && !current.signal.aborted && e.name !== 'AbortError') {
        $('mail-auto').checked = false;
        if (e.status === 410) { inbox.state = 'expired'; showIdentity(); renderInboxes(); $('mail-code-panel').hidden = true; }
        throw e;
      }
    } finally { if (polling === current) polling = null; }
  }
  function setMode(next) {
    mode = next; const attach = mode === 'attach';
    $('mail-mode-create').setAttribute('aria-pressed', String(!attach)); $('mail-mode-attach').setAttribute('aria-pressed', String(attach));
    $('mail-create-fields').hidden = attach; $('mail-attach-fields').hidden = !attach; $('mail-attach-hint').hidden = !attach;
    for (const name of ['provider', 'domain']) field(name).disabled = attach;
    for (const name of ['address', 'password', 'imapHost']) field(name).disabled = !attach;
    field('address').required = attach; $('mail-dispatch').hidden = attach; $('mail-dispatch-result').hidden = true;
    $('mail-submit').textContent = attach ? '接入邮箱 →' : '创建邮箱 →';
    $('mail-mode-hint').textContent = attach ? '仅收件，不删除或标记你的私人邮件。授权码保存在本机。' : '自动调度按顺序尝试渠道，创建后开始收件。';
  }
  async function connect() {
    if (connecting || ready) return;
    if (!local) { status('邮箱功能请在本机管理服务的 /preview/ 中使用。'); return; }
    connecting = true;
    try {
      const data = await api('/status'); csrf = data.csrf;
      const attach = data.attachDomains || [];
      $('mail-attach-hint').textContent = (attach.length ? '可直接接入（无需密码）：' + attach.map(d => 'name@' + d).join('、') + '。' : '') + '私人邮箱请使用邮箱授权码 / 应用专用密码。部分 Outlook 账号需要 OAuth，无法使用密码接入。';
      providers = (await api('/providers')).providers || [];
      field('provider').replaceChildren(new Option('自动调度 · 失败自动切换', ''));
      for (const p of providers) if (p.enabled && p.features?.pollInbox) field('provider').add(new Option(p.displayName || p.name, p.name));
      await list(); ready = true; controls.disabled = false; $('mail-reload').disabled = false;
      const saved = inboxes.find(i => i.id === recalled());
      const initial = saved || inboxes.find(i => state(i) === 'active');
      status('邮箱已就绪，创建临时邮箱或接入已有邮箱。');
      if (initial && !selected) { $('mail-composer').open = false; await select(initial); }
    } catch (e) {
      controls.disabled = false; $('mail-reload').disabled = false;
      throw new Error(`邮箱连接失败，请点击「刷新信箱」重试。${e.message ? ` 原因：${e.message}` : ''}`, {cause: e});
    } finally { connecting = false; }
  }
  function switchView() {
    const mail = location.hash === '#mail';
    $('sms-view').hidden = mail; $('mail-view').hidden = !mail;
    $('sms-tab').setAttribute('aria-pressed', String(!mail)); $('mail-tab').setAttribute('aria-pressed', String(mail)); stop();
    if (mail) run(async () => { const wasReady = ready; await connect(); if (wasReady && selected) await refresh(); });
  }
  async function copy(value, message) { await navigator.clipboard.writeText(value); status(message); }
  $('sms-tab').addEventListener('click', () => { location.hash = 'sms'; });
  $('mail-tab').addEventListener('click', () => { if (location.hash === '#mail') switchView(); else location.hash = 'mail'; });
  window.addEventListener('hashchange', switchView);
  form.addEventListener('submit', event => {
    event.preventDefault(); if (controls.disabled) return;
    if (!ready) { run(connect, $('mail-submit')); return; }
    controls.disabled = true; status(mode === 'attach' ? '正在验证并接入邮箱…' : '正在创建邮箱，自动调度可能需要一些时间…');
    run(async () => {
      try {
        const body = {for: field('for').value.trim()};
        const names = mode === 'attach' ? ['address', 'password', 'imapHost'] : ['provider', 'domain'];
        for (const name of names) if (field(name).value.trim()) body[name] = field(name).value.trim();
        const inbox = await api('/inbox', 'POST', body); field('password').value = '';
        $('mail-composer').open = false; $('mail-auto').checked = true; await list(); await select(inbox);
      } finally { controls.disabled = false; }
    });
  });
  $('mail-mode-create').addEventListener('click', () => setMode('create')); $('mail-mode-attach').addEventListener('click', () => setMode('attach'));
  $('mail-dispatch').addEventListener('click', () => run(async () => {
    const data = await api('/dispatch/preview?for=' + encodeURIComponent(field('for').value.trim()));
    $('mail-dispatch-result').textContent = '配置顺序（不代表上游实时可用性）\n' + (data.candidates || []).map((c, i) => `${i + 1}. ${c.displayName || c.provider} · ${c.eligible ? '参与调度' : c.reason}`).join('\n');
    $('mail-dispatch-result').hidden = false;
  }, $('mail-dispatch')));
  $('mail-reload').addEventListener('click', () => run(async () => { if (!ready) { await connect(); return; } await list(); if (selected) await select(selected); }, $('mail-reload')));
  for (const id of ['mail-group', 'mail-filter']) $(id).addEventListener('change', renderInboxes);
  $('mail-search').addEventListener('input', renderInboxes);
  $('mail-refresh').addEventListener('click', () => run(refresh));
  $('mail-auto').addEventListener('change', () => { clearTimeout(timer); if ($('mail-auto').checked) run(refresh); else status('自动收件已暂停，可点击「立即收件」。'); });
  $('mail-copy').addEventListener('click', () => run(async () => { if (selected) await copy(selected.address, '邮箱地址已复制。'); }));
  $('mail-copy-code').addEventListener('click', () => run(async () => { if (codeValue) await copy(codeValue, '验证码已复制，请核对邮件来源。'); }));
  $('mail-close').addEventListener('click', () => run(async () => {
    if (!selected || state(selected) === 'closed') return;
    const inbox = selected;
    if (!window.confirm(inbox.provider === 'imap' ? '断开该邮箱？本机将清除授权码，你的原邮箱邮件会保留。' : '关闭此临时邮箱？邮箱将停止收件，渠道资源可能被删除，无法恢复。')) return;
    stop();
    try { await api(inboxPath(inbox), 'DELETE'); if (selected === inbox) { clearInbox(); remember(''); } await list(); status('邮箱已关闭，本机凭据已清除。'); }
    finally { schedule(); }
  }));
  document.addEventListener('visibilitychange', () => { stop(); if (active() && ready) run(refresh); });
  window.addEventListener('pagehide', stop);
  setMode('create'); switchView();
})();
