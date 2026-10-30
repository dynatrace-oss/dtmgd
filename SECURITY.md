# Security Policy

## Supported versions

Only the latest release of dtmgd receives security fixes. Upgrade with
`brew upgrade dtmgd`, or `go install github.com/dynatrace-oss/dtmgd@latest`.

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Report them by email to opensource@dynatrace.com. PGP keys are available on request.

Include as much of the following as you can:

- Type of issue (for example credential exposure, injection, path traversal)
- The dtmgd version (`dtmgd version`) and the affected source files
- Any configuration needed to reproduce the issue
- Step-by-step reproduction instructions
- Proof-of-concept code, if you have it
- The impact, including how an attacker might exploit it

**Please report:** vulnerabilities in dtmgd and in the dependencies it ships.

**Please don't report:** requests for new security features, help with
security-related configuration, or issues unrelated to security.

### Response process

We acknowledge your report and keep you updated on progress. The Dynatrace
Open Source Community and the reporter agree on a public disclosure date. We
prefer to fix the issue before disclosure, which can take time depending on
complexity:

- Immediately, if the vulnerability is already public
- Within 7 days, for issues with a straightforward fix
- Several weeks, for issues that need significant code changes

Vulnerability information stays confidential among the dtmgd maintainers and
the Dynatrace Open Source Community, except where needed to fix the issue.

## Using dtmgd securely

1. **Token storage.** `dtmgd config set-credentials` stores API tokens in the
   OS keyring (macOS Keychain, Linux Secret Service, Windows Credential
   Manager); the config file keeps only an empty placeholder.
2. **Headless systems.** Containers, CI runners and SSH-only servers usually
   have no keyring. There, the token is written to the config file in
   **plaintext**, and a warning is printed. Whenever dtmgd saves the config
   file it writes it with mode 0600, including a hand-written file that was
   looser before. A file dtmgd only reads keeps its mode, so check a
   hand-written one with `ls -l` and `chmod 600` it. For CI
   prefer a `${VAR}` reference in the config (`token: ${DT_API_TOKEN}`), so the secret
   lives only in the environment, and use short-lived tokens. Run
   `dtmgd config migrate-tokens` once a keyring becomes available.
3. **Entering tokens.** Use `--token-stdin` or the interactive prompt rather
   than `--token`, which is visible in the process list and shell history.
4. **Never commit config files** (`~/.config/dtmgd/config`, `.dtmgd.yaml`)
   that contain tokens.
5. **Config discovery.** dtmgd uses a `.dtmgd.yaml` found in the working
   directory or any parent directory. A file found in a *parent* directory
   is named in a warning on every run, because anyone who can write to a
   shared parent (`/tmp`, a group project tree, a CI workspace) could plant
   one.
6. **Agent mode.** When an AI-agent environment variable such as `CLAUDECODE`
   is set, dtmgd switches to JSON envelope output and prints one stderr line
   saying so. Pass `--no-agent` to keep human-readable output, for example in
   scripts that parse tables. Outside a terminal, `dtmgd skills install` and
   `uninstall` require `--for <agent>`, so an environment variable alone
   never chooses where files are written. Requests made from inside an agent
   carry `(AI-Agent: <name>)` in their User-Agent, so cluster administrators
   can tell agent traffic apart in their access logs.
7. **Least privilege.** Give environment tokens only the read scopes dtmgd
   needs. A Cluster API token with the Service Provider API scope can also
   change the cluster, so treat it as an admin credential even though dtmgd
   only reads.
8. **Stay current.** Security fixes ship only in the latest release.
9. **Verify what you download.** Release archives are listed in a
   `checksums.txt` that the release workflow signs with cosign, and the
   install scripts check each archive against that file. See "Verifying a
   download" in the README.

## Design decisions

- **An explicit `--config <path>` is not warned about.** dtmgd loads whatever
  file you name, from any location, and only reports it under `-v`. The path
  was typed by you, and a warning on every deliberate choice would teach
  people to ignore the warning that matters: the config silently picked up
  from a parent directory (item 5 above).
