# Node Commands (Functions Menu)

The **Functions** button in the dashboard toolbar opens a dropdown of node commands. Commands are sent to Asterisk via the AMI `Command` action.

## Default commands

When no `commands:` section is present in `config.yaml`, the following commands are available to `readwrite` users and above. Commands that produce output open a modal; commands that produce no output (announcements, control) show a "Command sent" toast.

| Group | Name | AMI command |
|-------|------|-------------|
| Announce | Say Time of Day | `rpt cmd {node} status 12 xxx` |
| Announce | Force ID | `rpt cmd {node} status 11 xxx` |
| Link | Reconnect | `rpt cmd {node} ilink 16` |
| Status | Show Node Status | `rpt stats {node}` |
| Status | Show Link Status | `rpt lstats {node}` |
| Status | Show IAX Registry | `iax2 show registry` |
| Status | Show IAX Channels | `iax2 show channels` |
| Status | Show Network Status | `iax2 show netstats` |
| Status | Show Uptime | `core show uptime` |

> **Reconnect** restores links previously disconnected with "disconnect all" (`ilink 6`). Asterisk/app_rpt tracks this state internally.
>
> Commands with `xxx` as the last token are app_rpt `status`/`cop` actions that require a dummy trailing argument.

## Fields

| Field | Description |
|-------|-------------|
| `name` | Label shown in the dropdown |
| `cmd` | AMI command template. `{node}` is always replaced server-side with the node's number. |
| `role` | Minimum role required: `readonly`, `readwrite`, `admin`, or `superuser` |
| `group` | Optional grouping key. Commands with the same value are placed together; a labelled divider appears in the menu when the group changes. The group name is capitalised automatically in the UI. |
| `args` | Optional list of user-supplied arguments (see below) |

### Argument fields

| Field | Description |
|-------|-------------|
| `name` | Placeholder key used in `cmd` (e.g. `target` matches `{target}`) |
| `label` | Human-readable label shown in the UI input |
| `type` | `node_number` (numeric keyboard on mobile) or `string` |

## Configuration example

```yaml
commands:
  commands:
    # Announcements (group: announce)
    - name:  "Say Time of Day"
      cmd:   "rpt cmd {node} status 12 xxx"
      role:  readwrite
      group: announce
    - name:  "Force ID"
      cmd:   "rpt cmd {node} status 11 xxx"
      role:  readwrite
      group: announce

    # Linking (group: link)
    - name:  "Reconnect"
      cmd:   "rpt cmd {node} ilink 16"
      role:  readwrite
      group: link
    - name:  "Link to Node"
      cmd:   "rpt cmd {node} ilink 3 {target}"
      role:  readwrite
      group: link
      args:
        - name:  target
          label: "Target Node Number"
          type:  node_number

    # Status / information (group: status)
    - name:  "Show Node Status"
      cmd:   "rpt stats {node}"
      role:  readwrite
      group: status
    - name:  "Show Link Status"
      cmd:   "rpt lstats {node}"
      role:  readwrite
      group: status
    - name:  "Show IAX Registry"
      cmd:   "iax2 show registry"
      role:  readwrite
      group: status
    - name:  "Show Uptime"
      cmd:   "core show uptime"
      role:  readwrite
      group: status

    # Telemetry (group: telemetry)
    - name:  "Local Telemetry Enable"
      cmd:   "rpt cmd {node} cop 33 xxx"
      role:  readwrite
      group: telemetry
    - name:  "Local Telemetry Disable"
      cmd:   "rpt cmd {node} cop 34 xxx"
      role:  readwrite
      group: telemetry
    - name:  "Local Telemetry On Demand"
      cmd:   "rpt cmd {node} cop 35 xxx"
      role:  readwrite
      group: telemetry

    # System control — admin only (group: system)
    - name:  "Enable System"
      cmd:   "rpt cmd {node} cop 2 xxx"
      role:  admin
      group: system
    - name:  "Disable System"
      cmd:   "rpt cmd {node} cop 3 xxx"
      role:  admin
      group: system
    - name:  "Enable Linking"
      cmd:   "rpt cmd {node} cop 11 xxx"
      role:  admin
      group: system
    - name:  "Disable Linking"
      cmd:   "rpt cmd {node} cop 12 xxx"
      role:  admin
      group: system
    - name:  "Toggle Test Tone"
      cmd:   "rpt cmd {node} cop 4 1"
      role:  admin
      group: system
    - name:  "System State 0 (normal)"
      cmd:   "rpt cmd {node} cop 14 0"
      role:  admin
      group: system

    # Database — admin only (group: database)
    - name:  "Show Node Allowlist"
      cmd:   "database show allowlist/{node}"
      role:  admin
      group: database
    - name:  "Show Node Denylist"
      cmd:   "database show denylist/{node}"
      role:  admin
      group: database
    - name:  "Add to Allowlist"
      cmd:   'database put allowlist/{node}/{target} "Reason"'
      role:  admin
      group: database
      args:
        - name:  target
          label: "Target Node Number"
          type:  node_number
```

## Security model

Commands are stored entirely server-side. The client never sends raw command text — it sends the **index** of the command in the config list plus a **check string** (`SHA-256("{index}:{cmd_template}")[:16]`). The server:

1. Validates the index is in range.
2. Recomputes the check string and rejects mismatches (detects stale clients after a config reload).
3. Verifies the requesting user's session permission meets the command's `role`.
4. Substitutes `{node}` with the node's number from the database.
5. Substitutes any user-supplied `{arg}` values.
6. Rejects the request if any `{placeholder}` remains after substitution.

The **Functions button is disabled** when AMI is disconnected, so commands are never sent to an unreachable node.
