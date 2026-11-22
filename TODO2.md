# TODO 2

## WebUI
 [x] Type error when I sync from WebUI: FIXED
 [x] Quote original email in reply, GMail style
 [x] Forward and reply buttons do nothing — inline compose at bottom of thread, full-pane for new mail
 [x] TailWindCSS CDN load warning in browser console — vendored alpine.js and tailwind, no external requests
 [ ] Mail signature management in account settings page
 [ ] Display all labels in left panel, but hide nonsense labels like stars variants
 [x] Hide left panel like GMail
 [ ] The ? key should bring up shortcuts like GMail
 [x] GMail shortcut key support, advanced shortcut keys like "g i" go to inbox, "/" search, "#" delete, "!" spam
 [x] Load label colors from the backend, now all labels are just grey
 [x] Inbox not loading all emails, it's limited view. Sync has everything though
 [ ] How should I fetch new mail? Use websocket from WebUI, sync from GMail API to golang backend?
 [x] Pagination or unlimited scroll for mails that are already synced to sqlite.
 [ ] Pagination for mails (metadata only) for mails that are not yet synced.
 [ ] Implement right click functionality for advanced actions.
 [ ] Prevent mail send when To and CC are blank
 [x] Trash / mark-read / mark-unread wired in the Fyne desktop app
 [x] Star -- types to match GMail
 [ ] Star -- clicking on stars cycles through the star types
 [ ] Dark/Light mode must detect system settings and configure accordingly
 [ ] View attachments, browser viewable inline, other file types offer save as (like WhatsApp)
 [ ] Change HTML mail background to grey, now it's full white. This is for dark mode only. 


## General
 [ ] sync should fetch metadata for all, download body only for sync period?
 [ ] search must do backend search and cache results locally. fetch metadata only, load body on demand.
 [x] Replies do not thread in GMail, missing mail reply headers? — fixed RFC 2822 In-Reply-To/References headers
