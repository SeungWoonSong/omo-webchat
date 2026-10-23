# ujawhales deployment fork

This branch starts at upstream `v0.4.8` (`fd209174b5088e2b1a630401a3f05f5899810e27`).
`origin` is SeungWoonSong/omo-webchat; `upstream` is DevNewbie1826/omo-webchat.
Keep upstream module paths and the MIT license.

## Integrated features

- `feat/claude-import` (`27808b2`, merge `055eb18`): offline Claude JSONL
  converter in `tools/claude-import`. It creates native OMO v3 history, preserves
  parent/subagent relationships and labels sessions `[Claude]`. It never runs
  historical tools or modifies source transcripts.
- `feat/accounts` (`f4dfd87`, merge `b5bc1a9`): Settings > Accounts lists
  Anthropic Subscription (`anthropic-subscription`), ChatGPT Subscription
  (`chatgpt-subscription`) and Cursor status, and lets the owner add a login.
  Login flows use private, short-lived RPC sessions, not chat messages.

The deployed integration build is merge commit
`b5bc1a9e779a4cdc8cd71641226bd0bb9916c71b`, installed at
`/opt/ai-portal/webchat/fork-b5bc1a9/omo-webchat` (root:root, 0755).
Its `vcs.modified=false`. A subsequent documentation-only commit does not change
this build identity. The existing 17 workspaces and eight enrolled Claude
imports remain in the existing state directory.

## Re-run Claude import

From this repository, with the Go toolchain below:

```sh
export PATH="$HOME/.local/toolchains/go1.26.10/bin:$PATH"
go build -o /tmp/claude-import ./tools/claude-import
sudo -u ujawhales /tmp/claude-import \
  -root /home/ujawhales/Claude-Cowork \
  -sessions /home/ujawhales/.omo/agent/sessions -dry-run
# After reviewing the dry-run receipt, omit -dry-run to import.
sudo -u ujawhales /tmp/claude-import \
  -root /home/ujawhales/Claude-Cowork \
  -sessions /home/ujawhales/.omo/agent/sessions
rm /tmp/claude-import
```

Run as the target owner. Re-running skips existing destinations, including
changed-source snapshots, without overwriting resumed sessions or creating
duplicates. Sources stay read-only; destination directories are 0700 and session
files are 0600. The existing eight imports are already enrolled in webchat.
For newly discovered imports, conversion alone is not catalog enrollment: use
the authenticated workspace session-adoption API described in
`tools/claude-import/README.md`, with the correct workspace under the allowed
root. Do not edit the private cursor store.

## Owner account login

Log into webchat with its existing password, then open Settings > Accounts.
Check provider status and select Add / Login on the desired provider.
Complete login in the new provider tab using the owner's own account. If
prompted, paste the redirect URL or code into the Accounts panel and submit.
For ChatGPT, choose Browser login. Use Cancel to abandon a flow; closing the
panel also aborts an unfinished flow. Refresh the provider list to check the
result. More owner accounts can be added from the same menu.

Never paste codes or login URLs into a conversation, logs, screenshots or Git.
Integration QA only starts a login, checks the URL, then cancels; it does not
complete OAuth or replace OMO credentials.

## Rebuild

Use Go 1.26 and Node 22 (initial deployment: Go 1.26.10, Node 22.22.3,
Bun 1.4.4). Go is installed in user space at
`/home/lifening-dev/.local/toolchains/go1.26.10`.

```sh
export PATH="$HOME/.local/toolchains/go1.26.10/bin:$PATH"
GIT_MASTER=1 git switch ujawhales
GIT_MASTER=1 git status --short
# Commit intended changes first: Go embeds the VCS revision and dirty flag.
make build
go version -m bin/omo-webchat
go test ./...
(cd frontend && npx vitest run)
```

The README build embeds the React production bundle into `bin/omo-webchat`.
`go version -m` is the build identity command: compare `vcs.revision` with
`git rev-parse HEAD` and require `vcs.modified=false`.

