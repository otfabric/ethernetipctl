# Command and flag reference

Every command of `ethernetipctl` with its own flags, taken from `ethernetipctl <command> --help`. The global flags (`--format`, `--timeout`, `--port`, `--debug`) work with every command and are listed once, under the root; `-h`, `--help` prints the help of any command. The [README](../README.md#usage) explains what the commands do; [examples.md](examples.md) shows real runs.

## `ethernetipctl` (global flags)

Usage: `ethernetipctl [command]`

| Flag | Short | Meaning |
|---|---|---|
| `--debug` |  | Log go-ethernetip's protocol trace to stderr, and show the causes of an error |
| `--format string` |  | Output format: text, json, or table [env: ETHERNETIPCTL_FORMAT] (default "text") |
| `--port uint16` |  | Port of the target when the host names none (44818) [env: ETHERNETIPCTL_PORT] (default 44818) |
| `--timeout duration` |  | Timeout of connecting and of each request (e.g. 500ms, 5s) [env: ETHERNETIPCTL_TIMEOUT] (default 5s) |

Subcommands: `adapter`, `assembly`, `completion`, `connections`, `diagnose`, `discover`, `eds`, `forward-close`, `forward-open`, `help`, `identify`, `interfaces`, `io`, `object`, `objects`, `read`, `reset`, `services`, `session`, `status`, `tag`, `version`, `write`

## `ethernetipctl adapter`

Aliases: `server`

Usage: `ethernetipctl adapter [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--assembly strings` |  | Assembly as instance:kind:size, kinds input, output, config, input-only, listen-only (replaces the default set) |
| `--config string` |  | Adapter model file (YAML or JSON); without it the flags describe the adapter [env: ETHERNETIPCTL_ADAPTER_CONFIG] |
| `--duration duration` |  | Stop after this long (0 = until interrupted) |
| `--echo` |  | Copy the data written to each output assembly to the input assembly of the same size with the lowest instance |
| `--io-address string` |  | UDP address of implicit (I/O) messaging (default: port 2222 on the listen host) [env: ETHERNETIPCTL_IO_ADDRESS] |
| `--listen string` |  | TCP address to listen on, host[:port] (port: --port); ListIdentity is also answered on UDP at the same port [env: ETHERNETIPCTL_LISTEN] |
| `--logix` |  | Emulate a Logix tag database with a few demo tags (Rockwell, vendor specific) |
| `--product-code uint16` |  | Product code of the identity |
| `--product-name string` |  | Product name of the identity |
| `--requests` |  | Also log every explicit request as an event |
| `--revision string` |  | Revision of the identity, major.minor |
| `--serial uint32` |  | Serial number of the identity |
| `--vendor-id uint16` |  | Vendor ID of the identity |

## `ethernetipctl assembly`

Usage: `ethernetipctl assembly [flags]`

No flags of its own.

Subcommands: `read`, `write`

## `ethernetipctl assembly read`

Usage: `ethernetipctl assembly read <host[:port]> <instance> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl assembly write`

Usage: `ethernetipctl assembly write <host[:port]> <instance> --data HEX --yes [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--data string` |  | Data to write as hex octets (assembly write) |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |
| `--yes` |  | Confirm the write: without it nothing is sent (assembly write) |

## `ethernetipctl completion`

Usage: `ethernetipctl completion [bash|zsh|fish|powershell] [flags]`

No flags of its own.

## `ethernetipctl connections`

Usage: `ethernetipctl connections <host[:port]> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl diagnose`

Usage: `ethernetipctl diagnose <host[:port]> [flags]`

No flags of its own.

## `ethernetipctl discover`

Usage: `ethernetipctl discover --address <broadcast or host> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--address string` |  | Where the one ListIdentity request goes: a broadcast address (255.255.255.255, 192.168.1.255) or one device, with an optional :port [env: ETHERNETIPCTL_DISCOVER_ADDRESS] |
| `--local string` |  | Local IPv4 address to send from, which picks the interface |
| `--max-delay duration` |  | Largest delay a device may add to its reply (0 = the device's default) |
| `--max-devices int` |  | Stop after this many replies (0 = 1024) |
| `--wait duration` |  | How long replies are collected |

## `ethernetipctl eds`

Usage: `ethernetipctl eds [flags]`

No flags of its own.

Subcommands: `codegen`, `generate`, `inspect`, `validate`

## `ethernetipctl eds codegen`

Usage: `ethernetipctl eds codegen <file.eds> --package NAME [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--force` |  | Overwrite an existing --output file |
| `--output string` |  | File to write (- = stdout) (default "-") |
| `--package string` |  | Package name of the generated Go file (default "device") |

## `ethernetipctl eds generate`

Usage: `ethernetipctl eds generate (--from-device <host> | --vendor-id N --product-name NAME ...) [--assembly ...] [--connection ...] [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--assembly strings` |  | Assembly as instance:size[:name], repeatable (100:32:Input) |
| `--connection strings` |  | I/O connection as config:output:input:output-size:input-size[:rpi[:name]], repeatable (151:150:100:32:32:10ms:Exclusive Owner) |
| `--date string` |  | Creation date of the file, YYYY-MM-DD or RFC 3339 (default: now) |
| `--description string` |  | Description of the file (DescText) |
| `--device-type uint16` |  | Device type (12 = communications adapter) (default 12) |
| `--force` |  | Overwrite an existing --output file |
| `--from-device string` |  | Read the identity from this device (host[:port]) instead of the identity flags |
| `--output string` |  | File to write (- = stdout) (default "-") |
| `--product-code uint16` |  | Product code |
| `--product-name string` |  | Product name |
| `--revision string` |  | Major.minor revision (default "1.1") |
| `--vendor-id uint16` |  | Vendor ID |
| `--vendor-name string` |  | Vendor name (default: ODVA's name of --vendor-id) |

## `ethernetipctl eds inspect`

Usage: `ethernetipctl eds inspect <file.eds> [flags]`

No flags of its own.

## `ethernetipctl eds validate`

Usage: `ethernetipctl eds validate <file.eds> [flags]`

No flags of its own.

## `ethernetipctl forward-close`

Usage: `ethernetipctl forward-close <host[:port]> --connection-serial N --originator-serial N --output N --input N [--config N] [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--config uint32` |  | Configuration assembly instance (0 = none; sent as instance 0 in the path) |
| `--connection-serial uint16` |  | Connection serial number (0 = random) |
| `--input uint32` |  | T->O connection point: the input assembly |
| `--originator-serial uint32` |  | Originator serial number (0 = random) |
| `--originator-vendor uint16` |  | Originator vendor ID (0 = 0xFFFF, unregistered) |
| `--output uint32` |  | O->T connection point: the output assembly (or a heartbeat point for input-only and listen-only connections) |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl forward-open`

Usage: `ethernetipctl forward-open <host[:port]> --output N --input N --rpi D [--config N] [--output-size N] [--input-size N] [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--config uint32` |  | Configuration assembly instance (0 = none; sent as instance 0 in the path) |
| `--config-data string` |  | Configuration data sent in the connection path, as hex (an even number of octets) |
| `--connection-serial uint16` |  | Connection serial number (0 = random) |
| `--input uint32` |  | T->O connection point: the input assembly |
| `--input-size uint16` |  | Size of the input (T->O) data in octets, without sequence count |
| `--keep` |  | Leave the connection open instead of closing it with Forward_Close: the target times it out, or forward-close closes it |
| `--large` |  | Use Large_Forward_Open (chosen anyway for sizes above 511 octets) |
| `--multicast` |  | Ask for the input (T->O) data by multicast |
| `--originator-serial uint32` |  | Originator serial number (0 = random) |
| `--originator-vendor uint16` |  | Originator vendor ID (0 = 0xFFFF, unregistered) |
| `--output uint32` |  | O->T connection point: the output assembly (or a heartbeat point for input-only and listen-only connections) |
| `--output-size uint16` |  | Size of the output (O->T) data in octets, without sequence count and run/idle header; 0 for a heartbeat |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |
| `--rpi duration` |  | Requested packet interval of both directions, e.g. 10ms |
| `--timeout-multiplier uint8` |  | The connection times out after RPI x 4 << multiplier (0-7) |

## `ethernetipctl help`

Usage: `ethernetipctl help [command] [flags]`

No flags of its own.

## `ethernetipctl identify`

Usage: `ethernetipctl identify <host[:port]> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl interfaces`

Usage: `ethernetipctl interfaces <host[:port]> [flags]`

No flags of its own.

## `ethernetipctl io`

Usage: `ethernetipctl io <host[:port]> --output N --input N --rpi D [--output-size N] [--input-size N] [--duration D] [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--changes` |  | Only write a sample when the input data or run state changes |
| `--config uint32` |  | Configuration assembly instance (0 = none; sent as instance 0 in the path) |
| `--config-data string` |  | Configuration data sent in the connection path, as hex (an even number of octets) |
| `--count uint32` |  | Stop after this many input samples (0 = until interrupted) |
| `--duration duration` |  | Stop after this long (0 = until interrupted) |
| `--idle` |  | Send the output in idle instead of run |
| `--input uint32` |  | T->O connection point: the input assembly |
| `--input-size uint16` |  | Size of the input (T->O) data in octets, without sequence count |
| `--io-address string` |  | Local UDP address of the I/O endpoint; a port other than 2222 is announced to the target in Forward_Open [env: ETHERNETIPCTL_IO_ADDRESS] |
| `--large` |  | Use Large_Forward_Open (chosen anyway for sizes above 511 octets) |
| `--multicast` |  | Ask for the input (T->O) data by multicast |
| `--output uint32` |  | O->T connection point: the output assembly (or a heartbeat point for input-only and listen-only connections) |
| `--output-data string` |  | Output data to send, as hex (--output-size octets; default zeros); needs --yes |
| `--output-size uint16` |  | Size of the output (O->T) data in octets, without sequence count and run/idle header; 0 for a heartbeat |
| `--rpi duration` |  | Requested packet interval of both directions, e.g. 10ms |
| `--timeout-multiplier uint8` |  | The connection times out after RPI x 4 << multiplier (0-7) |
| `--yes` |  | Confirm that the connection writes output data (--output-size above 0): without it nothing is sent |

## `ethernetipctl object`

Usage: `ethernetipctl object <host[:port]> <class> [instance] [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl objects`

Usage: `ethernetipctl objects <host[:port]> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl read`

Aliases: `attribute`, `get`

Usage: `ethernetipctl read <host[:port]> <@class/instance/attribute> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--all` |  | Read every attribute of the instance with Get_Attributes_All (the path is @class/instance) |
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |
| `--type string` |  | Decode the value as this CIP data type: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, BYTE, WORD, DWORD, LWORD, SHORT_STRING, STRING, STRING2 (an array when the data holds several) [env: ETHERNETIPCTL_TYPE] |

## `ethernetipctl reset`

Usage: `ethernetipctl reset <host[:port]> [--type 0|1] --yes [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |
| `--type int` |  | Reset type of the Identity object: 0 emulates a power cycle, 1 returns to factory defaults |
| `--yes` |  | Confirm the reset: without it nothing is sent |

## `ethernetipctl services`

Usage: `ethernetipctl services <host[:port]> [flags]`

No flags of its own.

## `ethernetipctl session`

Aliases: `connect`

Usage: `ethernetipctl session <host[:port]> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Also open a connected (Class 3) messaging connection to the Message Router [env: ETHERNETIPCTL_CONNECTED] |
| `--hold duration` |  | Keep the session (and connection) open this long before closing it, e.g. 30s |
| `--large` |  | Open the Class 3 connection with Large_Forward_Open |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |
| `--size uint16` |  | Connection size of the Class 3 connection in octets (0 = 500, 4000 with --large) |

## `ethernetipctl status`

Usage: `ethernetipctl status <host[:port]> [flags]`

No flags of its own.

## `ethernetipctl tag`

Usage: `ethernetipctl tag [flags]`

No flags of its own.

Subcommands: `decode`, `list`, `read`, `types`, `write`

## `ethernetipctl tag decode`

Usage: `ethernetipctl tag decode <host[:port]> <tag> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl tag list`

Usage: `ethernetipctl tag list <host[:port]> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--program string` |  | List the tags of this program (Main or Program:Main) instead of the controller-scoped tags |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |
| `--system` |  | Include system tags and programs in the list |

## `ethernetipctl tag read`

Usage: `ethernetipctl tag read <host[:port]> <tag> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--count int` |  | Number of elements to read (default 1) |
| `--decode` |  | Decode a structure value with its templates |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl tag types`

Usage: `ethernetipctl tag types <host[:port]> <template-id> [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |

## `ethernetipctl tag write`

Usage: `ethernetipctl tag write <host[:port]> <tag> --type T --value V[,V...] --yes [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |
| `--type string` |  | Data type of the value: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, BYTE, WORD, DWORD, LWORD or STRING (Logix STRING) |
| `--value string` |  | Value to write; several comma-separated for consecutive array elements |
| `--yes` |  | Confirm the write: without it nothing is sent |

## `ethernetipctl version`

Usage: `ethernetipctl version [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--json` |  | Print the version as one JSON object |

One line on stdout; `--json`, or `--format json`, prints it as JSON:

```text
ethernetipctl 0.7.0 (tag v0.7.0, commit 1802d0f9afe8642d1e278120b76fcdfac967cb9e, built 2026-11-08T15:41:49Z, go-ethernetip 0.6.0, go1.26.10, linux/amd64)
{"name":"ethernetipctl","version":"0.7.0","tag":"v0.7.0","commit":"1802d0f9afe8642d1e278120b76fcdfac967cb9e","built":"2026-11-08T15:41:49Z","libraries":[{"name":"go-ethernetip","version":"0.6.0"}],"go":"go1.26.10","platform":"linux/amd64"}
```

The OT Fabric libraries the binary is built with follow the build date. A build without a
tag leaves out `tag …, ` (and has `"tag": ""`); a development build (`go build`, `go run`,
`make build` off a tag) is version `dev`, with the commit Go embeds (`-dirty` for a
modified tree) and build date `unknown`.

## `ethernetipctl write`

Usage: `ethernetipctl write <host[:port]> <@class/instance/attribute> (--data HEX | --type T --value V) --yes [flags]`

| Flag | Short | Meaning |
|---|---|---|
| `--connected` |  | Send the requests over a connected (Class 3) messaging connection, opened with Forward_Open, instead of as unconnected messages [env: ETHERNETIPCTL_CONNECTED] |
| `--data string` |  | Value to write as hex octets, as the target encodes it (0a00, "0a 00") |
| `--route string` |  | Route through the target to another device, as port/link pairs, comma-separated: 1/0 is slot 0 of the backplane, 1/0,2/192.168.1.10 then port 2 to that address [env: ETHERNETIPCTL_ROUTE] |
| `--type string` |  | Data type of --value: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, BYTE, WORD, DWORD, LWORD, SHORT_STRING, STRING, STRING2 |
| `--value string` |  | Value to write, with --type; several comma-separated for an array |
| `--yes` |  | Confirm the write: without it nothing is sent |

