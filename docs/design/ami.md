# AMI Interface

YAAMon uses the Asterisk Manager Interface (AMI) to monitor link state in real time and to issue commands to the repeater controller. This document describes exactly which actions, commands, and events are used and what each one drives.

## Architecture

Two packages implement the AMI layer:

- **`internal/ami/client.go`** — low-level persistent TCP client. One instance per managed node. Handles reconnection, login, message framing, and response routing.
- **`internal/ami/manager.go`** — owns one `Client` per enabled node, fans events out to subscribers, and exposes `SendActionWait` and `Subscribe` to the rest of the server.

The server starts one `Manager` at boot. When a node is added or enabled, `Manager.Add` starts its `Client`. The client maintains a persistent connection and reconnects automatically (exponential backoff, 1 s → 60 s cap) if Asterisk restarts or the network drops.

## Connection Setup

On each (re)connection the client:

1. Dials `host:port` (default `localhost:5038`) with a 10-second timeout. `.local` hostnames are resolved via mDNS first, with fallback to the system resolver.
2. Reads the AMI banner line (e.g. `Asterisk Call Manager/1.3`).
3. Sends a **Login** action:
   ```
   Action: Login
   ActionID: auth-1
   Username: <ami_user>
   Secret: <ami_pass>
   ```
4. Reads the response block. On `Response: Success` / `Message: Authentication accepted` it marks the connection live. On `Response: Error` it closes and retries.

The `TestConnection` helper (used by the Admin → Add/Edit Node dialog) performs steps 1–4 on a one-shot connection and returns the result to the user without joining the persistent session.

## AMI Wire Format

AMI messages are blocks of `Key: Value` lines terminated by a blank line (`\r\n\r\n`). YAAMon's parser handles two output formats emitted by Asterisk's `Command` response:

- **Asterisk 12+**: individual `Output: <line>` headers, one per output line. YAAMon accumulates these into a single multi-line string.
- **Older Asterisk / `Response: Follows`**: raw lines without a key prefix, terminated by `--END COMMAND--`. YAAMon accumulates these the same way.

All outgoing actions write the `Action` header first (required by the AMI protocol spec), then remaining headers in arbitrary order.

## ActionID Routing

Every `SendActionWait` call stamps the outgoing action with a unique `ActionID` (`ya-<nanosecond-timestamp>`). The read loop routes incoming response blocks that carry a matching `ActionID` directly to the waiting caller's channel. Events without an `ActionID`, or with an `ActionID` that has no registered waiter, are forwarded to the shared event channel. The response timeout is 5 seconds for all callers.

## Actions Sent

YAAMon uses exactly one AMI action type in normal operation: `Command`. Login is handled internally by the client on every connection.

### `rpt show variables <nodeNumber>`

**Used by:** `pollNodeLinksInner` (`internal/server/links.go`)
**Frequency:** Every 5 seconds (background poller), plus immediately on `NodeConn`, `NodeDisconn`, and `Hangup` events.

```
Action: Command
Command: rpt show variables <nodeNumber>
```

The `Output` field of the response is scanned for two variables:

**`RPT_ALINKS`** — comma-separated list of currently connected nodes.

Format: `<count>,<ident><typechar><keyedchar>[,<ident><typechar><keyedchar>...]`

| Field | Values | Meaning |
|---|---|---|
| `count` | integer | Number of links |
| `ident` | node number or callsign | Node number for AllStar links; callsign for direct/IAXRPT/web clients |
| `typechar` | `T` `M` `L` `P` | Transceive / Monitor / Local monitor / Permanent transceive |
| `keyedchar` | `K` `U` | Keyed (transmitting to us) / Unkeyed (idle) |

Example: `3,41522TU,667342TK,KR4YXXLU`

**`RPT_TXKEYED`** — home node transmitter state.

Format: `0` or `1`. `1` means the home node's transmitter is currently keyed.

Both variables are parsed by `parseRPTALinks` and `parseRPTTXKeyed` in `links.go`. When the parsed link map differs from the cached state, an SSE `links` event is pushed to all connected dashboard clients.

### `rpt cmd <nodeNumber> ilink <function> [target]`

