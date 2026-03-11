/* tui-streamer – frontend application
 * Vanilla JS, no build step, no external dependencies.
 */

'use strict';

// ── ANSI escape code parser ────────────────────────────────────────────────

const ANSI_COLOR_NAMES = [
  'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
];

class AnsiParser {
  constructor() { this._reset(); }

  _reset() {
    this.bold = false;
    this.dim  = false;
    this.italic = false;
    this.underline = false;
    this.blink = false;
    this.fg = null;
    this.bg = null;
  }

  /** Convert an ANSI-escaped string to safe HTML. */
  toHtml(text) {
    // Split on SGR escape sequences: ESC [ ... m
    const parts = text.split(/(\x1b\[[0-9;]*m)/);
    let html = '';
    for (const part of parts) {
      if (part.startsWith('\x1b[')) {
        this._applyEscape(part);
      } else if (part) {
        html += this._wrap(part);
      }
    }
    return html;
  }

  _applyEscape(seq) {
    const inner = seq.slice(2, -1); // strip ESC[ and m
    if (!inner || inner === '0') { this._reset(); return; }

    const codes = inner.split(';').map(Number);
    for (let i = 0; i < codes.length; i++) {
      const c = codes[i];
      if      (c === 0)  this._reset();
      else if (c === 1)  this.bold = true;
      else if (c === 2)  this.dim  = true;
      else if (c === 3)  this.italic = true;
      else if (c === 4)  this.underline = true;
      else if (c === 5)  this.blink = true;
      else if (c === 22) this.bold = false;
      else if (c === 23) this.italic = false;
      else if (c === 24) this.underline = false;
      else if (c === 25) this.blink = false;
      else if (c >= 30 && c <= 37) this.fg = `var(--ansi-${ANSI_COLOR_NAMES[c - 30]})`;
      else if (c >= 90 && c <= 97) this.fg = `var(--ansi-bright-${ANSI_COLOR_NAMES[c - 90]})`;
      else if (c >= 40 && c <= 47) this.bg = `var(--ansi-${ANSI_COLOR_NAMES[c - 40]})`;
      else if (c === 39) this.fg = null;
      else if (c === 49) this.bg = null;
      // 256-colour  38;5;n  /  48;5;n
      else if (c === 38 && codes[i + 1] === 5 && i + 2 < codes.length) {
        this.fg = this._color256(codes[i + 2]); i += 2;
      } else if (c === 48 && codes[i + 1] === 5 && i + 2 < codes.length) {
        this.bg = this._color256(codes[i + 2]); i += 2;
      }
      // True-colour  38;2;r;g;b  /  48;2;r;g;b
      else if (c === 38 && codes[i + 1] === 2 && i + 4 < codes.length) {
        this.fg = `rgb(${codes[i+2]},${codes[i+3]},${codes[i+4]})`; i += 4;
      } else if (c === 48 && codes[i + 1] === 2 && i + 4 < codes.length) {
        this.bg = `rgb(${codes[i+2]},${codes[i+3]},${codes[i+4]})`; i += 4;
      }
    }
  }

  _color256(n) {
    if (n < 16) {
      const names = [
        'black','red','green','yellow','blue','magenta','cyan','white',
        'bright-black','bright-red','bright-green','bright-yellow',
        'bright-blue','bright-magenta','bright-cyan','bright-white',
      ];
      return `var(--ansi-${names[n]})`;
    }
    if (n < 232) {
      // 6×6×6 colour cube
      n -= 16;
      const toV = v => v ? v * 40 + 55 : 0;
      return `rgb(${toV(Math.floor(n/36))},${toV(Math.floor(n/6)%6)},${toV(n%6)})`;
    }
    // Grayscale ramp
    const v = (n - 232) * 10 + 8;
    return `rgb(${v},${v},${v})`;
  }

  _wrap(text) {
    const escaped = text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');

    const styles  = [];
    const classes = [];

    if (this.fg) styles.push(`color:${this.fg}`);
    if (this.bg) styles.push(`background:${this.bg}`);
    if (this.bold)      classes.push('ansi-bold');
    if (this.dim)       classes.push('ansi-dim');
    if (this.italic)    classes.push('ansi-italic');
    if (this.underline) classes.push('ansi-underline');
    if (this.blink)     classes.push('ansi-blink');

    if (!styles.length && !classes.length) return escaped;

    const sa = styles.length  ? ` style="${styles.join(';')}"` : '';
    const ca = classes.length ? ` class="${classes.join(' ')}"` : '';
    return `<span${ca}${sa}>${escaped}</span>`;
  }
}

// ── REST API helpers ────────────────────────────────────────────────────────

const api = {
  async listSessions() {
    const r = await fetch('/api/sessions');
    return r.json();
  },
  async createSession(name) {
    const r = await fetch('/api/sessions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    return r.json();
  },
  async deleteSession(id) {
    return fetch(`/api/sessions/${id}`, { method: 'DELETE' });
  },
  async execCommand(sessionId, command, opts = {}) {
    const r = await fetch(`/api/sessions/${sessionId}/exec`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command, ...opts }),
    });
    if (!r.ok) {
      const body = await r.json().catch(() => ({ error: r.statusText }));
      throw new Error(body.error || r.statusText);
    }
    return r.json();
  },
  async killSession(id) {
    return fetch(`/api/sessions/${id}/kill`, { method: 'POST' });
  },
};

