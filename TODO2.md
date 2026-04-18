# TODO 2

## WebUI
 [x] Type error when I sync from WebUI: FIXED
 [x] Quote original email in reply, GMail style
 [x] Forward and reply buttons do nothing — inline compose at bottom of thread, full-pane for new mail
 [x] TailWindCSS CDN load warning in browser console — vendored alpine.js and tailwind, no external requests
 [ ] Mail signature management in account settings page
 [ ] Display all labels in left panel
 [ ] Hide left panel like GMail
 [ ] The ? key should bring up shortcuts like GMail
 [ ] GMail shortcut key support, advanced shortcut keys like "g i" go to inbox.
 [ ] Inbox not loading all emails, it's limited view. Sync has everything though
 [ ] How should I fetch new mail? Use websocket from WebUI, sync from GMail API to golang backend?
 [ ] Pagination or unlimited scroll for mailbox metadata view, now only 60 emails are loaded no way to paginate.
 [ ] Implement right click functionality for advanced actions.
 [ ] Prevent mail send when To and CC are blank
 [ ] Add CC, BCC fields
 [ ] Trash / mark-read / mark-unread not wired in web server (HTTP mode) — only works in Wails desktop

## General
 [ ] sync should fetch metadata for all, download body only for sync period?
 [ ] search must do backend search and cache results locally. fetch metadata only, load body on demand.
 [x] Replies do not thread in GMail, missing mail reply headers? — fixed RFC 2822 In-Reply-To/References headers
