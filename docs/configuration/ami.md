# AMI Configuration

YAAMon connects to Asterisk nodes via the [Asterisk Manager Interface](https://wiki.asterisk.org/wiki/display/AST/The+Asterisk+Manager+TCP+IP+API) (AMI). Each node you add requires AMI credentials configured in Asterisk's `manager.conf`.

## Minimum manager.conf — local node (same machine)

When YAAMon runs on the same host as Asterisk:

```ini
[general]
enabled = yes
bindaddr = 127.0.0.1    ; loopback only — safest default

[yaamon]
secret = your-secret-here
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan
write = system,call,agent,user,config,command,reporting,originate
permit = 127.0.0.1/255.255.255.255
```

Set **AMI host** to `localhost` and **AMI port** to `5038` when adding the node in YAAMon.

## Minimum manager.conf — remote node (different machine)

When YAAMon is on a separate host (e.g. a Docker server), Asterisk must listen on a network interface:

```ini
[general]
enabled = yes
bindaddr = 0.0.0.0      ; or the specific interface IP

[yaamon]
secret = your-secret-here
read = system,call,log,verbose,agent,user,config,dtmf,reporting,cdr,dialplan
write = system,call,agent,user,config,command,reporting,originate
permit = 192.168.1.50/255.255.255.255   ; YAAMon host IP
deny = 0.0.0.0/0.0.0.0
```

> **Security**: AMI transmits credentials in plain text. Never expose port 5038 to the internet. See [AMI Security](../security/ami-security.md) for VPN and SSH tunnel options.

## Reload after changes

```bash
sudo asterisk -rx "module reload manager"
```

## Verify connectivity

```bash
yaamon node test <id>
```

This opens and closes an AMI connection without starting the server. The node ID comes from `yaamon node list`.

## Connection status

In the YAAMon UI:
- Green dot on a node card = live AMI connection
- Red dot = connection failed (check credentials, firewall, and that Asterisk is running)
