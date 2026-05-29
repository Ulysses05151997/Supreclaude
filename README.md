# Mederos & Associates — Client Records (CRM)

A simple, self-contained client database for the firm. It replaces the paper
notebook of names and phone numbers with a searchable list where you click a
client to see their full details and browse the documents in their case-file
folder on the server.

- **Runs anywhere the staff already have a browser** — Windows, macOS, Linux.
  There is nothing to install on the office computers; they just open a
  bookmark like `http://192.168.1.50:8080`.
- **One central database** lives on the office server (the headless Dell tower)
  right next to the case files. A single program file (`crm.exe`) is the whole
  application — templates, styles, and the database engine are baked in.
- **Per-user logins** protect the confidential client information, with an
  activity log of who did what.

It is built to grow: calendaring, intake parsing, and document features can be
added later without rebuilding the foundation.

---

## What it does today

- Add, edit, and search clients (name, address, phone, email, birthday, type of
  case, adversary, notes).
- Click a client name to open their full record.
- Link each client to a folder of case files on the server and **browse and
  download those files right in the browser** (no need to hunt through the
  server's file shares).
- Staff accounts with admin/staff roles; first-run setup creates the first
  administrator.
- Activity log and safe, soft-delete of clients (records are never truly lost).

---

## For the office: installing on the server (Windows)

You only do this once, on the Dell tower where the case files live.

1. Create a folder `C:\CRM`. Copy these three files into it:
   - `crm.exe`
   - `config.toml` (copy `config.example.toml` and rename it)
   - `install-windows.bat`
2. Open `config.toml` in Notepad. Check two lines:
   - `case_files_root` — set this to the folder that holds the clients' case
     files (e.g. `C:\CaseFiles`). Use a **local path** or a `\\SERVER\Share`
     path — never a mapped drive letter like `Z:\`.
   - `listen_addr` — leave as `0.0.0.0:8080` unless port 8080 is taken.
   Save and close.
3. Right-click `install-windows.bat` and choose **Run as administrator**. This
   installs the program as a Windows service that starts automatically on
   reboot, opens the firewall for the port, and starts it.
4. On the tower, open a browser to **http://localhost:8080**. You'll see a
   one-time **Welcome** screen — create the administrator account here.
5. Find the tower's network address: open Command Prompt, type `ipconfig`, and
   note the **IPv4 Address** (e.g. `192.168.1.50`). Ask whoever set up your
   router to give the tower a fixed/reserved address so this never changes.
6. On each office computer (Windows, Mac, Linux), open
   `http://<that-address>:8080` and bookmark it. Sign in.

### Adding staff
Sign in as the administrator → **Staff** → create accounts. Each new user gets
a temporary password and is asked to choose their own the first time they sign
in.

### Backups
Run `backup.bat` any time (it's safe while the program is running) — it writes a
timestamped copy of the database next to `crm.db`. Schedule it nightly with
Windows Task Scheduler, and **keep copies off the machine** (USB stick or a
cloud folder). To restore, stop the service and replace `crm.db` with a backup.

### Updating
Stop the service (Services app, or `crm.exe stop`), replace `crm.exe` with the
new version, start it again. Any database changes are applied automatically.

---

## Turning on encryption (recommended for client data)

By default the program serves plain `http://` on the office network, which is
fine for a trusted, wired LAN but sends data unencrypted. To encrypt traffic
(`https://`) without paying for a public domain, generate a self-signed
certificate and trust it on the few office machines:

1. On the server, create a certificate and key (one-time). With OpenSSL:
   ```
   openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
     -keyout C:\CRM\key.pem -out C:\CRM\cert.pem \
     -subj "/CN=mederos-crm" \
     -addext "subjectAltName=IP:192.168.1.50,DNS:mederos-crm"
   ```
   (Replace the IP with the tower's address.)
2. In `config.toml`, uncomment and set `tls_cert` and `tls_key`. Restart.
3. Install `cert.pem` as a trusted certificate on each office computer so the
   browser doesn't warn:
   - **Windows:** double-click `cert.pem` → Install Certificate → Local Machine
     → "Trusted Root Certification Authorities".
   - **macOS:** open in Keychain Access → System → set to "Always Trust".
   - **Linux (Arch):** copy to `/etc/ca-certificates/trust-source/anchors/` and
     run `sudo trust extract-compat` (and import into the browser if needed).

The bookmark becomes `https://<address>:8080`.

---

## For developers

Go 1.24+ is required (uses `os.Root` for safe file serving). No CGO, no npm.

```bash
# Run locally with a dev config
go run ./cmd/crm -config dev.config.toml run

# Tests (includes path-traversal security tests)
go test ./...

# Cross-compile release binaries into ./dist
./build.sh
```

### Project layout
```
cmd/crm/            entrypoint + subcommands (run/install/backup/...)
internal/config/    config file + env loading
internal/db/        SQLite open, pragmas, embedded migrations, backup
internal/models/    clients, users, sessions, audit (database/sql, hand-written SQL)
internal/auth/      bcrypt, server-side sessions, middleware, CSRF
internal/files/     path-traversal-safe case-file browsing/download (os.Root)
internal/web/       routes, html/template + htmx handlers, embedded assets
internal/service/   OS-service wrapper (auto-start) via kardianos/service
```

### Subcommands
| Command | Purpose |
|---|---|
| `crm run` | Start the server (default) |
| `crm install` / `uninstall` | Register/remove the auto-start OS service (run as admin) |
| `crm start` / `stop` | Control the installed service |
| `crm backup [dest]` | Write a consistent DB snapshot (`VACUUM INTO`) |
| `crm version` | Print the version |

### Design notes
- **Database:** SQLite in WAL mode with a single writer connection — no separate
  database server, robust for a handful of concurrent office users.
- **File safety:** the configured `case_files_root` is opened with `os.Root`;
  every user-supplied path is validated and contained to the client's folder.
  See `internal/files/files_test.go` for the traversal test matrix.
- **Sessions:** stored server-side so they can be revoked instantly (logout,
  password reset, deactivation).
- **No data in git:** `config.toml`, `*.db`, logs, backups, and certs are
  git-ignored. Never commit client data.
