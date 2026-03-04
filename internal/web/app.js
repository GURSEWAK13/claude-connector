// app.js — claude-connector web dashboard
// D3.js v7 force-directed graph + WebSocket real-time updates

const WS_URL = `ws://${location.host}/ws`;
const API_BASE = `${location.origin}/api`;

// State
let state = {
  sessions: [],
  peers: [],
  fallbacks: [],
  requests: [],
  nodeName: document.getElementById('node-name')?.textContent || 'unknown',
};

// ─── WebSocket ────────────────────────────────────────────────────────────────

let ws;
function connectWS() {
  ws = new WebSocket(WS_URL);

  ws.onopen = () => {
    console.log('[ws] connected');
    document.getElementById('conn-status').textContent = 'connected';
    document.getElementById('conn-status').className = 'status-pill pill-green';
  };

  ws.onmessage = (e) => {
    try {
      const evt = JSON.parse(e.data);
      handleEvent(evt);
    } catch (err) {
      console.error('[ws] parse error', err);
    }
  };

  ws.onclose = () => {
    document.getElementById('conn-status').textContent = 'disconnected';
    document.getElementById('conn-status').className = 'status-pill pill-red';
    setTimeout(connectWS, 2000); // reconnect
  };

  ws.onerror = (e) => console.error('[ws] error', e);
}

function handleEvent(evt) {
  switch (evt.type) {
    case 'session_update':
      if (Array.isArray(evt.payload)) {
        state.sessions = evt.payload;
      } else {
        upsert(state.sessions, evt.payload, 'id');
      }
      renderSessions();
      break;

    case 'peer_update':
      if (Array.isArray(evt.payload)) {
        state.peers = evt.payload;
      } else {
        upsert(state.peers, evt.payload, 'id');
      }
      renderGraph();
      renderPeers();
      break;

    case 'request_log':
      state.requests.unshift(evt.payload);
      if (state.requests.length > 200) state.requests.length = 200;
      renderLog();
      break;

    case 'metrics_update':
      if (evt.payload.sessions) state.sessions = evt.payload.sessions;
      if (evt.payload.peers) state.peers = evt.payload.peers;
      if (evt.payload.fallbacks) state.fallbacks = evt.payload.fallbacks;
      renderSessions();
      renderGraph();
      renderPeers();
      renderFallbacks();
      break;
  }
}

function upsert(arr, item, key) {
  const idx = arr.findIndex(x => x[key] === item[key]);
  if (idx >= 0) arr[idx] = item;
  else arr.push(item);
}

// ─── Initial Load ─────────────────────────────────────────────────────────────

async function loadInitialState() {
  try {
    const r = await fetch(`${API_BASE}/status`);
    const data = await r.json();
    state.sessions = data.sessions || [];
    state.peers = data.peers || [];
    state.fallbacks = data.fallbacks || [];
    document.getElementById('node-name').textContent = data.node_name || '';
    document.getElementById('proxy-port').textContent = ':' + data.proxy_port;
    renderSessions();
    renderGraph();
    renderPeers();
    renderFallbacks();
  } catch (e) {
    console.error('initial load failed', e);
  }
}

// ─── Sessions Panel ───────────────────────────────────────────────────────────

function renderSessions() {
  const el = document.getElementById('sessions-list');
  if (!el) return;

  el.innerHTML = (state.sessions || []).map(s => {
    const dotClass = stateToColor(s.State);
    const stateLabel = stateLabel_(s);
    return `<div class="session-item">
      <div class="session-dot ${dotClass}"></div>
      <div class="session-name">${esc(s.ID)}</div>
      <div class="session-state ${s.State === 2 || s.State === 3 ? 'session-cooldown' : ''}">${stateLabel}</div>
    </div>`;
  }).join('');
}

function stateToColor(state) {
  switch (state) {
    case 0: return 'dot-green';   // AVAILABLE
    case 1: return 'dot-yellow';  // IN_USE
    case 2: return 'dot-red';     // RATE_LIMITED
    case 3: return 'dot-yellow';  // COOLING_DOWN
    default: return 'dot-gray';
  }
}

