# ujawhales deployment fork

This branch starts at upstream `v0.4.8` (`fd209174b5088e2b1a630401a3f05f5899810e27`).
`origin` is SeungWoonSong/omo-webchat; `upstream` is DevNewbie1826/omo-webchat.
Keep upstream module paths and the MIT license. This initial fork changes no application behavior.

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
history. Future subscription-login UI and Claude-history features belong here;
neither is implemented by this deployment.

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
ExecStart=/opt/ai-portal/webchat/fork-<shortsha>/omo-webchat --host 127.0.0.1 --port 3102 --root /home/ujawhales/projects --state-dir /var/lib/ai-portal/webchat/ujawhales
```

```sh
sudo systemctl daemon-reload
sudo systemctl restart ai-portal-webchat@ujawhales
systemctl is-active ai-portal-webchat@ujawhales
curl --fail http://127.0.0.1:3102/ -o /dev/null
```

Never alter `/opt/ai-portal/webchat/current` or restart the lifening-dev
instance on port 3101. Record its MainPID before and after deployment.

## Roll back to stock 0.4.8

In the same ujawhales override, change only the executable to
`/opt/ai-portal/webchat/0.4.8/omo-webchat`, keeping all arguments.
Run `sudo systemctl daemon-reload` and
`sudo systemctl restart ai-portal-webchat@ujawhales`, then check HTTP 200 and
successful login. This preserves state and the current password; server
restart invalidates login cookies. Leave port 3101 and the shared symlink alone.

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
