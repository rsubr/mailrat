// ─── Wails bridge ────────────────────────────────────────────────────────────
// Wraps window.go so the app works gracefully when opened in a plain browser.
const Wails = {
  _available() {
    return typeof window !== 'undefined' &&
           typeof window.go !== 'undefined' &&
           window.go.desktop &&
           window.go.desktop.App;
  },
  async call(method, ...args) {
    if (!this._available()) {
      console.warn('[Wails] runtime not available, skipping:', method);
      return null;
    }
    return window.go.desktop.App[method](...args);
  },
  onEvent(name, cb) {
    if (typeof window.runtime !== 'undefined') {
      window.runtime.EventsOn(name, cb);
    }
  }
};

// ─── Helpers ─────────────────────────────────────────────────────────────────
function formatDate(ms) {
  if (!ms) return '';
  const d = new Date(ms);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) {
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  const diff = now - d;
  const days = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];
  if (diff < 7 * 86400000) return days[d.getDay()];
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

function initials(str) {
  if (!str) return '?';
  const parts = str.replace(/<[^>]+>/g, '').trim().split(/\s+/);
  return parts.slice(0, 2).map(p => p[0]).join('').toUpperCase() || '?';
}

function formatReplyDate(ms) {
  if (!ms) return '';
  const d = new Date(ms);
  const dateStr = d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' });
  const timeStr = d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', hour12: true });
  return dateStr + ' at ' + timeStr;
}

