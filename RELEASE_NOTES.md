## What's new

- **Location now populates on page load** — callsign, description, and location from the AllStar node database (astdb) are now included in the favorites API response, so the table is fully populated immediately without waiting for live stats to arrive over SSE.

- **User-set values always take priority** — if you have manually set a callsign, description, or location on a favorite, it is never overridden by the astdb or the stats feed.

- **Page refresh no longer loses stats** — the on-connect stats fetch now completes even if the browser closes the SSE connection mid-request (e.g. during a page reload), so the cache stays warm.

- **mDNS fallback to system resolver** — when multicast mDNS fails to resolve a `.local` hostname (e.g. inside Docker on macOS), YAAMon now retries via the system DNS resolver. Docker Desktop on macOS routes `.local` queries through the host, so AMI connections to `.local` nodes work without manual IP configuration.

- **Inline astdb enrichment for stats** — live stats published over SSE are enriched with astdb location data server-side, so the active-links panel also shows location for nodes not in your favorites list.
