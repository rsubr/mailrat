# UX tests under Xvfb

Run mailrat in a virtual X display (Xvfb + openbox) so it can be driven by
screenshots and xdotool clicks. State lives in the gitignored `.ux/`.

## Display target

All UI tests run only on the MacBook Air 13" M4 default scaled display: 2560x1664 native, 2x,
so 1470x956 logical points; usable area 1470x918 (minus the ~38pt macOS menu bar).
Assumption: 13-inch, default scaling (not the 15-inch 1710x1107).
Xvfb emulation: 1470x956x24 with `FYNE_SCALE=1` (same logical layout as the Mac, lighter than 2940x1912 at scale 2).
The app window is sized 1470x918 at 0,0 by `start.sh`; `ux.sh size` prints the config (`UX_LOGICAL_W/H` in `env.sh`
is the single source of truth). `ux.sh shot` downscales shots wider than 2000px to 1470 and keeps `NAME-full.png`.

Rule: do not test other resolutions.

## Start / stop
```
scripts/ux/setup.sh      # once: apt packages (prints them if no root)
scripts/ux/start.sh      # Xvfb :99 1470x956, openbox, builds + launches app
scripts/ux/stop.sh
```
`start.sh` sets `MAILRAT_DATA_DIR=.ux/profile`, `MAILRAT_OAUTH_HEADLESS=1`,
`MAILRAT_OAUTH_PORT=53682`. Tokens are stored in the profile's SQLite db (there
is no keyring), so no extra token-store switch exists. It copies
`~/.mailrat/credentials.json` into the profile if the profile has none.

## Watching the display in a browser
`scripts/ux/vnc.sh start|stop|status|url` runs x11vnc on `127.0.0.1:5900` (no authentication: security relies on Tailscale) and noVNC
(websockify) on port 6080, bound to the Tailscale IP only (never 0.0.0.0/eth0). `start.sh` does this only with `UX_VNC=1`;
`stop.sh` stops it. Open the URL printed by `vnc.sh url`
(`http://<tailscale-ip>:6080/vnc.html?autoconnect=1&resize=scale&path=websockify`); no password is asked.
Without `tailscale0` it binds `127.0.0.1`; tunnel with `ssh -L 6080:127.0.0.1:6080 <user>@<host>` and open `http://127.0.0.1:6080/...`.
Your mouse/keyboard and Claude's xdotool input drive the same X server, so do not both drive at once.

## Driving it (Claude)
`scripts/ux/ux.sh shot NAME` writes `.ux/shots/NAME.png`; Read that file to see the
screen. Then `click X Y`, `dblclick X Y`, `type TEXT`, `key KEYS`, `move X Y`,
`wait SECS`, `window`, `oauth-url`. Coordinates are screen pixels in the shot.

## Google login (human, once)
1. Claude clicks Add Account; the consent URL is written to `.ux/profile/oauth-url.txt`
   (`ux.sh oauth-url`) and printed as `OAUTH_URL:` in `.ux/mailrat.log`.
2. Open that URL in a browser on your machine and approve with the TEST account.
3. Finish with either method:
   - SSH forward: before step 2 run `ssh -L 53682:127.0.0.1:53682 <user>@<lxc-host>`;
     Google then redirects to the app's listener and login completes by itself.
   - Paste redirect: without forwarding the final page fails to load. Copy the full
     address (`http://127.0.0.1:53682/callback?state=...&code=...`) and run
     `echo '<that url>' > .ux/profile/oauth-redirect.txt` on the LXC (polled every
     second), or paste it into the app's sign-in dialog.
4. Run `scripts/ux/save-golden.sh` to snapshot the logged-in profile.

## Safety
Use a dedicated test Gmail account only. Never send to real recipients; use the
test account's own address. Do not use a personal mailbox.

## Reset
`scripts/ux/reset.sh` stops the app and restores `.ux/golden` (or wipes the profile
if no golden exists); then `start.sh`.
