(function () {
  const $ = (id) => document.getElementById(id);
  const getToken = () => localStorage.getItem('admin_token') || $('token').value.trim();
  const setToken = (v) => localStorage.setItem('admin_token', v);

  $('token').value = getToken() || '';
  $('token').addEventListener('change', (e) => setToken(e.target.value.trim()));

  async function api(path, opts = {}) {
    const token = getToken();
    const headers = Object.assign({ 'Authorization': 'Bearer ' + token }, opts.headers || {});
    const resp = await fetch(path, Object.assign({}, opts, { headers }));
    if (!resp.ok) throw new Error('Request failed: ' + resp.status);
    return resp.json();
  }

  $('create-source').onclick = async () => {
    try {
      const name = $('src-name').value.trim();
      const max = parseInt($('src-max').value || '1048576', 10);
      const cidrs = $('src-cidrs').value.trim().split(',').map(x => x.trim()).filter(Boolean);
      const res = await api('/admin/sources', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, max_body_bytes: max, ip_allow_cidrs: cidrs })
      });
      $('sources-out').textContent = JSON.stringify(res, null, 2);
    } catch (e) { $('sources-out').textContent = e.message; }
  };

  $('list-sources').onclick = async () => {
    try { $('sources-out').textContent = JSON.stringify(await api('/admin/sources'), null, 2); }
    catch (e) { $('sources-out').textContent = e.message; }
  };

  $('create-dst').onclick = async () => {
    try {
      const name = $('dst-name').value.trim();
      const url = $('dst-url').value.trim();
      const headers = JSON.parse($('dst-headers').value || '{}');
      const rps = parseFloat($('dst-rps').value || '5.0');
      const burst = parseInt($('dst-burst').value || '10', 10);
      const inflight = parseInt($('dst-inflight').value || '5', 10);
      const append_path = $('dst-append').value === 'true';
      const res = await api('/admin/destinations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, url, headers, max_rps: rps, burst, max_inflight: inflight, append_path })
      });
      $('dst-out').textContent = JSON.stringify(res, null, 2);
    } catch (e) { $('dst-out').textContent = e.message; }
  };

  $('list-dst').onclick = async () => {
    try { $('dst-out').textContent = JSON.stringify(await api('/admin/destinations'), null, 2); }
    catch (e) { $('dst-out').textContent = e.message; }
  };

  $('create-rt').onclick = async () => {
    try {
      const source_name = $('rt-src').value.trim();
      const destination_name = $('rt-dst').value.trim();
      const content_type_like = $('rt-ct').value.trim() || null;
      const ord = parseInt($('rt-ord').value || '0', 10);
      const res = await api('/admin/routes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ source_name, destination_name, content_type_like, ord })
      });
      $('rt-out').textContent = JSON.stringify(res, null, 2);
    } catch (e) { $('rt-out').textContent = e.message; }
  };

  $('list-rt').onclick = async () => {
    try { $('rt-out').textContent = JSON.stringify(await api('/admin/routes'), null, 2); }
    catch (e) { $('rt-out').textContent = e.message; }
  };

  async function refreshSourcesSelects() {
    try {
      const data = await api('/admin/sources');
      const items = data.items || [];
      const selects = [ $('ti-source'), $('ev-source') ];
      selects.forEach(sel => { sel.innerHTML=''; items.forEach(it => { const opt = document.createElement('option'); opt.value = it.source_id; opt.textContent = `${it.name} (${it.source_id.slice(0,8)})`; sel.appendChild(opt); }); });
    } catch {}
  }

  $('list-sources').addEventListener('click', refreshSourcesSelects);
  refreshSourcesSelects();

  async function getSourceToken(sourceId) {
    const res = await api(`/admin/sources/${sourceId}`);
    return res.token;
  }

  $('ti-send').onclick = async () => {
    const out = $('ti-out'); out.textContent='';
    try {
      const sid = $('ti-source').value; if (!sid) throw new Error('Select a source');
      const tokRes = await api(`/admin/sources/${sid}`);
      const token = tokRes.token;
      const method = $('ti-method').value;
      const path = $('ti-path').value || '/';
      const headers = JSON.parse($('ti-headers').value || '{}');
      const body = $('ti-body').value || '';
      const resp = await fetch(`/ingest/${token}${path}`, { method, headers, body });
      const text = await resp.text();
      out.textContent = `HTTP ${resp.status}\n` + text;
    } catch (e) { out.textContent = e.message; }
  };

  $('ev-load').onclick = async () => {
    const list = $('ev-list'); list.innerHTML='Loading...';
    try {
      const sid = $('ev-source').value; const lim = parseInt($('ev-limit').value||'20',10);
      const res = await api(`/admin/events?source_id=${encodeURIComponent(sid)}&limit=${lim}`);
      const items = res.items || [];
      list.innerHTML = '';
      items.forEach(it => {
        const row = document.createElement('div'); row.style.padding='8px 0'; row.style.borderBottom='1px solid #eee';
        row.innerHTML = `<code>${it.event_id}</code> — ${it.received_at} — ${it.method} ${it.path} (${it.content_type}, ${it.body_size} bytes)
          <button data-id="${it.event_id}">Replay</button>`;
        const btn = row.querySelector('button');
        btn.onclick = async () => {
          btn.disabled = true; btn.textContent='Replaying...';
          try { const r = await fetch(`/events/${it.event_id}/replay`, { method:'POST' }); const t = await r.text(); btn.textContent = r.ok ? 'Replayed' : ('Error ' + r.status); }
          catch { btn.textContent='Error'; }
          finally { btn.disabled=false; }
        };
        list.appendChild(row);
      });
    } catch (e) { list.textContent = e.message; }
  };
})();