// ── WebSocket manager ───────────────────────────────────────────────────────

class SessionSocket {
  constructor(sessionId, onMessage, onStatusChange) {
    this.sessionId     = sessionId;
    this.onMessage     = onMessage;
    this.onStatusChange = onStatusChange;
    this._ws           = null;
    this._reconnectTimer = null;
    this._closed       = false;
    this.connect();
  }

  connect() {
    if (this._closed) return;
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${location.host}/ws/${this.sessionId}`;
    this.onStatusChange('connecting');

    const ws = new WebSocket(url);
    this._ws = ws;

    ws.addEventListener('open', () => {
      this.onStatusChange('connected');
    });

    ws.addEventListener('message', (e) => {
      try {
        const msg = JSON.parse(e.data);
        this.onMessage(msg);
      } catch { /* ignore malformed frames */ }
    });

    ws.addEventListener('close', () => {
      this.onStatusChange('disconnected');
      if (!this._closed) {
        this._reconnectTimer = setTimeout(() => this.connect(), 2000);
      }
    });

    ws.addEventListener('error', () => {
      ws.close();
    });
  }

  destroy() {
    this._closed = true;
    clearTimeout(this._reconnectTimer);
    if (this._ws) this._ws.close();
  }
}

// ── Terminal renderer ───────────────────────────────────────────────────────

class Terminal {
  constructor(container) {
    this.container  = container;
    this.parser     = new AnsiParser();
    this.autoScroll = true;
  }

  clear() {
    this.container.innerHTML = '';
    this.parser._reset();
  }

  /** Append a parsed server message as a formatted terminal line. */
  append(msg) {
    const el = this._buildLine(msg);
    if (!el) return;
    this.container.appendChild(el);
    if (this.autoScroll) {
      el.scrollIntoView({ block: 'end' });
    }
  }

  _buildLine(msg) {
    const ts = this._fmt(msg.timestamp);

    switch (msg.type) {
      case 'start': {
        const el = document.createElement('div');
        el.className = 'line-event start';
        el.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
          <path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0zm3.5 7.5l-5-3A.5.5 0 0 0 6 5v6a.5.5 0 0 0 .5.5.5.5 0 0 0 .25-.066l5-3a.5.5 0 0 0 0-.866z"/>
        </svg><span>Process started</span><span style="margin-left:auto;opacity:.5;font-size:10px">${ts}</span>`;
        return el;
      }