function stateLabel_(s) {
  switch (s.State) {
    case 0: return 'AVAILABLE';
    case 1: return 'IN USE';
    case 2: return 'RATE LIMITED';
    case 3:
      const rem = Math.max(0, Math.round((s.CooldownRemaining || 0) / 1e9));
      return `COOLING ${rem}s`;
    default: return 'UNKNOWN';
  }
}

// ─── Peers Panel ─────────────────────────────────────────────────────────────

function renderPeers() {
  const el = document.getElementById('peers-list');
  if (!el) return;
  el.innerHTML = (state.peers || []).map(p => `
    <div class="peer-item">
      <div class="session-dot ${p.available ? 'dot-green' : 'dot-gray'}"></div>
      <div class="peer-name">${esc(p.name)}</div>
      <div class="peer-sessions">${p.available_sessions} avail</div>
    </div>
  `).join('') || '<div style="color:#8b949e; font-size:12px;">No peers discovered</div>';
}

// ─── Fallbacks ────────────────────────────────────────────────────────────────

function renderFallbacks() {
  const el = document.getElementById('fallbacks-list');
  if (!el) return;
  el.innerHTML = (state.fallbacks || []).map(f => `
    <div class="fallback-item">
      <div class="session-dot ${f.available ? 'dot-green' : 'dot-gray'}"></div>
      <div>${esc(f.name)}</div>
      <div style="color:${f.available ? 'var(--green)' : 'var(--red)'}; font-size: 11px;">
        ${f.available ? '✓ online' : '✗ offline'}
      </div>
    </div>
  `).join('') || '<div style="color:#8b949e;">None configured</div>';
}

// ─── Request Log ──────────────────────────────────────────────────────────────

function renderLog() {
  const el = document.getElementById('log-entries');
  if (!el) return;

  const MAX = 50;
  const visible = state.requests.slice(0, MAX);
  el.innerHTML = visible.map(r => {
    const viaClass = r.via === 'local' ? 'log-via-local'
      : r.via?.startsWith('peer:') ? 'log-via-peer'
      : (r.via === 'ollama' || r.via === 'lmstudio') ? 'log-via-fallback'
      : 'log-via-error';
    const statusClass = r.status < 400 ? 'log-status-ok' : 'log-status-err';
    const time = r.time ? new Date(r.time).toLocaleTimeString() : '';
    return `<div class="log-entry">
      <span class="log-time">${time}</span>
      <span class="${viaClass}">→ ${esc(r.via || '?')}</span>
      <span class="${statusClass}">${r.status || '?'}</span>
      <span class="log-duration">${r.duration || ''}</span>
      <span class="log-model">${esc(r.model || '')}</span>
    </div>`;
  }).join('');
}

// ─── D3.js Force Graph ────────────────────────────────────────────────────────

let simulation, svg, linkGroup, nodeGroup;

function initGraph() {
  const container = document.getElementById('graph-canvas');
  if (!container || typeof d3 === 'undefined') return;

  svg = d3.select(container)
    .append('svg')
    .attr('width', '100%')
    .attr('height', '100%');

  const defs = svg.append('defs');
  defs.append('marker')
    .attr('id', 'arrowhead')
    .attr('viewBox', '0 -5 10 10')
    .attr('refX', 20)
    .attr('refY', 0)
    .attr('markerWidth', 6)
    .attr('markerHeight', 6)
    .attr('orient', 'auto')
    .append('path')
    .attr('d', 'M0,-5L10,0L0,5')
    .attr('fill', '#444');

  linkGroup = svg.append('g').attr('class', 'links');
  nodeGroup = svg.append('g').attr('class', 'nodes');

  const w = container.clientWidth || 600;
  const h = container.clientHeight || 400;

  simulation = d3.forceSimulation()
    .force('link', d3.forceLink().id(d => d.id).distance(120))
    .force('charge', d3.forceManyBody().strength(-300))
    .force('center', d3.forceCenter(w / 2, h / 2))
    .force('collision', d3.forceCollide(40));

  renderGraph();
}

