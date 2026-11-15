# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in `ethernetipctl`, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email **security@otfabric.io**, or open a private advisory on GitHub (the [Security](https://github.com/otfabric/ethernetipctl/security) tab → **Advisories** → **Report a vulnerability**), with:

- A description of the vulnerability
- Steps to reproduce the issue
- The affected version(s) (`ethernetipctl version` output)
- Any potential impact you've identified

We will acknowledge your report within 48 hours and aim to provide a fix or mitigation within 7 days for critical issues.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | No |

## Security Considerations

### EtherNet/IP Has No Built-In Security

EtherNet/IP and CIP as most devices implement them carry no authentication and no encryption: anyone who can reach port 44818 of a device can read it, write its attributes, open I/O connections and reset it, and anyone on the path can read or alter the traffic. `ethernetipctl` does not change that. (CIP Security, EtherNet/IP over TLS on port 2221, is supported by go-ethernetip but not yet by ethernetipctl.)

- **Use it only on devices and networks you own or are authorized to test**
- **Keep control networks isolated and trusted**; never expose ports 44818 and 2222 to the Internet

### Commands That Change a Device

These need `--yes`. Without it they validate the invocation, open no connection, send nothing and exit 2 (`confirmation_required`). `--yes` cannot be set from the environment, so that a variable left in a shell cannot turn a read into a write.

- **`write`** sets an attribute. Attributes of the TCP/IP Interface object change the device's IP configuration (and can make it unreachable); attributes of application objects change what the device does
- **`assembly write`** writes assembly data: on an output assembly, the device's outputs
- **`reset`** restarts the device; `--type 1` also returns it to its factory defaults, its IP configuration included
- **`tag write`** changes a Logix controller's data, which its program acts on; a bit name (`Flags.3`) changes one bit with Read-Modify-Write
- **`io` with output data** (`--output-size` above 0) sends output data at every RPI: zeros until `--output-data` is applied (go-ethernetip's scanner starts the exchange before the application can set its first output), in run mode unless `--idle`. On real equipment this drives outputs. Watch inputs with an input-only connection instead (`--output` the heartbeat point, `--output-size 0`), which writes nothing
- **`forward-open` and `io` with `--config-data`** send the data in the connection path, and the device writes it to its configuration assembly (`--config`) when it accepts the connection. Without `--config-data` nothing is written to the configuration assembly

These commands take a route only from `--route`, never from `ETHERNETIPCTL_ROUTE`, so that a route left in a shell cannot send a change to another device. With the variable set and no `--route`, they send the change to the host on the command line and say on stderr that the variable is not used; `reset` names its route in its output, as the other commands do. The reading commands go on taking the variable.

Requests are never repeated: after a timeout (exit 5) a write may or may not have been executed. Read back before writing again.

### Commands That Are Read-Only but Not Invisible

- **`forward-open`** without `--config-data` opens a connection (and closes it again unless `--keep`). An exclusive-owner connection to an output assembly takes ownership of it while open: a PLC that owns it gets its own Forward_Open refused, or the device refuses yours (0x0106 ownership conflict). With `--keep` the connection stays until the device times it out or `forward-close` closes it
- **`io`** without output data and without `--config-data` opens an input-only or listen-only connection, which counts against the device's connection limit
- **`session --hold`** keeps a session, and with `--connected` a Class 3 connection, for as long as asked
- **`discover`** sends one ListIdentity datagram. To a broadcast address it reaches every device of the segment, and each answers. It never scans: prefixes, ranges and lists are refused everywhere
- Every command opens a TCP session, which a device may log or count

### Running the Adapter

- `adapter` listens on **all network interfaces** by default and accepts **any client**. Use `--listen 127.0.0.1` (or one interface address) for local tests, or a host firewall
- The adapter accepts every write, I/O connection and Reset it is sent; Reset does nothing but log an event
- A model file is read with the permissions of the user running the adapter. A model holds at most 4096 assemblies, 4096 tags and 4096 templates, and 64 MiB of assembly and tag data together; a larger one is refused (exit 3) before anything is built
- Up to 1024 events wait for stdout. While stdout does not keep up, the adapter goes on serving, the events beyond those are left out, and a `dropped` event counts them

### Output

- Command output and adapter event logs contain process data, addresses and identities, which may be sensitive operational data. Be careful when storing or sharing them
- Text and table output write what a device sends (names, product names, string values) with the characters that do not print escaped (`\x1b`, `\r`); JSON escapes them as JSON does
- `io` keeps up to 1024 samples (4 MiB of data as hex) waiting for stdout; when stdout is slower, the oldest are dropped and the summary counts them
- `--debug` prints go-ethernetip's protocol log to stderr

### Privileges

`ethernetipctl` needs no elevated privileges. The adapter on port 44818 and the I/O endpoint on UDP 2222 work as a normal user (ports above 1024). `discover` to a broadcast address needs the network to allow broadcasts, not root.

### Verifying Downloads

Release archives are published with SHA-256 checksums. The installer in the [Install section](README.md#install) verifies the checksum before it installs anything.