      case 'exit': {
        const ok  = msg.exit_code === 0;
        const el  = document.createElement('div');
        el.className = `line-event ${ok ? 'exit-ok' : 'exit-err'}`;
        const icon = ok
          ? `<svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.75.75 0 0 1 1.06-1.06L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0z"/></svg>`
          : `<svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor"><path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06z"/></svg>`;
        el.innerHTML = `${icon}<span>Process exited (${msg.exit_code})</span><span style="margin-left:auto;opacity:.5;font-size:10px">${ts}</span>`;
        return el;
      }

      case 'error': {
        const el = document.createElement('div');
        el.className = 'line-event error';
        el.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor"><path d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zM0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8zm9-.25v.317c0 .417-.293.811-.716.956C7.38 9.214 7 9.614 7 10.25v.75a.75.75 0 0 0 1.5 0v-.316c.721-.166 1.5-.747 1.5-1.934v-.317a.75.75 0 0 0-1.5 0zM9 12.5a1 1 0 1 1-2 0 1 1 0 0 1 2 0z"/></svg>
          <span>${this._escHtml(msg.data || '')}</span>`;
        return el;
      }

      case 'stdout':
      case 'stderr': {
        const line = document.createElement('div');
        line.className = `terminal-line ${msg.type}`;
        const tsEl = document.createElement('span');
        tsEl.className = 'line-ts';
        tsEl.textContent = ts;
        const content = document.createElement('span');
        content.className = 'line-content';
        content.innerHTML = this.parser.toHtml(msg.data || '');
        line.appendChild(tsEl);
        line.appendChild(content);
        return line;
      }

      default:
        return null;
    }
  }

  _fmt(ms) {
    if (!ms) return '';
    const d = new Date(ms);
    return d.toLocaleTimeString('en-GB', { hour12: false });
  }

  _escHtml(s) {
    return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }
}

// ── Application ─────────────────────────────────────────────────────────────

class App {
  constructor() {
    // State
    this.sessions    = [];   // Info[] from API
    this.activeId    = null;
    this.sockets     = {};   // sessionId → SessionSocket
    this.statuses    = {};   // sessionId → 'connecting'|'connected'|'disconnected'
    this.buffers     = {};   // sessionId → Line[]

    // DOM refs
    this.$sessionList = document.getElementById('session-list');
    this.$newBtn      = document.getElementById('btn-new-session');
    this.$overlay     = document.getElementById('modal-overlay');
    this.$newForm     = document.getElementById('form-new-session');
    this.$nameInput   = document.getElementById('input-session-name');
    this.$termPanel   = document.getElementById('terminal-panel');
    this.$emptyState  = document.getElementById('empty-state');
    this.$termTitle   = document.getElementById('term-title');
    this.$termSubtitle= document.getElementById('term-subtitle');
    this.$connDot     = document.getElementById('conn-dot');
    this.$connLabel   = document.getElementById('conn-label');
    this.$termOutput  = document.getElementById('terminal-output');
    this.$cmdInput    = document.getElementById('cmd-input');
    this.$runBtn      = document.getElementById('btn-run');
    this.$clearBtn    = document.getElementById('btn-clear');
    this.$scrollBtn   = document.getElementById('btn-scroll');
    this.$killBtn     = document.getElementById('btn-kill');
    this.$themeSelect = document.getElementById('theme-select');

    this.terminal = new Terminal(this.$termOutput);

    this._bindEvents();
    this._loadTheme();
    this._refresh();
  }

  _bindEvents() {
    // New session modal
    this.$newBtn.addEventListener('click', () => this._showModal());
    this.$overlay.addEventListener('click', (e) => {
      if (e.target === this.$overlay) this._hideModal();
    });
    document.getElementById('btn-cancel-new').addEventListener('click', () => this._hideModal());
    this.$newForm.addEventListener('submit', (e) => {
      e.preventDefault();
      this._createSession();
    });

    // Run command
    this.$runBtn.addEventListener('click', () => this._runCommand());
    this.$cmdInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this._runCommand(); }
    });

    // Clear / scroll / kill
    this.$clearBtn.addEventListener('click', () => this.terminal.clear());
    this.$scrollBtn.addEventListener('click', () => this._toggleScroll());
    this.$killBtn.addEventListener('click',   () => this._killActive());

    // Theme
    this.$themeSelect.addEventListener('change', () => {
      const t = this.$themeSelect.value;
      document.documentElement.setAttribute('data-theme', t === 'dark' ? '' : t);
      localStorage.setItem('tui-theme', t);
    });
  }

  _loadTheme() {
    const t = localStorage.getItem('tui-theme') || 'dark';
    this.$themeSelect.value = t;
    document.documentElement.setAttribute('data-theme', t === 'dark' ? '' : t);
  }

  async _refresh() {
    try {
      this.sessions = await api.listSessions();
      this.sessions.sort((a, b) => new Date(a.created_at) - new Date(b.created_at));
    } catch { this.sessions = []; }
    this._renderSidebar();
    // Subscribe to any sessions we don't have sockets for yet
    for (const s of this.sessions) {
      if (!this.sockets[s.id]) this._subscribe(s.id);
    }
    setTimeout(() => this._refresh(), 4000);
  }

  _subscribe(sessionId) {
    this.buffers[sessionId]  = this.buffers[sessionId] || [];
    this.statuses[sessionId] = 'connecting';

    this.sockets[sessionId] = new SessionSocket(
      sessionId,
      (msg) => {
        this.buffers[sessionId].push(msg);
        if (this.activeId === sessionId) {
          this.terminal.append(msg);
        }
        // Update running state in sidebar
        if (msg.type === 'start' || msg.type === 'exit') {
          this._refreshSession(sessionId, msg.type === 'start');
        }
      },
      (status) => {
        this.statuses[sessionId] = status;
        if (this.activeId === sessionId) this._updateConnBadge(status);
        this._renderSidebar();
      }
    );
  }

  _refreshSession(sessionId, running) {
    const s = this.sessions.find(s => s.id === sessionId);
    if (s) s.running = running;
    this._renderSidebar();
  }

  _renderSidebar() {
    const prev = this.$sessionList.scrollTop;
    this.$sessionList.innerHTML = '';
    for (const s of this.sessions) {
      this.$sessionList.appendChild(this._buildSessionItem(s));
    }
    this.$sessionList.scrollTop = prev;
  }

  _buildSessionItem(s) {
    const status = this.statuses[s.id] || 'disconnected';
    const item = document.createElement('div');
    item.className = `session-item${s.id === this.activeId ? ' active' : ''}`;
    item.dataset.id = s.id;

    const dotClass = s.running ? 'running' : status === 'connected' ? 'connected' : '';
    // Show a "ready" pill for bundle sessions that have a pending command and
    // have never been run (no buffered output yet).
    const hasPending = s.pending_command && !s.running &&
                       !(this.buffers[s.id] && this.buffers[s.id].length > 0);
    item.innerHTML = `
      <span class="session-dot ${dotClass}"></span>
      <span class="session-name" title="${this._esc(s.id)}">${this._esc(s.name)}</span>
      ${hasPending ? `<span class="session-badge session-badge-ready" title="Pre-configured command ready to run">&#9656;</span>` : (s.client_count > 0 ? `<span class="session-badge">${s.client_count}</span>` : '')}
      <span class="session-actions">
        <button class="btn btn-icon btn-danger" data-action="delete" title="Delete session">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
            <path d="M5.5 5.5A.5.5 0 0 1 6 6v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5m2.5 0a.5.5 0 0 1 .5.5v6a.5.5 0 0 1-1 0V6a.5.5 0 0 1 .5-.5m3 .5a.5.5 0 0 0-1 0v6a.5.5 0 0 0 1 0z"/>
            <path d="M14.5 3a1 1 0 0 1-1 1H13v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V4h-.5a1 1 0 0 1-1-1V2a1 1 0 0 1 1-1H6a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1h3.5a1 1 0 0 1 1 1zM4.118 4 4 4.059V13a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V4.059L11.882 4zM2.5 3h11V2h-11z"/>
          </svg>
        </button>
      </span>
    `;

    item.addEventListener('click', (e) => {
      const action = e.target.closest('[data-action]')?.dataset.action;
      if (action === 'delete') {
        e.stopPropagation();
        this._deleteSession(s.id);
      } else {
        this._selectSession(s.id);
      }
    });

    return item;
  }

  _selectSession(id) {
    if (this.activeId === id) return;
    this.activeId = id;
    const s = this.sessions.find(s => s.id === id);
    if (!s) return;

    this.$emptyState.classList.add('hidden');
    this.$termPanel.classList.remove('hidden');
    this.$termTitle.textContent   = s.name;
    this.$termSubtitle.textContent = id.substring(0, 8) + '…';

    // Replay buffered output
    this.terminal.clear();
    for (const msg of (this.buffers[id] || [])) {
      this.terminal.append(msg);
    }

    // Pre-populate command input from bundle pending_command if the input is empty
    // or if it still holds the previous session's pending command.
    this.$cmdInput.value = s.pending_command || '';

    this._updateConnBadge(this.statuses[id] || 'disconnected');
    this._renderSidebar();
    this.$cmdInput.focus();
  }

  _updateConnBadge(status) {
    this.$connDot.className  = `conn-dot ${status}`;
    this.$connLabel.textContent = status;
  }

  async _createSession() {
    const name = this.$nameInput.value.trim() || 'session';
    this._hideModal();
    this.$nameInput.value = '';
    try {
      const s = await api.createSession(name);
      this.sessions.push(s);
      this._subscribe(s.id);
      this._renderSidebar();
      this._selectSession(s.id);
    } catch (e) {
      alert('Failed to create session: ' + e.message);
    }
  }

  async _deleteSession(id) {
    if (!confirm('Delete this session?')) return;
    await api.deleteSession(id);
    if (this.sockets[id]) { this.sockets[id].destroy(); delete this.sockets[id]; }
    delete this.buffers[id];
    delete this.statuses[id];
    this.sessions = this.sessions.filter(s => s.id !== id);
    if (this.activeId === id) {
      this.activeId = null;
      this.$emptyState.classList.remove('hidden');
      this.$termPanel.classList.add('hidden');
    }
    this._renderSidebar();
  }

  async _runCommand() {
    if (!this.activeId) return;
    const raw = this.$cmdInput.value.trim();
    if (!raw) return;

    // Shell-split the command (basic: split on spaces, respect quotes)
    const command = this._shellSplit(raw);
    if (!command.length) return;

    try {
      await api.execCommand(this.activeId, command);
      this.$cmdInput.value = '';
    } catch (e) {
      // Show error inline
      const errMsg = { type: 'error', data: e.message, timestamp: Date.now() };
      this.terminal.append(errMsg);
    }
  }

  async _killActive() {
    if (!this.activeId) return;
    await api.killSession(this.activeId);
  }

  _toggleScroll() {
    this.terminal.autoScroll = !this.terminal.autoScroll;
    this.$scrollBtn.classList.toggle('scroll-active', this.terminal.autoScroll);
    if (this.terminal.autoScroll) {
      this.$termOutput.lastElementChild?.scrollIntoView({ block: 'end' });
    }
  }

  _showModal() {
    this.$overlay.classList.remove('hidden');
    this.$nameInput.focus();
  }

  _hideModal() {
    this.$overlay.classList.add('hidden');
  }

  _shellSplit(cmd) {
    const tokens = [];
    let cur = '';
    let inSingle = false, inDouble = false;
    for (const ch of cmd) {
      if (ch === "'" && !inDouble) { inSingle = !inSingle; }
      else if (ch === '"' && !inSingle) { inDouble = !inDouble; }
      else if (ch === ' ' && !inSingle && !inDouble) {
        if (cur) { tokens.push(cur); cur = ''; }
      } else {
        cur += ch;
      }
    }
    if (cur) tokens.push(cur);
    return tokens;
  }

  _esc(s) {
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
                    .replace(/"/g,'&quot;');
  }
}

// ── Bootstrap ────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
  window.__app = new App();
});