**Used by:** `handleConnect` and `handleDisconnect` (`internal/server/stats.go`)
**Triggered by:** user action in the dashboard Links panel.

```
Action: Command
Command: rpt cmd <nodeNumber> ilink <function> [<target>]
```

YAAMon uses the following `ilink` function codes:

| Code | Direction | Meaning |
|---|---|---|
| `1` | disconnect | Disconnect one node (temporary link) |
| `2` | connect | Monitor (one-way receive) |
| `3` | connect | Transceive (default; bidirectional) |
| `6` | disconnect | Disconnect all nodes |
| `8` | connect | Local monitor |
| `11` | disconnect | Disconnect one node (permanent link) |
| `12` | connect | Monitor permanent |
| `13` | connect | Transceive permanent |
| `18` | connect | Local monitor permanent |

When `exclusive` connect is requested the server first sends `ilink 6` (if there are active links), waits 500 ms, then sends the connect command.

### User-configured commands (Functions menu)

**Used by:** `handleAPIRunCommand` (`internal/server/commands.go`)
**Triggered by:** user clicking an item in the Functions dropdown.

```
Action: Command
Command: <resolved command string>
```

The command template comes from `config.yaml` (or the built-in defaults listed below). `{node}` is substituted server-side with the node's AllStar node number before dispatch. Additional `{argname}` placeholders are filled from user input collected by the args modal. Any unresolved placeholder causes a 400 error before the command reaches AMI.

**Default command set** (used when `commands:` is absent from `config.yaml`):

| Name | Command template | Min. role |
|---|---|---|
| Node Time | `rpt cmd {node} status 12 xxx` | readwrite |
| Node ID | `rpt cmd {node} status 11 xxx` | readwrite |
| Reconnect | `rpt cmd {node} ilink 16` | readwrite |
| Node Status | `rpt stats {node}` | readwrite |
| Link Status | `rpt lstats {node}` | readwrite |
| IAX Registry | `iax2 show registry` | readwrite |
| IAX Channels | `iax2 show channels` | readwrite |
| Network Status | `iax2 show netstats` | readwrite |
| Uptime | `core show uptime` | readwrite |

The `Output` field of the response is returned to the browser as plain text and displayed in a toast notification. A special Asterisk message `"Command output follows but no following output"` is treated as success with empty output (the command ran but produced no console output).

## Events Consumed

The server's event listener (`startAMIEventListener`, `internal/server/events.go`) subscribes to all events from all managed nodes via `Manager.Subscribe()`. Only the following event types trigger server-side behaviour; all others are silently discarded.

| Event type | Fields read | Action |
|---|---|---|
| `NodeConn` | `Channel` (logged) | Re-poll `rpt show variables` immediately |
| `NodeDisconn` | `Channel` (logged) | Re-poll `rpt show variables` immediately |
| `FullyBooted` | — | Re-poll `rpt show variables` (Asterisk restart recovery) |
| `Hangup` | — | Re-poll `rpt show variables` (possible link drop) |
| `DTMF` | `Digit`, `Channel` | Publish `{type:"dtmf", digit, channel}` SSE event to dashboard |
| `DTMFBegin` | `Digit`, `Channel` | Same as `DTMF` |

The `NodeConn` / `NodeDisconn` / `Hangup` path is a latency optimisation: rather than waiting up to 5 seconds for the next poll tick, the link list is refreshed immediately when Asterisk signals a change.

`DTMF` and `DTMFBegin` events are forwarded to SSE subscribers so the dashboard can display a brief digit flash animation without polling.

## Event Channel Capacity

Each `Client` has an internal event channel of 128 entries. The `Manager`'s fan-out subscriber channels hold 256 entries each. Both are non-blocking sends: if a consumer falls behind, events are dropped rather than blocking the read loop. Under normal load neither limit is approached.

## Required manager.conf Permissions

The AMI user configured in YAAMon needs the following `manager.conf` permissions:

```ini
read  = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan
write = system,call,agent,user,config,command,reporting,originate
```

`write = command` is what allows `Action: Command` to execute CLI commands. Without it every command call returns an AMI auth error.