function renderGraph() {
  if (!svg || typeof d3 === 'undefined') return;

  const container = document.getElementById('graph-canvas');
  const w = container?.clientWidth || 600;
  const h = container?.clientHeight || 400;

  // Build nodes: self + peers
  const selfName = document.getElementById('node-name')?.textContent || 'self';
  const selfNode = { id: 'self', name: selfName, isSelf: true, available_sessions: 1, available: true };
  const peerNodes = (state.peers || []).map(p => ({
    id: p.id || p.name,
    name: p.name,
    available_sessions: p.available_sessions || 0,
    available: p.available || false,
  }));

  const nodes = [selfNode, ...peerNodes];
  const links = peerNodes.map(p => ({ source: 'self', target: p.id }));

  // Update simulation
  simulation.nodes(nodes).on('tick', ticked);
  simulation.force('link').links(links);
  simulation.force('center').x(w / 2).y(h / 2);
  simulation.alpha(0.3).restart();

  // Links
  const link = linkGroup.selectAll('line').data(links, d => d.source + '-' + d.target);
  link.enter().append('line')
    .attr('stroke', '#30363d')
    .attr('stroke-width', 2)
    .attr('marker-end', 'url(#arrowhead)')
    .merge(link);
  link.exit().remove();

  // Nodes
  const node = nodeGroup.selectAll('g.node').data(nodes, d => d.id);

  const nodeEnter = node.enter().append('g').attr('class', 'node')
    .call(d3.drag()
      .on('start', dragstarted)
      .on('drag', dragged)
      .on('end', dragended));

  nodeEnter.append('circle')
    .attr('r', 24)
    .attr('stroke', '#30363d')
    .attr('stroke-width', 2);

  nodeEnter.append('text')
    .attr('text-anchor', 'middle')
    .attr('dy', '0.35em')
    .attr('font-size', '10px')
    .attr('fill', '#c9d1d9')
    .attr('pointer-events', 'none');

  nodeEnter.append('text')
    .attr('class', 'sessions-label')
    .attr('text-anchor', 'middle')
    .attr('dy', '2.2em')
    .attr('font-size', '9px')
    .attr('fill', '#8b949e')
    .attr('pointer-events', 'none');

  const nodeAll = nodeEnter.merge(node);

  nodeAll.select('circle')
    .attr('fill', d => d.isSelf ? '#1f6feb'
      : d.available && d.available_sessions > 0 ? '#1a4731'
      : d.available_sessions === 0 ? '#161b22' : '#3b2300');

  nodeAll.select('text:not(.sessions-label)')
    .text(d => truncate(d.name, 8));

  nodeAll.select('.sessions-label')
    .text(d => d.isSelf ? 'self' : `${d.available_sessions}av`);

  node.exit().remove();

  function ticked() {
    linkGroup.selectAll('line')
      .attr('x1', d => d.source.x)
      .attr('y1', d => d.source.y)
      .attr('x2', d => d.target.x)
      .attr('y2', d => d.target.y);

    nodeGroup.selectAll('g.node')
      .attr('transform', d => `translate(${d.x},${d.y})`);
  }
}

function dragstarted(event, d) {
  if (!event.active) simulation.alphaTarget(0.3).restart();
  d.fx = d.x; d.fy = d.y;
}
function dragged(event, d) { d.fx = event.x; d.fy = event.y; }
function dragended(event, d) {
  if (!event.active) simulation.alphaTarget(0);
  d.fx = null; d.fy = null;
}

// ─── Pulse animation when request routes through a peer ─────────────────────

function pulseEdge(peerId) {
  linkGroup.selectAll('line')
    .filter(d => d.target.id === peerId || d.source.id === peerId)
    .classed('edge-active', true);
  setTimeout(() => {
    linkGroup.selectAll('line').classed('edge-active', false);
  }, 2500);
}

// ─── Utils ────────────────────────────────────────────────────────────────────

function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function truncate(s, max) {
  if (!s) return '';
  return s.length > max ? s.slice(0, max - 1) + '…' : s;
}

// ─── Boot ─────────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
  initGraph();
  loadInitialState();
  connectWS();
});