## Upgrade from upstream

```sh
GIT_MASTER=1 git fetch upstream --tags
GIT_MASTER=1 git switch ujawhales
GIT_MASTER=1 git merge <reviewed-upstream-tag>
```

Resolve changes, run the rebuild and tests above, and push `ujawhales` to
`origin`. Do not replace fork changes with a release binary or rewrite shared
history. Preserve the account-login and Claude-import fork changes.

## Deploy only ujawhales

```sh
sha=$(GIT_MASTER=1 git rev-parse --short HEAD)
sudo install -d -o root -g root -m 0755 "/opt/ai-portal/webchat/fork-$sha"
sudo install -o root -g root -m 0755 bin/omo-webchat \
  "/opt/ai-portal/webchat/fork-$sha/omo-webchat"
```

Edit only `/etc/systemd/system/ai-portal-webchat@ujawhales.service.d/override.conf`.
Preserve its `CHAT_PI_BINARY` override. Keep `ExecStart=` to clear the inherited
command, then set the command to:

```ini
ExecStart=/opt/ai-portal/webchat/fork-<shortsha>/omo-webchat --host 127.0.0.1 --port 3102 --root /home/ujawhales/Claude-Cowork --state-dir /var/lib/ai-portal/webchat/ujawhales
```

```sh
sudo systemctl daemon-reload
sudo systemctl restart ai-portal-webchat@ujawhales
systemctl is-active ai-portal-webchat@ujawhales
curl --fail http://127.0.0.1:3102/ -o /dev/null
```

Never alter `/opt/ai-portal/webchat/current` or restart the lifening-dev
instance on port 3101. Record its MainPID before and after deployment.

## Roll back this integration

In the same ujawhales override, change only the executable to
`/opt/ai-portal/webchat/fork-282f5ac/omo-webchat`, keeping all arguments,
especially `--root /home/ujawhales/Claude-Cowork`, and the `CHAT_PI_BINARY`
environment override. The previous directory is retained for rollback.
Run `sudo systemctl daemon-reload` and
`sudo systemctl restart ai-portal-webchat@ujawhales`, then check HTTP 200 and
successful login. This preserves state and the current password; server
restart invalidates login cookies. Leave port 3101 and the shared symlink alone.
The Accounts panel is absent in the previous binary; the imported session data
and catalog are retained. No auth-file restoration or transcript modification
is part of rollback.

## Change the password

Use a protected local editor (`sudoedit /etc/ai-portal/webchat/ujawhales.env`)
to set `TH_PASSWORD` without placing the value in command arguments, shell
history, Git, screenshots, or evidence. Preserve other environment settings.
Ensure `sudo chown root:ujawhales /etc/ai-portal/webchat/ujawhales.env` and
`sudo chmod 0640 /etc/ai-portal/webchat/ujawhales.env`, then restart only
`ai-portal-webchat@ujawhales`. Test a successful login and verify an incorrect
password returns 401 without a Set-Cookie header.

The owner's intended ID is `wonhyuk`, but stock webchat has only a password
field. There is no username login field in this step.

## Tailscale/front proxy

The 2026-10-23 front-proxy deployment keeps the webchat process on
`127.0.0.1:3102`. On whitening1, these root-owned units under
`/etc/systemd/system` expose only `100.116.235.1:3102`:

- `ai-portal-webchat-ujawhales-ts.socket`: `ListenStream=100.116.235.1:3102`,
  `FreeBind=yes`, enabled under `sockets.target`.
- `ai-portal-webchat-ujawhales-ts.service`: socket-activated
  `/usr/lib/systemd/systemd-socket-proxyd 127.0.0.1:3102`, with `DynamicUser=yes`,
  requiring and ordered after `ai-portal-webchat@ujawhales.service`.

The socket has no wildcard, LAN, or public bind. `FreeBind=yes` allows it to
start before Tailscale assigns the address. No Tailscale, ACL, or nft settings
were changed. The comparison dashboard, `omo-usage-dashboard`, uses Docker
host networking and its Python process listens on `127.0.0.1:8377`; its ens4
Caddy configuration was not accessible and was not modified.

