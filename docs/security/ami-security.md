# AMI Security

The Asterisk Manager Interface (AMI) transmits credentials and commands in **plain text**. Never expose AMI port 5038 directly to the internet.

## Local node (same machine)

When YAAMon and Asterisk run on the same host, bind AMI to loopback only:

```ini
[general]
enabled = yes
bindaddr = 127.0.0.1
```

This prevents any external access to the AMI port without a firewall rule.

## Nodes on the same LAN

Plain AMI over a trusted LAN is acceptable for many home setups. Restrict the `permit` line to the YAAMon host's IP:

```ini
[yaamon]
secret = your-secret-here
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan
write = system,call,agent,user,config,command,reporting,originate
permit = 192.168.1.50/255.255.255.255   ; YAAMon host IP only
deny = 0.0.0.0/0.0.0.0
```

> AMI over TLS is planned for a future release — see [issue #10](https://github.com/jchonig/allstar-yaamon/issues/10).

## Internet-connected remote nodes

**Never expose port 5038 to the internet.** For nodes on different networks, secure the AMI connection with a tunnel.

### Option A — VPN (recommended)

Put the YAAMon host and the remote node on the same VPN (WireGuard or OpenVPN). Use the remote node's VPN IP address as the AMI host in YAAMon. No inbound firewall holes are needed on the remote node's public interface, and all traffic is encrypted.

### Option B — SSH tunnel

On the YAAMon host, open a persistent tunnel to the remote node:

```bash
ssh -N -L 5038:localhost:5038 youruser@remote-node-ip
```

Set the AMI host to `127.0.0.1` and port `5038` in YAAMon. The tunnel forwards the local port to the remote Asterisk over an encrypted SSH connection.

For a persistent tunnel, use `autossh`:

```bash
autossh -M 0 -N -L 5038:localhost:5038 youruser@remote-node-ip
```

Or configure it as a systemd service:

```ini
# /etc/systemd/system/yaamon-tunnel.service
[Unit]
Description=AMI tunnel to remote node
After=network.target

[Service]
ExecStart=/usr/bin/autossh -M 0 -N -o ServerAliveInterval=30 \
  -L 5038:localhost:5038 youruser@remote-node-ip
Restart=always

[Install]
WantedBy=multi-user.target
```

### manager.conf on the remote node

Restrict `permit` to the YAAMon host's VPN or tunnel address:

```ini
[yaamon]
secret = your-secret-here
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan
write = system,call,agent,user,config,command,reporting,originate
permit = 10.0.0.2/255.255.255.255    ; YAAMon host VPN address
deny = 0.0.0.0/0.0.0.0
```

```bash
sudo asterisk -rx "module reload manager"
```
