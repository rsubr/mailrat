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
    composeData: { accountId: '', to: '', subject: '', body: '', inReplyTo: '', threadId: '' },
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
      this.error = null;
      try {
        const threads = await Wails.call('GetInbox', this.activeAccountId, 60);
        this.threads = threads || [];
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
      this.error = null;
      try {
        const threads = await Wails.call('GetLabelThreads', this.activeAccountId, labelId, 60);
        this.threads = threads || [];
      } catch (e) { this.error = e.toString(); }
      finally { this.loading = false; }
    },

    async search(q) {
      this.searchQuery = q;
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
      const defaultAccountId = this.accounts.length > 0 ? this.accounts[0].id : '';
      if (mode === 'reply' && replyTo) {
        const subj = replyTo.subject || '';
        this.composeData = {
          accountId: replyTo.accountId || defaultAccountId,
          to:        replyTo.from || '',
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
          subject:   subj.startsWith('Fwd: ') ? subj : 'Fwd: ' + subj,
          body:      buildQuote(replyTo),
          inReplyTo: '',
          threadId:  '',
        };
      } else {
        this.composeData = { accountId: defaultAccountId, to: '', subject: '', body: '', inReplyTo: '', threadId: '' };
      }
      this.composing = true;
    },

    closeCompose() { this.composing = false; this.sendError = null; },

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

    // Gmail star label name → display character + Tailwind colour class
    _STAR_LABELS: {
      'yellow star':      { char: '★', cls: 'text-yellow-400' },
      'red star':         { char: '★', cls: 'text-red-500'   },
      'orange star':      { char: '★', cls: 'text-orange-400' },
      'green star':       { char: '★', cls: 'text-green-500' },
      'blue star':        { char: '★', cls: 'text-blue-400'  },
      'purple star':      { char: '★', cls: 'text-purple-500' },
      'red-bang':         { char: '!', cls: 'font-bold text-red-500'   },
      'orange guillemet': { char: '»', cls: 'font-bold text-orange-400' },
      'green check':      { char: '✓', cls: 'font-bold text-green-500' },
      'blue info':        { char: 'ℹ', cls: 'text-blue-400'  },
      'purple question':  { char: '?', cls: 'font-bold text-purple-500' },
    },

    labelName(id) {
      const found = this.store.labels.find(l => l.id === id);
      return found ? found.name : id;
    },

    // Returns {char, cls} for the thread's star type, or null if not starred.
    starInfo(thread) {
      for (const id of (thread.labelIds || [])) {
        if (id === 'STARRED') continue;
        const lbl = this.store.labels.find(l => l.id === id);
        if (!lbl) continue;
        const match = this._STAR_LABELS[lbl.name.toLowerCase()];
        if (match) return match;
      }
      if (thread.isStarred) return this._STAR_LABELS['yellow star'];
      return null;
    },

    // Returns user-created label IDs that should appear as chips (max 2).
    // Hides all system labels and star-type user labels.
    visibleLabels(thread) {
      const starNames = new Set(Object.keys(this._STAR_LABELS));
      return (thread.labelIds || []).filter(id => {
        const lbl = this.store.labels.find(l => l.id === id);
        if (!lbl || lbl.type !== 'user') return false;
        return !starNames.has(lbl.name.toLowerCase());
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

    reply(msg)   { this.store.openCompose('reply', msg); },
    forward(msg) { this.store.openCompose('forward', msg); },

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
      if (e.key === 'Escape') { e.preventDefault(); this.store.closeCompose(); }
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); this.syncAndSend(); }
    },
  }));
});