function escHtml(s) {
  return (s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

function buildQuote(msg) {
  const date = formatReplyDate(msg.date);
  const attr = date
    ? `On ${date}, ${escHtml(msg.from)} wrote:`
    : `${escHtml(msg.from)} wrote:`;

  let bodyHtml = msg.body || '';
  if (bodyHtml.trim() && !bodyHtml.trim().startsWith('<')) {
    bodyHtml = escHtml(bodyHtml).replace(/\n/g, '<br>');
  }
  if (!bodyHtml.trim()) {
    bodyHtml = escHtml(msg.snippet) || '<i>(no body)</i>';
  }

  return `<div><br></div>` +
    `<div class="gmail-quote">` +
      `<div class="gmail-quote-attr">${attr}</div>` +
      `<blockquote class="gmail-blockquote">${bodyHtml}</blockquote>` +
    `</div>`;
}

function avatarColor(str) {
  const colors = [
    '#5e6ad2','#26b5ce','#e16f3d','#32c27a',
    '#d44f9a','#f0a54e','#7b6cf6','#3ea6a6',
  ];
  let hash = 0;
  for (let i = 0; i < str.length; i++) hash = str.charCodeAt(i) + ((hash << 5) - hash);
  return colors[Math.abs(hash) % colors.length];
}

// System labels always shown in sidebar (order matters)
const SYSTEM_LABELS = [
  { id: 'INBOX',   name: 'Inbox',    icon: 'inbox'    },
  { id: 'STARRED', name: 'Starred',  icon: 'star'     },
  { id: 'SNOOZED', name: 'Snoozed',  icon: 'clock'    },
  { id: 'DRAFT',   name: 'Drafts',   icon: 'doc'      },
  { id: 'SENT',    name: 'Sent',     icon: 'send'     },
  { id: 'SPAM',    name: 'Spam',     icon: 'warning'  },
  { id: 'TRASH',   name: 'Trash',    icon: 'trash'    },
];

// ─── Alpine.js setup ─────────────────────────────────────────────────────────
document.addEventListener('alpine:init', () => {

  // ── Global store ──────────────────────────────────────────────────────────
  Alpine.store('mail', {
    // Accounts
    accounts: [],
    activeAccountId: 'unified',

    // Inbox
    activeLabel: 'INBOX',
    threads: [],
    labels: [],
    loading: false,
    loadingMore: false,
    allLoaded: false,

    // Reading pane
    activeThread: null,
    messages: [],

    // Search
    searchQuery: '',

    // Sync
    syncStatus: 'idle',   // 'idle' | 'syncing' | 'done'
    syncAccount: '',

    // Error banner
    error: null,

    // Compose
    composing: false,
    composeMode: 'new',
    composeData: { accountId: '', to: '', cc: '', bcc: '', subject: '', body: '', inReplyTo: '', threadId: '' },
    showCc: false,
    showBcc: false,
    sending: false,
    sendError: null,

    // Theme
    theme: localStorage.getItem('mailrat-theme') || 'dark',
    toggleTheme() {
      this.theme = this.theme === 'dark' ? 'light' : 'dark';
      localStorage.setItem('mailrat-theme', this.theme);
      document.documentElement.setAttribute('data-theme', this.theme);
    },

    async init() {
      document.documentElement.setAttribute('data-theme', this.theme);
      await this.loadAccounts();
      await Promise.all([this.loadInbox(), this.loadLabels()]);

      Wails.onEvent('inbox:updated',  ()      => { this.loadInbox(); });
      Wails.onEvent('sync:progress',  (data)  => { this.syncStatus = 'syncing'; this.syncAccount = data?.account || ''; });
      Wails.onEvent('sync:complete',  ()      => { this.syncStatus = 'done'; setTimeout(() => { this.syncStatus = 'idle'; }, 2000); });
    },

    async loadAccounts() {
      try {
        const accs = await Wails.call('GetAccounts');
        this.accounts = accs || [];
      } catch (e) { this.error = e.toString(); }
    },

    async loadLabels() {
      const accountId = this.activeAccountId === 'unified'
        ? (this.accounts[0]?.id || '')
        : this.activeAccountId;
      if (!accountId) return;
      try {
        const labels = await Wails.call('GetLabels', accountId);
        this.labels = labels || [];
      } catch (e) { /* labels are non-critical */ }
    },

    async loadInbox() {
      this.loading = true;
      this.allLoaded = false;
      this.error = null;
      try {
        const threads = await Wails.call('GetInbox', this.activeAccountId, 60, 0);
        this.threads = threads || [];
        if (this.threads.length < 60) this.allLoaded = true;
      } catch (e) { this.error = e.toString(); }
      finally { this.loading = false; }
    },

    async selectAccount(id) {
      this.activeAccountId = id;
      this.activeThread = null;
      this.messages = [];
      this.searchQuery = '';
      await Promise.all([this.loadInbox(), this.loadLabels()]);
    },

    async selectThread(thread) {
      this.activeThread = thread;
      if (this._markReadTimer) { clearTimeout(this._markReadTimer); this._markReadTimer = null; }
      try {
        const msgs = await Wails.call('GetThread', thread.accountId, thread.id);
        this.messages = msgs || [];
        if (this.messages.some(m => m.isUnread)) {
          this._markReadTimer = setTimeout(() => this._autoMarkRead(thread), 5000);
        }
      } catch (e) { this.error = e.toString(); }
    },

    async _autoMarkRead(thread) {
      if (this.activeThread?.id !== thread.id) return;
      for (const msg of this.messages.filter(m => m.isUnread)) {
        try { await Wails.call('MarkMessageRead', msg.accountId, msg.id); msg.isUnread = false; }
        catch (e) { /* best-effort */ }
      }
      const t = this.threads.find(t => t.id === thread.id && t.accountId === thread.accountId);
      if (t) t.isUnread = false;
    },

    async trashThread() {
      if (!this.activeThread) return;
      const { id, accountId } = this.activeThread;
      try {
        await Wails.call('TrashThread', accountId, id);
        this.threads = this.threads.filter(t => !(t.id === id && t.accountId === accountId));
        this.activeThread = null;
        this.messages = [];
        this.composing = false;
      } catch (e) { this.error = e.toString(); }
    },

    async markUnread() {
      if (!this.activeThread || !this.messages.length) return;
      const last = this.messages[this.messages.length - 1];
      try {
        await Wails.call('MarkMessageUnread', last.accountId, last.id);
        last.isUnread = true;
        const t = this.threads.find(t => t.id === this.activeThread.id && t.accountId === this.activeThread.accountId);
        if (t) t.isUnread = true;
      } catch (e) { this.error = e.toString(); }
    },

    async fetchBody(msg) {
      try {
        const full = await Wails.call('GetMessage', msg.accountId, msg.id);
        if (!full) return;
        const idx = this.messages.findIndex(m => m.id === msg.id);
        if (idx >= 0) this.messages[idx] = full;
      } catch (e) { console.error('[fetchBody]', e); }
    },

    async sync() {
      if (this.syncStatus === 'syncing') return;
      this.syncStatus = 'syncing';
      try {
        await Wails.call('Sync', this.activeAccountId);
      } catch (e) {
        this.syncStatus = 'idle';
        this.error = 'Sync failed: ' + e.toString();
      }
    },

    async selectLabel(labelId) {
      this.activeLabel = labelId;
      this.activeThread = null;
      this.messages = [];
      this.composing = false;
      if (labelId === 'INBOX') {
        await this.loadInbox();
      } else {
        await this.loadLabelThreads(labelId);
      }
    },

    async loadLabelThreads(labelId) {
      this.loading = true;
      this.allLoaded = false;
      this.error = null;
      try {
        const threads = await Wails.call('GetLabelThreads', this.activeAccountId, labelId, 60, 0);
        this.threads = threads || [];
        if (this.threads.length < 60) this.allLoaded = true;
      } catch (e) { this.error = e.toString(); }
      finally { this.loading = false; }
    },

    onListScroll(e) {
      const el = e.target;
      if (el.scrollTop + el.clientHeight >= el.scrollHeight - 200) this.loadMore();
    },

    async loadMore() {
      if (this.loadingMore || this.allLoaded || this.loading || this.searchQuery) return;
      this.loadingMore = true;
      try {
        const offset = this.threads.length;
        let more;
        if (this.activeLabel === 'INBOX') {
          more = await Wails.call('GetInbox', this.activeAccountId, 60, offset);
        } else {
          more = await Wails.call('GetLabelThreads', this.activeAccountId, this.activeLabel, 60, offset);
        }
        more = more || [];
        if (more.length < 60) this.allLoaded = true;
        this.threads = [...this.threads, ...more];
      } catch (_) { /* silently ignore load-more errors */ }
      finally { this.loadingMore = false; }
    },

    async search(q) {
      this.searchQuery = q;
      this.allLoaded = true; // no pagination for search results
      if (!q.trim()) { await this.loadInbox(); return; }
      this.loading = true;
      try {
        const threads = await Wails.call('SearchInbox', q, 50);
        this.threads = threads || [];
        this.activeThread = null;
        this.messages = [];
      } catch (e) { this.error = e.toString(); }
      finally { this.loading = false; }
    },

    openCompose(mode = 'new', replyTo = null) {
      this.composeMode = mode;
      this.sendError = null;
      this.showCc = false;
      this.showBcc = false;
      const defaultAccountId = this.accounts.length > 0 ? this.accounts[0].id : '';
      if (mode === 'reply' && replyTo) {
        const subj = replyTo.subject || '';
        this.composeData = {
          accountId: replyTo.accountId || defaultAccountId,
          to:        replyTo.from || '',
          cc:        '',
          bcc:       '',
          subject:   subj.startsWith('Re: ') ? subj : 'Re: ' + subj,
          body:      buildQuote(replyTo),
          inReplyTo: replyTo.id || '',
          threadId:  replyTo.threadId,
        };
      } else if (mode === 'reply-all' && replyTo) {
        const subj = replyTo.subject || '';
        this.showCc = true;
        this.composeData = {
          accountId: replyTo.accountId || defaultAccountId,
          to:        replyTo.from || '',
          cc:        replyTo.to || '',
          bcc:       '',
          subject:   subj.startsWith('Re: ') ? subj : 'Re: ' + subj,
          body:      buildQuote(replyTo),
          inReplyTo: replyTo.id || '',
          threadId:  replyTo.threadId,
        };
      } else if (mode === 'forward' && replyTo) {
        const subj = replyTo.subject || '';
        this.composeData = {
          accountId: replyTo.accountId || defaultAccountId,
          to:        '',
          cc:        '',
          bcc:       '',
          subject:   subj.startsWith('Fwd: ') ? subj : 'Fwd: ' + subj,
          body:      buildQuote(replyTo),
          inReplyTo: '',
          threadId:  '',
        };
      } else {
        this.composeData = { accountId: defaultAccountId, to: '', cc: '', bcc: '', subject: '', body: '', inReplyTo: '', threadId: '' };
      }
      this.composing = true;
    },

    closeCompose() { this.composing = false; this.sendError = null; this.showCc = false; this.showBcc = false; },

    async sendCompose() {
      if (this.sending) return;
      this.sendError = null;
      this.sending = true;
      try {
        await Wails.call('SendMessage', this.composeData);
        this.composing = false;
      } catch (e) {
        this.sendError = e.toString();
      } finally { this.sending = false; }
    },
  });

  // ── Sidebar component ──────────────────────────────────────────────────────
  Alpine.data('sidebar', () => ({
    moreOpen: false,

    // System label IDs always pinned at top
    PINNED: ['INBOX', 'STARRED', 'SENT', 'DRAFT'],
    // System label IDs never shown in sidebar (internal / category tabs)
    HIDDEN: new Set(['UNREAD', 'CATEGORY_PERSONAL', 'CATEGORY_SOCIAL',
                     'CATEGORY_PROMOTIONS', 'CATEGORY_UPDATES', 'CATEGORY_FORUMS']),
    // Fallback names when labels haven't loaded yet
    LABEL_NAMES: { INBOX: 'Inbox', STARRED: 'Starred', SENT: 'Sent', DRAFT: 'Drafts',
                   IMPORTANT: 'Important', SPAM: 'Spam', TRASH: 'Trash' },
    // Known system label IDs that have dedicated icons in the sprite
    ICON_IDS: new Set(['INBOX','STARRED','SENT','DRAFT','IMPORTANT','SPAM','TRASH','ALL_MAIL','SNOOZED']),

    get store() { return Alpine.store('mail'); },

    get pinnedLabels() {
      return this.PINNED.map(id => {
        const found = this.store.labels.find(l => l.id === id);
        return found || { id, name: this.LABEL_NAMES[id] || id, type: 'system' };
      });
    },
    get moreSystemLabels() {
      return this.store.labels.filter(l =>
        l.type === 'system' && !this.PINNED.includes(l.id) && !this.HIDDEN.has(l.id)
      );
    },
    get userLabels() {
      return this.store.labels.filter(l => l.type === 'user');
    },
    get hasMore() {
      return this.moreSystemLabels.length > 0 || this.userLabels.length > 0;
    },

    iconKey(id)        { return this.ICON_IDS.has(id) ? id : '_tag'; },
    isActiveLabel(id)  { return this.store.activeLabel === id; },
    selectLabel(id)    { this.store.selectLabel(id); },

    selectAccount(id)  { this.store.selectAccount(id); },
    openCompose()      { this.store.openCompose('new'); },
  }));

  // ── Email list component ───────────────────────────────────────────────────
  Alpine.data('emailList', () => ({
    formatDate,
    initials,
    avatarColor,
    get store() { return Alpine.store('mail'); },

    // Gmail system star label ID → SVG icon ID + Tailwind colour class.
    // All starred messages carry STARRED; coloured stars add a second label ID.
    _STAR_LABELS: {
      'YELLOW_STAR':   { icon: 'icon-star-filled',    cls: 'text-yellow-400' },
      'BLUE_STAR':     { icon: 'icon-star-filled',    cls: 'text-blue-400'   },
      'RED_CIRCLE':    { icon: 'icon-star-bang',      cls: 'text-red-500'    },
      'ORANGE_CIRCLE': { icon: 'icon-star-guillemet', cls: 'text-orange-400' },
      'GREEN_CIRCLE':  { icon: 'icon-star-check',     cls: 'text-green-500'  },
      'PURPLE_CIRCLE': { icon: 'icon-star-question',  cls: 'text-purple-500' },
    },

    labelName(id) {
      const found = this.store.labels.find(l => l.id === id);
      return found ? found.name : id;
    },

    labelInfo(id) {
      return this.store.labels.find(l => l.id === id) || null;
    },

    // Returns {char, cls} for the thread's star type, or null if not starred.
    // Checks for a specific coloured-star label first; falls back to yellow ★.
    starInfo(thread) {
      for (const id of (thread.labelIds || [])) {
        const match = this._STAR_LABELS[id];
        if (match) return match;
      }
      if (thread.isStarred) return this._STAR_LABELS['YELLOW_STAR'];
      return null;
    },

    // Returns user-created label IDs that should appear as chips (max 2).
    // Hides all system labels (they are already represented in the sidebar).
    visibleLabels(thread) {
      return (thread.labelIds || []).filter(id => {
        const lbl = this.store.labels.find(l => l.id === id);
        return lbl && lbl.type === 'user';
      }).slice(0, 2);
    },

    isActive(thread) {
      return this.store.activeThread?.id === thread.id &&
             this.store.activeThread?.accountId === thread.accountId;
    },

    selectThread(thread) { this.store.selectThread(thread); },
    sync()               { this.store.sync(); },

    doSearch(e) {
      if (e.key === 'Enter') this.store.search(this.store.searchQuery);
      if (e.key === 'Escape') { this.store.searchQuery = ''; this.store.loadInbox(); }
    },
  }));

  // ── Reading pane component (also owns compose UI) ─────────────────────────
  Alpine.data('readingPane', () => ({
    formatDate,
    initials,
    avatarColor,
    expanded: [],

    get store() { return Alpine.store('mail'); },
    get data()  { return Alpine.store('mail').composeData; },

    init() {
      this.$watch('store.messages', msgs => {
        if (!msgs || msgs.length === 0) { this.expanded = []; return; }
        const last = msgs[msgs.length - 1];
        this.expanded = [last.id];
        if (!last.body) this.store.fetchBody(last);
      });
    },

    toggle(msg) {
      const idx = this.expanded.indexOf(msg.id);
      if (idx >= 0) {
        this.expanded.splice(idx, 1);
      } else {
        this.expanded.push(msg.id);
        if (!msg.body) this.store.fetchBody(msg);
      }
    },

    isExpanded(id) { return this.expanded.includes(id); },

    reply(msg)      { this.store.openCompose('reply', msg); },
    replyAll(msg)   { this.store.openCompose('reply-all', msg); },
    forward(msg)    { this.store.openCompose('forward', msg); },

    // Called by x-init on compose container when it's inserted into DOM
    initEditor() {
      this.$nextTick(() => {
        const ed = this.$refs.editor;
        if (!ed) return;
        ed.innerHTML = this.data.body || '';
        ed.focus();
        const sel = window.getSelection();
        if (sel) {
          const range = document.createRange();
          range.setStart(ed, 0);
          range.collapse(true);
          sel.removeAllRanges();
          sel.addRange(range);
        }
      });
    },

    exec(cmd, val = null) {
      this.$refs.editor.focus();
      document.execCommand(cmd, false, val);
      this.store.composeData.body = this.$refs.editor.innerHTML;
    },

    insertLink() {
      const url = window.prompt('Enter URL:', 'https://');
      if (url) this.exec('createLink', url);
    },

    syncAndSend() {
      if (this.$refs.editor) {
        this.store.composeData.body = this.$refs.editor.innerHTML;
      }
      this.store.sendCompose();
    },

    handleKeydown(e) {
      if (!this.store.composing) return;
      if (e.key === 'Escape') { e.preventDefault(); this.store.closeCompose(); return; }
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); this.syncAndSend(); return; }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === 'C') { e.preventDefault(); this.store.showCc = !this.store.showCc; return; }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === 'B') { e.preventDefault(); this.store.showBcc = !this.store.showBcc; return; }
    },
  }));

  // ── Address autocomplete ───────────────────────────────────────────────────
  Alpine.data('addressField', (field, placeholder) => ({
    suggestions: [],
    open: false,
    activeIdx: -1,
    _timer: null,

    get value()  { return this.$store.mail.composeData[field]; },
    set value(v) { this.$store.mail.composeData[field] = v; },

    lastToken() {
      const parts = this.value.split(',');
      return parts[parts.length - 1].trim();
    },

    async fetchSuggestions() {
      const q = this.lastToken();
      if (q.length < 1) { this.suggestions = []; this.open = false; return; }
      const accountId = this.$store.mail.composeData.accountId || 'unified';
      try {
        const r = await Wails.call('SearchContacts', accountId, q, 8);
        this.suggestions = r || [];
        this.activeIdx = -1;
        this.open = this.suggestions.length > 0;
      } catch(_) { this.suggestions = []; }
    },

    onInput() {
      clearTimeout(this._timer);
      this._timer = setTimeout(() => this.fetchSuggestions(), 150);
    },

    select(s) {
      const parts = this.value.split(',').map(p => p.trim()).filter(Boolean);
      parts.pop();
      parts.push(s.name ? `${s.name} <${s.email}>` : s.email);
      this.value = parts.join(', ') + ', ';
      this.close();
      this.$nextTick(() => this.$refs.input.focus());
    },

    close() { this.suggestions = []; this.open = false; this.activeIdx = -1; },

    onKeydown(e) {
      if (!this.open) return;
      if (e.key === 'ArrowDown')  { e.preventDefault(); this.activeIdx = Math.min(this.activeIdx + 1, this.suggestions.length - 1); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); this.activeIdx = Math.max(this.activeIdx - 1, -1); }
      else if (e.key === 'Tab' && this.suggestions.length > 0) { e.preventDefault(); this.select(this.suggestions[this.activeIdx >= 0 ? this.activeIdx : 0]); }
      else if (e.key === 'Enter' && this.activeIdx >= 0) { e.preventDefault(); this.select(this.suggestions[this.activeIdx]); }
      else if (e.key === 'Escape') { e.preventDefault(); this.close(); }
    },

    onBlur() { setTimeout(() => this.close(), 200); },
  }));
});
