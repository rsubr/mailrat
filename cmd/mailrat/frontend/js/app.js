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

    async init() {
      await this.loadAccounts();
      await this.loadInbox();

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
      await this.loadInbox();
    },

    async selectThread(thread) {
      this.activeThread = thread;
      try {
        const msgs = await Wails.call('GetThread', thread.accountId, thread.id);
        this.messages = msgs || [];
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
          body:      '',
          inReplyTo: replyTo.messageId || '',
          threadId:  replyTo.threadId,
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
    systemLabels: SYSTEM_LABELS,
    get store() { return Alpine.store('mail'); },

    selectAccount(id) { this.store.selectAccount(id); },
    openCompose()     { this.store.openCompose('new'); },
  }));

  // ── Email list component ───────────────────────────────────────────────────
  Alpine.data('emailList', () => ({
    formatDate,
    initials,
    avatarColor,
    get store() { return Alpine.store('mail'); },

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

  // ── Reading pane component ─────────────────────────────────────────────────
  Alpine.data('readingPane', () => ({
    formatDate,
    initials,
    avatarColor,
    expanded: [],          // message IDs that are currently expanded

    get store() { return Alpine.store('mail'); },

    init() {
      this.$watch('store.messages', msgs => {
        if (!msgs || msgs.length === 0) { this.expanded = []; return; }
        // Expand the last message by default
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

    reply(msg) { this.store.openCompose('reply', msg); },
  }));

  // ── Compose modal component ────────────────────────────────────────────────
  Alpine.data('compose', () => ({
    get store() { return Alpine.store('mail'); },
    get data()  { return Alpine.store('mail').composeData; },

    handleKeydown(e) {
      if (e.key === 'Escape') this.store.closeCompose();
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') this.store.sendCompose();
    },
  }));
});
