# yarilo-patches branch

This file lives **only on the `yarilo-patches` branch** of the
[0kaba0hub/go-smtp](https://github.com/0kaba0hub/go-smtp) fork. The `master`
branch mirrors [emersion/go-smtp](https://github.com/emersion/go-smtp)
verbatim and must never carry downstream changes.

## Why this fork exists

[yarilo](https://github.com/yarilomail/yarilo) needs SMTP/LMTP behaviour that
upstream does not carry: the XCLIENT extension on both sides of a proxy, and
the reply texts a mail server's operators and MTAs expect. These are
**permanent yarilo patches**, not cherry-picks of upstream PRs, so there is no
upstream PR to track. They are rebased onto `master` nightly.

## Branch layout

| Branch | Purpose | Contains |
|:---|:---|:---|
| `master` | Upstream mirror | Exactly `emersion/go-smtp` `master`. No downstream commits. |
| `yarilo-patches` | Library yarilo pins | `master` + the commits listed below. This file. |

`yarilo-patches` is the default branch on GitHub. The module path stays
`github.com/emersion/go-smtp`; yarilo imports that path and points it here with
a `replace` directive.

## Patch commits

Hashes are the branch's after the latest rebase; they change on every rebase,
the subjects do not.

| Commit | Subject | What it changes | Why yarilo needs it |
|:---|:---|:---|:---|
| `4ea639a` | feat(client): add XClient() method for XCLIENT relay support (Postfix) | `xclient.go`: `Client.XClient(XClientData)` sends `XCLIENT` with the attributes the caller sets and reads the 220 reply, after which the client must greet again. | `submission-login` relays an authenticated session to the backend and must hand over the real client address, or every message looks as if it came from the proxy. |
| `fbf49fc` | feat(server): standard LMTP/SMTP responses aligned with Dovecot | `conn.go`: the server's reply texts become the terse standard ones: MAIL `250 2.1.0 OK`, RCPT `250 2.1.5 OK`, DATA `250 2.0.0 <rcpt> Saved`, NOOP `250 2.0.0 OK`, instead of upstream's conversational texts and `2.0.0` codes. | An MTA logs these lines and operators grep them; the enhanced codes say which stage accepted the message (RFC 3463: X.1.0 sender, X.1.5 recipient). |
| `097197c` | feat(server): inbound XCLIENT command (Postfix extension) | `server.go`, `conn.go`, `parse.go`, `xclient.go`: `Server.EnableXCLIENT` advertises and accepts `XCLIENT` (off by default); NAME/ADDR/PORT/PROTO/HELO/LOGIN are parsed and passed to a `Session` implementing `XClientReceiver`, then the state is reset with 220. The server makes no trust decision; the receiver does. Tests in `xclient_server_test.go`. | `lmtp-login` sits behind an MTA and must see the original client's attributes for its own logs and checks; it decides whether the peer may send them. |
| `f5b6336` | ci: mirror upstream sync + test workflows (yarilo-patches automation) | `.github/workflows/sync-upstream.yml` (daily: fast-forward `master`, rebase this branch, open an `upstream-conflict` issue on a conflict) and `.github/workflows/go.yml` (build and test). | Keeps the branch on current upstream without manual work, and tests every push. |
| `b88fb02` | ci: pin stable Go toolchain (upstream go.mod declares go 1.13) | `go.yml` builds with `go-version: stable`. | Upstream's `go.mod` says 1.13 while the code uses newer standard library calls, so the declared version cannot build it. |
| `b4fe94b` | ci: keep the scheduled workflow enabled through quiet upstream periods | `sync-upstream.yml` re-enables itself through the API at the end of each run. | GitHub disables a schedule after 60 days without pushes; a quiet upstream would otherwise stop the sync silently. |

## Tracking upstream

Automated by [`.github/workflows/sync-upstream.yml`](.github/workflows/sync-upstream.yml),
daily and on `workflow_dispatch`. Manual fallback:

```sh
git fetch upstream
git checkout master && git merge --ff-only upstream/master && git push origin master
git checkout yarilo-patches && git rebase master
git push --force-with-lease origin yarilo-patches
```

## yarilo's go.mod replace

```
replace github.com/emersion/go-smtp => github.com/0kaba0hub/go-smtp <yarilo-patches-commit>
```

After this branch is re-pushed, repin in yarilo with
`go mod edit -replace github.com/emersion/go-smtp=github.com/0kaba0hub/go-smtp@<new-hash>`
and `go mod tidy`, then run the whole tree.
