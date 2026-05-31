# Callsign Lookup

YAAMon enriches node listings with callsign data from multiple sources and displays it in connection list tooltips and favorite buttons.

## Data sources

| Source | Coverage | Cost | Notes |
|--------|----------|------|-------|
| AllStarLink node database (astdb) | AllStar nodes | Free | Downloaded automatically; used for node number → callsign mapping |
| [callook.info](https://callook.info) | US callsigns | Free | REST API; used for QRZ-style license data |
| [QRZ.com](https://www.qrz.com) | US and international | Subscription required | XML API; richer data; requires QRZ.com credentials |

## Lookup behaviour

When a callsign tooltip is triggered on the dashboard:

1. **ASL stats** are fetched for the node number from the AllStarLink stats API.
2. The callsign from ASL is used to look up the QRZ or callook record.
3. If the favorite has a manually configured callsign that differs from the ASL callsign, **both** are looked up and displayed — the configured one first, the ASL one second.
4. If both lookups resolve to the same callsign (e.g. "ECR" and "W2ECR" both return "W2ECR"), the result is de-duplicated and shown once.

## Caching

- **astdb**: Downloaded on startup and refreshed hourly. Stored in the file at `astdb.path`.
- **callook.info / QRZ.com**: Records are cached in the database for 30 days. The cache is pre-seeded on startup so lookups are fast without a network round-trip.
- **ASL stats**: Cached in memory and the database; updated every few minutes per node.

## Configuring the lookup source

Each user can choose their preferred lookup source in **My Profile → Callsign Lookup**:

| Option | Behaviour |
|--------|-----------|
| **Automatic** | Uses QRZ.com when credentials are configured, otherwise callook.info |
| **callook.info** | Always use callook.info (US only, no credentials needed) |
| **QRZ.com** | Always use QRZ.com (requires subscription credentials) |

## Configuring QRZ.com credentials

In **My Profile → Callsign Lookup → QRZ.com Credentials**:

1. Enter your QRZ.com username and password.
2. Click **Save QRZ**.
3. Click **Clear cache** if you want to force a fresh lookup of previously cached records.
4. Click **Remove** to clear stored credentials.

QRZ passwords are stored AES-256-GCM encrypted, keyed from the session secret. They are never stored in plain text.

## Forcing a cache refresh

- Per-user: **My Profile → Callsign Lookup → Clear cache** clears your personal QRZ cache.
- Admin: **Admin → (gear icon)** → **Clear QRZ Cache** clears the shared lookup cache for all users.
