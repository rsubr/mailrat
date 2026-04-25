// Browser compatibility shim for the Wails runtime.
// Only activates when the page is NOT running inside the Wails desktop webview
// (i.e. window.go is not already injected). Safe to include in all builds.
(function () {
  if (typeof window.go !== 'undefined') return; // already in Wails — do nothing

  // ── HTTP helper ──────────────────────────────────────────────────────────
  async function api(path, opts) {
    const res = await fetch(path, opts);
    if (!res.ok) {
      const body = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(body.error || res.statusText);
    }
    return res.json();
  }

  function qs(params) {
    return '?' + new URLSearchParams(params).toString();
  }

  // ── window.go shim ───────────────────────────────────────────────────────
  window.go = {
    desktop: {
      App: {
        GetAccounts: () =>
          api('/api/accounts'),

        GetInbox: (accountId, limit, offset) =>
          api('/api/inbox' + qs({ accountId: accountId || '', limit: limit || 60, offset: offset || 0 })),

        SearchInbox: (q, limit) =>
          api('/api/search' + qs({ q, limit: limit || 50 })),

        GetThread: (accountId, threadId) =>
          api('/api/thread' + qs({ accountId, threadId })),

        GetMessage: (accountId, messageId) =>
          api('/api/message' + qs({ accountId, messageId })),

        SendMessage: (msg) =>
          api('/api/send', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(msg),
          }),

        Sync: (accountId) =>
          api('/api/sync' + qs({ accountId: accountId || '' }), { method: 'POST' }),

        GetLabels: (accountId) =>
          api('/api/labels' + qs({ accountId })),

        GetLabelUnreadCounts: (accountId) =>
          api('/api/label-unread-counts' + qs({ accountId })),

        MarkMessageRead: (accountId, messageId) =>
          api('/api/mark-read' + qs({ accountId, messageId }), { method: 'POST' }),

        MarkMessageUnread: (accountId, messageId) =>
          api('/api/mark-unread' + qs({ accountId, messageId }), { method: 'POST' }),

        TrashThread: (accountId, threadId) =>
          api('/api/trash' + qs({ accountId, threadId }), { method: 'POST' }),

        ArchiveThread: (accountId, threadId) =>
          api('/api/archive' + qs({ accountId, threadId }), { method: 'POST' }),

        MarkThreadSpam: (accountId, threadId) =>
          api('/api/spam' + qs({ accountId, threadId }), { method: 'POST' }),

        MarkThreadNotSpam: (accountId, threadId) =>
          api('/api/not-spam' + qs({ accountId, threadId }), { method: 'POST' }),

        GetLabelThreads: (accountId, labelId, limit, offset) =>
          api('/api/label-threads' + qs({ accountId: accountId || '', labelId, limit: limit || 60, offset: offset || 0 })),

        SearchContacts: (accountId, q, limit) =>
          api('/api/contacts/search' + qs({ accountId: accountId || '', q, limit: limit || 8 })),

        RemoveAccount: (accountId) =>
          api('/api/accounts/remove' + qs({ accountId }), { method: 'POST' }),

        GetAccountSettings: (accountId) =>
          api('/api/account-settings' + qs({ accountId })),

        SetAccountSettings: (settings) =>
          api('/api/account-settings/set', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(settings),
          }),

        StartAccountAdd: () => {
          window.location.href = '/api/oauth/start';
          return Promise.resolve();
        },
      },
    },
  };

  // ── window.runtime shim (SSE-backed) ─────────────────────────────────────
  const listeners = {};

  function connectSSE() {
    const src = new EventSource('/api/events');
    src.onmessage = (e) => {
      try {
        const { event, data } = JSON.parse(e.data);
        (listeners[event] || []).forEach((cb) => cb(data));
      } catch (err) {
        console.warn('[wails-compat] SSE parse error:', err);
      }
    };
    src.onerror = () => {
      src.close();
      setTimeout(connectSSE, 3000); // reconnect
    };
  }
  connectSSE();

  window.runtime = {
    EventsOn:   (name, cb) => { (listeners[name] = listeners[name] || []).push(cb); },
    EventsOff:  (name, cb) => { listeners[name] = (listeners[name] || []).filter(f => f !== cb); },
    EventsEmit: () => { /* no-op: browser → Go events not supported in web mode */ },
  };

  console.info('[mailrat] browser mode — using HTTP API at', window.location.origin);
})();
