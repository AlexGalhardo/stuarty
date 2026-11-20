# deploy/: Stuarty on your own server

Stuarty and Caddy on one Linux host with Docker Compose, HTTPS included. The full guide is the page
"Install on a server" of the documentation (`site/content/docs/en/self-hosting/vps.mdx`); the
specification is SDD-009 section 21. A Portuguese version is in [README.pt-BR.md](README.pt-BR.md).

```console
git clone --branch vX.Y.Z https://github.com/AlexGalhardo/stuarty.git && cd stuarty
bash deploy/install.sh --domain stuarty.example.com --email you@example.com
bash deploy/backup.sh
bash deploy/update.sh X.Y.Z
bash deploy/restore.sh deploy/backups/stuarty-backup-<time>.tar
```

| File | What it is |
| --- | --- |
| `compose.yml` | the two containers, the volumes and the secrets |
| `Caddyfile` | TLS and the reverse proxy |
| `.env.example` | the variables; `install.sh` writes the real `.env` |
| `install.sh` | checks Docker, makes the keys, builds, starts, waits for HTTPS (safe to repeat) |
| `update.sh` | backup, build, start, health check, rollback |
| `backup.sh`, `restore.sh` | one archive with a checksum manifest; never the master key |
| `test/` | the tests of the scripts: `bash deploy/test/install-test.sh` and the two next to it |

The master key is `deploy/secrets/master_key`. A backup does not contain it: keep a copy elsewhere.
