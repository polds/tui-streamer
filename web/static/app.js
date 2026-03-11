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
  async importBundle(file) {
    const text = await file.text();
    const r = await fetch('/api/bundles', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: text,
    });
    if (!r.ok) {
      const body = await r.json().catch(() => ({ error: r.statusText }));
      throw new Error(body.error || r.statusText);
    }
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
        this._reconnectTimer = setTimeout(() => this.connect(), 500);
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
        </svg><span>Process started</span>
        <div style="margin-left:auto;display:flex;align-items:center;gap:8px;">
          <button class="btn btn-icon" data-action="copy-all" title="Copy output" style="padding:2px 4px;">
            <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
              <path d="M4 1.5H3a2 2 0 0 0-2 2V14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V3.5a2 2 0 0 0-2-2h-1c0-1-1-1.5-2-1.5H6c-1 0-2 .5-2 1.5zM5.5 2c0-.5.5-.5 1-.5h3c.5 0 1 0 1 .5V3h-5V2zM3 3.5h1V4c0 .5.5 1 1 1h6c.5 0 1-.5 1-1V3.5h1a1 1 0 0 1 1 1V14a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V4.5a1 1 0 0 1 1-1z"/>
            </svg>
          </button>
          <span style="opacity:.5;font-size:10px">${ts}</span>
        </div>`;
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
    this.collapsedBundles = new Set(); // bundle names that are collapsed
    this.lastSelectedElement = null;

    // DOM refs
    this.$sessionList = document.getElementById('session-list');
    this.$newBtn      = document.getElementById('btn-new-session');
    this.$importBtn   = document.getElementById('btn-import-bundle');
    this.$bundleInput = document.getElementById('input-bundle-file');
    this.$sidebarFn   = document.getElementById('sidebar-footer');
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
    this.$termHint    = document.getElementById('terminal-hint');
    this.$selPrompt   = document.getElementById('sel-prompt');
    this.$selCount    = document.getElementById('sel-count');
    this.$selCopyBtn  = document.getElementById('btn-sel-copy');
    this.$selRaysoBtn = document.getElementById('btn-sel-rayso');
    this.$selClearBtn = document.getElementById('btn-sel-clear');
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

    // Import bundle
    if (window.STARTUP_BUNDLE) {
      this.$importBtn.style.display = 'none';
      this.$newBtn.style.flex = 'none';
      this.$newBtn.style.width = '100%';
    } else {
      this.$importBtn.addEventListener('click', async () => {
        if (window.openFileDialog) {
          try {
            const content = await window.openFileDialog();
            if (!content) return; // user cancelled
            const file = new File([content], "bundle.json", { type: "application/json" });
            await api.importBundle(file);
            this._refresh();
          } catch (err) {
            this._uiAlert(`Failed to import bundle from dialog:\n\n${err.message}`);
          }
        } else {
          this.$bundleInput.click();
        }
      });
      this.$bundleInput.addEventListener('change', (e) => this._importBundle(e));
    }

    // Run command
    this.$runBtn.addEventListener('click', () => this._runCommand());
    this.$cmdInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this._runCommand(); }
    });

    // Clear / scroll / kill
    this.$clearBtn.addEventListener('click', () => this.terminal.clear());
    this.$scrollBtn.addEventListener('click', () => this._toggleScroll());
    this.$killBtn.addEventListener('click',   () => this._killActive());

    // Hint chip / run-button delegation (innerHTML is replaced on each call,
    // so we delegate on the container itself which persists).
    this.$termHint.addEventListener('click', (e) => {
      const chip = e.target.closest('.hint-chip');
      if (chip) { this._runExampleCmd(chip.dataset.cmd); return; }
      if (e.target.closest('[data-action="run-hint"]')) this._runFromHint();
    });

    // Terminal selection
    this.$termOutput.addEventListener('click', (e) => this._handleTerminalClick(e));
    this.$selCopyBtn.addEventListener('click', () => this._copySelection());
    this.$selRaysoBtn.addEventListener('click', () => this._raysoSelection());
    this.$selClearBtn.addEventListener('click', () => this._clearSelection());

    // Theme
    this.$themeSelect.addEventListener('change', () => {
      const t = this.$themeSelect.value;
      document.documentElement.setAttribute('data-theme', t === 'dark' ? '' : t);
      localStorage.setItem('tui-theme', t);
    });

    // Global keyboard shortcuts
    document.addEventListener('keydown', (e) => {
      if (e.metaKey || e.ctrlKey) {
        if (e.key.toLowerCase() === 'u' && !e.shiftKey && !e.altKey) {
          e.preventDefault();
          this._showModal();
        } else if (e.key.toLowerCase() === 'o' && !e.shiftKey && !e.altKey) {
          if (!window.STARTUP_BUNDLE) {
            e.preventDefault();
            if (window.openFileDialog) {
              this.$importBtn.click(); // Trigger the logic that uses openFileDialog
            } else {
              this.$bundleInput.click();
            }
          }
        } else if (e.key.toLowerCase() === 'c' && e.shiftKey && !e.altKey) {
          if (this.activeId && !this.$termPanel.classList.contains('hidden')) {
            e.preventDefault();
            this._copyAllOutput();
          }
        }
      }
    });
  }

  _loadTheme() {
    const t = localStorage.getItem('tui-theme') || 'catppuccin-macchiato';
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
          // Dismiss the hint the moment the first output line arrives.
          if (!this.$termHint.classList.contains('hidden')) {
            this._updateTerminalHint(sessionId);
          }
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
    
    // Group sessions
    const standalone = [];
    const bundles = {};
    for (const s of this.sessions) {
      if (s.bundle_name) {
        if (!bundles[s.bundle_name]) bundles[s.bundle_name] = [];
        bundles[s.bundle_name].push(s);
      } else {
        standalone.push(s);
      }
    }

    // Render standalone sessions
    for (const s of standalone) {
      this.$sessionList.appendChild(this._buildSessionItem(s));
    }

    // Render bundled sessions
    for (const bName of Object.keys(bundles)) {
      const isExpanded = !this.collapsedBundles.has(bName);
      
      const header = document.createElement('div');
      header.className = 'bundle-header';
      header.innerHTML = `
        <span class="bundle-arrow ${isExpanded ? 'expanded' : ''}">▶</span>
        <span class="bundle-name" title="${this._esc(bName)}">${this._esc(bName)}</span>
      `;
      header.addEventListener('click', () => {
        if (this.collapsedBundles.has(bName)) {
          this.collapsedBundles.delete(bName);
        } else {
          this.collapsedBundles.add(bName);
        }
        this._renderSidebar();
      });
      this.$sessionList.appendChild(header);

      if (isExpanded) {
        const container = document.createElement('div');
        container.className = 'bundle-container';
        for (const s of bundles[bName]) {
          const item = this._buildSessionItem(s);
          item.classList.add('bundled');
          container.appendChild(item);
        }
        this.$sessionList.appendChild(container);
      }
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

    this._clearSelection();

    // Replay buffered output
    this.terminal.clear();
    for (const msg of (this.buffers[id] || [])) {
      this.terminal.append(msg);
    }

    // Pre-populate command input from bundle pending_command.
    this.$cmdInput.value = s.pending_command || '';

    this._updateTerminalHint(id);
    this._updateConnBadge(this.statuses[id] || 'disconnected');
    this._renderSidebar();
    this.$cmdInput.focus();
  }

  // Show a helpful overlay when the terminal has no output yet.
  // Two variants: pending-command (bundle session) and new-session (example chips).
  _updateTerminalHint(id) {
    const s = this.sessions.find(s => s.id === id);
    const hasOutput = this.buffers[id] && this.buffers[id].length > 0;

    if (!s || hasOutput) {
      this.$termHint.classList.add('hidden');
      this.$termOutput.classList.remove('hidden');
      return;
    }

    this.$termOutput.classList.add('hidden');
    this.$termHint.classList.remove('hidden');

    const icon = `<svg class="hint-icon" width="40" height="40" viewBox="0 0 24 24" fill="none"
      stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>
    </svg>`;

    if (s.pending_command) {
      this.$termHint.innerHTML = `
        ${icon}
        <p class="hint-heading">Ready to run</p>
        <p class="hint-sub">This session has a pre-configured command</p>
        <code class="hint-cmd-preview">${this._esc(s.pending_command)}</code>
        <button class="btn btn-success" style="padding:7px 20px;font-size:13px"
                data-action="run-hint">
          <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
            <path d="M11.596 8.697l-6.363 3.692c-.54.313-1.233-.066-1.233-.697V4.308c0-.63.692-1.01 1.233-.696l6.363 3.692a.802.802 0 0 1 0 1.393z"/>
          </svg>
          Run Command
        </button>`;
    } else {
      const examples = [
        'echo "Hello, World!"',
        'ls -la',
        'date',
        'uptime',
        'curl -s ipinfo.io/ip',
      ];
      const chips = examples.map(cmd =>
        `<button class="hint-chip" data-cmd="${this._esc(cmd)}"
                 title="Run: ${this._esc(cmd)}">${this._esc(cmd)}</button>`
      ).join('');
      this.$termHint.innerHTML = `
        ${icon}
        <p class="hint-heading">Nothing running yet</p>
        <p class="hint-sub">Just want to try it out? Run one of these:</p>
        <div class="hint-examples">${chips}</div>`;
    }
  }

  // Called by the "Run Command" button in the pending-command hint.
  _runFromHint() {
    const s = this.sessions.find(s => s.id === this.activeId);
    if (s && s.pending_command) {
      this.$cmdInput.value = s.pending_command;
      this._runCommand();
    }
  }

  // Called when the user clicks an example chip — fills the input and runs immediately.
  _runExampleCmd(cmd) {
    this.$cmdInput.value = cmd;
    this._runCommand();
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
      this._uiAlert('Failed to create session: ' + e.message);
    }
  }

  async _importBundle(e) {
    const file = e.target.files[0];
    if (!file) return;
    
    // Reset input so importing the same file again triggers change event
    e.target.value = '';
    
    try {
      await api.importBundle(file);
      // Let the _refresh loop naturally pull the new sessions, but we can fast-track
      this._refresh();
    } catch (err) {
      this._uiAlert(`Failed to import bundle:\n\n${err.message}`);
    }
  }

  async _deleteSession(id) {
    if (!(await this._uiConfirm('Delete this session?'))) return;
    await api.deleteSession(id);
    if (this.sockets[id]) { this.sockets[id].destroy(); delete this.sockets[id]; }
    delete this.buffers[id];
    delete this.statuses[id];
    this.sessions = this.sessions.filter(s => s.id !== id);
    if (this.activeId === id) {
      this.activeId = null;
      this.$emptyState.classList.remove('hidden');
      this.$termPanel.classList.add('hidden');
      // Reset output/hint visibility for next session selection.
      this.$termOutput.classList.remove('hidden');
      this.$termHint.classList.add('hidden');
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

  _handleTerminalClick(e) {
    if (window.getSelection().toString().trim().length > 0) return;

    const copyBtn = e.target.closest('[data-action="copy-all"]');
    if (copyBtn) {
      this._copyAllOutput(copyBtn);
      return;
    }

    const line = e.target.closest('.terminal-line');
    if (!line) {
      if (e.target === this.$termOutput) this._clearSelection();
      return;
    }

    if (e.metaKey || e.ctrlKey) {
      line.classList.toggle('selected');
      this.lastSelectedElement = line.classList.contains('selected') ? line : null;
    } else if (e.shiftKey && this.lastSelectedElement) {
      const lines = Array.from(this.$termOutput.querySelectorAll('.terminal-line'));
      const idx1 = lines.indexOf(this.lastSelectedElement);
      const idx2 = lines.indexOf(line);
      if (idx1 !== -1 && idx2 !== -1) {
        const start = Math.min(idx1, idx2);
        const end = Math.max(idx1, idx2);
        for (let i = start; i <= end; i++) {
          lines[i].classList.add('selected');
        }
      }
    } else {
      this._clearSelection();
      line.classList.add('selected');
      this.lastSelectedElement = line;
    }

    this._updateSelectionPrompt();
  }

  _clearSelection() {
    if (!this.$termOutput) return;
    const selected = this.$termOutput.querySelectorAll('.terminal-line.selected');
    for (const el of selected) el.classList.remove('selected');
    this.lastSelectedElement = null;
    this._updateSelectionPrompt();
  }

  _updateSelectionPrompt() {
    if (!this.$selPrompt) return;
    const count = this.$termOutput.querySelectorAll('.terminal-line.selected').length;
    if (count > 0) {
      this.$selCount.textContent = `${count} line${count > 1 ? 's' : ''} selected`;
      this.$selPrompt.classList.remove('hidden');
    } else {
      this.$selPrompt.classList.add('hidden');
    }
  }

  _getSelectedText() {
    const selected = Array.from(this.$termOutput.querySelectorAll('.terminal-line.selected'));
    return selected.map(el => {
      const content = el.querySelector('.line-content');
      return content ? content.textContent : '';
    }).join('\n');
  }

  async _copySelection() {
    const text = this._getSelectedText();
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      const orig = this.$selCopyBtn.textContent;
      this.$selCopyBtn.textContent = 'Copied!';
      setTimeout(() => this.$selCopyBtn.textContent = orig, 1500);
    } catch {
      this._uiAlert('Failed to copy to clipboard');
    }
  }

  async _copyAllOutput(btn) {
    const lines = Array.from(this.$termOutput.querySelectorAll('.terminal-line'));
    const text = lines.map(el => {
      const content = el.querySelector('.line-content');
      return content ? content.textContent : '';
    }).join('\n');
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      if (btn) {
        const origHtml = btn.innerHTML;
        btn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="var(--success)"><path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.75.75 0 0 1 1.06-1.06L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0z"/></svg>`;
        setTimeout(() => btn.innerHTML = origHtml, 1500);
      }
    } catch {
      this._uiAlert('Failed to copy to clipboard');
    }
  }

  _raysoSelection() {
    const text = this._getSelectedText();
    if (!text) return;
    
    const title = this.$termTitle.textContent || 'tui-streamer';
    const b64 = btoa(unescape(encodeURIComponent(text)));
    const isDark = document.documentElement.getAttribute('data-theme') !== 'light';
    const url = `https://ray.so/#theme=candy&background=true&darkMode=${isDark}&padding=16&title=${encodeURIComponent(title)}&code=${b64}&language=auto`;
    window.open(url, '_blank');
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

  // Custom UI Dialogs to replace native alert() and confirm() which don't work well in App mode
  _uiAlert(message) {
    return new Promise((resolve) => {
      const overlay = document.createElement('div');
      overlay.className = 'overlay';
      overlay.style.zIndex = '9999';
      
      const modal = document.createElement('div');
      modal.className = 'modal';
      modal.innerHTML = `
        <h3 style="margin-bottom:12px;display:flex;align-items:center;gap:8px;">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="var(--danger)"><path d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zM0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8zm9-.25v.317c0 .417-.293.811-.716.956C7.38 9.214 7 9.614 7 10.25v.75a.75.75 0 0 0 1.5 0v-.316c.721-.166 1.5-.747 1.5-1.934v-.317a.75.75 0 0 0-1.5 0zM9 12.5a1 1 0 1 1-2 0 1 1 0 0 1 2 0z"/></svg>
          Alert
        </h3>
        <p style="margin-bottom:20px;line-height:1.4;white-space:pre-wrap;word-break:break-word;">${this._esc(message)}</p>
        <div class="modal-actions">
          <button class="btn btn-primary" id="ui-alert-ok">OK</button>
        </div>
      `;
      overlay.appendChild(modal);
      document.body.appendChild(overlay);

      const cleanup = () => {
        document.body.removeChild(overlay);
        resolve();
      };

      modal.querySelector('#ui-alert-ok').addEventListener('click', cleanup);
      overlay.addEventListener('click', (e) => {
        if (e.target === overlay) cleanup();
      });
      
      // Auto-focus OK button
      setTimeout(() => modal.querySelector('#ui-alert-ok').focus(), 10);
      
      // Handle Enter/Escape
      const keyHandler = (e) => {
        if (e.key === 'Enter' || e.key === 'Escape') {
          e.preventDefault();
          document.removeEventListener('keydown', keyHandler);
          cleanup();
        }
      };
      document.addEventListener('keydown', keyHandler);
    });
  }

  _uiConfirm(message) {
    return new Promise((resolve) => {
      const overlay = document.createElement('div');
      overlay.className = 'overlay';
      overlay.style.zIndex = '9999';
      
      const modal = document.createElement('div');
      modal.className = 'modal';
      modal.innerHTML = `
        <h3 style="margin-bottom:12px;display:flex;align-items:center;gap:8px;">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="var(--primary)"><path d="M8 15A7 7 0 1 1 8 1a7 7 0 0 1 0 14zm0 1A8 8 0 1 0 8 0a8 8 0 0 0 0 16z"/><path d="M7.002 11a1 1 0 1 1 2 0 1 1 0 0 1-2 0zM7.1 4.995a.905.905 0 1 1 1.8 0l-.35 3.507a.552.552 0 0 1-1.1 0L7.1 4.995z"/></svg>
          Confirm
        </h3>
        <p style="margin-bottom:20px;line-height:1.4;white-space:pre-wrap;word-break:break-word;">${this._esc(message)}</p>
        <div class="modal-actions">
          <button class="btn" id="ui-confirm-cancel">Cancel</button>
          <button class="btn btn-primary" id="ui-confirm-ok">OK</button>
        </div>
      `;
      overlay.appendChild(modal);
      document.body.appendChild(overlay);

      const cleanup = (result) => {
        document.body.removeChild(overlay);
        resolve(result);
      };

      modal.querySelector('#ui-confirm-ok').addEventListener('click', () => cleanup(true));
      modal.querySelector('#ui-confirm-cancel').addEventListener('click', () => cleanup(false));
      overlay.addEventListener('click', (e) => {
        if (e.target === overlay) cleanup(false);
      });
      
      // Auto-focus Cancel button by default
      setTimeout(() => modal.querySelector('#ui-confirm-cancel').focus(), 10);
      
      // Handle Escape
      const keyHandler = (e) => {
        if (e.key === 'Escape') {
          e.preventDefault();
          document.removeEventListener('keydown', keyHandler);
          cleanup(false);
        }
      };
      document.addEventListener('keydown', keyHandler);
    });
  }
}

// ── Bootstrap ────────────────────────────────────────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
  window.__app = new App();
});