The owner will point `ai.sungwoonsong.com` at `221.154.187.87`, the ens4 node
(`100.105.223.66`). Its administrator must add and reload:

```caddyfile
ai.sungwoonsong.com {
    reverse_proxy 100.116.235.1:3102
}
```

Preserve the incoming Host and Origin; do not override Host to the upstream IP.
`internal/wsbridge/bridge.go:303-320` accepts HTTP/HTTPS Origins whose host
matches `r.Host` case-insensitively, including any explicit port. No extra
header directive is needed. In particular, `https://ai.sungwoonsong.com` with
`Host: ai.sungwoonsong.com` is accepted over the HTTP upstream connection.

Known shared-lockout risk: `internal/api/router.go:179-185` identifies clients
only by `RemoteAddr`; it never trusts `X-Forwarded-For`. With this TCP proxy,
webchat sees `127.0.0.1` for every forwarded client (including local users).
`internal/auth/auth.go:23-27,110-155` bans that IP for one hour after ten wrong
passwords within a fixed one-hour window. The tenth failure returns 401;
subsequent attempts return 429, even with the correct password, until the ban
expires. A successful attempt before the ban resets the failure count. Merely
adding X-Forwarded-For at Caddy will not separate clients. This behavior is
documented, not changed.

Local verification and reproducible browser checks are under
`/home/lifening-dev/ai-website/.omo/evidence/uja-webchat-fork/tailscale/`.
The browser check uses a temporary loopback forward proxy to send HTTP and
native WebSocket upgrades to the Tailscale IP with the public Host, HTTPS
Origin and forwarded scheme. HTTP and login returned 200; the authenticated
WebSocket returned 101 and exchanged hello/pong, all with exit 0. It does not
prove ens4 ingress, public DNS or
TLS. No SSH access to ens4 was available; its Tailscale ping is reachability
evidence only. Disable the new entry point, if needed, with
`sudo systemctl disable --now ai-portal-webchat-ujawhales-ts.socket` followed
by `sudo systemctl stop ai-portal-webchat-ujawhales-ts.service`; the original
loopback webchat remains available.

## Track B — chat file exchange

- `POST /api/workspaces/{wsId}/chats/{chatId}/upload` accepts optional multipart `dir` (workspace-relative; created if missing). Root/symlink escapes are rejected using `os.Root`. All callers, including the file panel, receive `{ "uploaded": ["uploads/name (1).ext"] }` with final workspace-relative names. Existing names are never overwritten (`O_EXCL`, then ` (1)`, ` (2)`, … before the final extension). The 100 MiB limit includes the multipart envelope.
- Composer `+`, drop and paste accept multiple files. Non-images upload immediately, individually, to `uploads/`; pending/error chips block submission until completed, retried or removed. Removing a chip does **not** delete the uploaded file. Images retain the existing single-image content slot and model-capability gate; image bytes are not uploaded as ordinary files.
- Stable, locale-independent prompt suffix (two leading newlines, literal Korean marker, one unescaped workspace-relative path per line):
  ```text


  [첨부 파일]
  - uploads/보고서 (1).pdf
  - uploads/data.xlsx
  ```
  The exact delimiter is `\n\n[첨부 파일]\n`; entries use `- ` with no trailing newline. The existing session dispatcher trims prompts, so an attachment-only prompt can start directly with `[첨부 파일]`; the transcript accepts both forms. Newline-containing upload names are rejected. Failed-send drafts keep this suffix as text; transcript user bubbles display it as clickable file chips.
- Assistant markdown relative file links and inline file paths with extensions, and write/edit/create tool targets, open the existing file panel viewer. Rendering does not perform filesystem requests. File existence is checked on click; missing files show a localized toast. Absolute paths are only recognized under the current workspace root; external URLs, code fences, paths escaping the root, and plain words stay unchanged.
