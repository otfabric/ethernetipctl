# Adapter model (`ethernetipctl adapter --config`)

`ethernetipctl adapter` serves a simulated EtherNet/IP adapter (a target) with go-ethernetip's server. Without `--config` it serves a default adapter, shaped by flags; with `--config` it serves the model of a YAML file (or a JSON file, by its `.json` extension). Unknown fields are an error: a misspelt key does not pass silently. An invalid model exits `3` (`invalid_config`) before anything listens.

## What every adapter has

The standard objects of go-ethernetip's server: Identity (1), Message Router (2), Assembly (4), Connection Manager (6), Port (0xF4), TCP/IP Interface (0xF5) and Ethernet Link (0xF6). It accepts sessions on TCP, answers ListIdentity and ListServices on UDP at the same port, routes Unconnected_Send for slot 0 of the backplane (`--route 1/0`) to itself, opens Class 3 connections to the Message Router and Class 0/1 I/O connections to its assemblies, and answers the Reset service of the Identity object with success (logging a `reset` event; nothing else happens).

## The model

```yaml
# ethernetipctl adapter model: a small I/O adapter with a Logix tag database.
identity:
  vendor_id: 65535
  device_type: 12          # communications adapter
  product_code: 7
  revision: "2.1"
  serial_number: 0x00C0FFEE
  product_name: "model adapter"

assemblies:
  - {instance: 100, kind: input, size: 8, data: "0102030405060708"}
  - {instance: 150, kind: output, size: 8}
  - {instance: 151, kind: config, size: 4}
  - {instance: 152, kind: input-only}
  - {instance: 153, kind: listen-only}

echo:
  - {output: 150, input: 100}

logix:
  program_name: "Line1"
  templates:
    - id: 0x200
      name: Valve
      handle: 0x1234
      size: 8
      members:
        - {name: Position, type: REAL, offset: 0}
        - {name: Open, type: BOOL, bit: 0, offset: 4}
        - {name: Hosts, type: SINT, offset: 4}
  tags:
    - {name: Count, type: DINT, value: "5"}
    - {name: Levels, type: REAL, dims: [3], value: "1.5,2.5,3.5"}
    - {name: Label, type: STRING, value: "tank 1"}
    - {name: Valves, type: struct, template: 0x200, dims: [2]}
    - {name: "Program:Main.Step", type: INT, value: "3"}
```

This file is [`testdata/adapter.yaml`](../testdata/adapter.yaml); the tests serve it.

### `identity`

| Field | Meaning |
|-------|---------|
| `vendor_id` | Vendor ID. 65535 (0xFFFF) marks an unregistered vendor |
| `device_type` | Device type: 12 communications adapter, 43 generic device, 14 programmable logic controller, … |
| `product_code` | Product code |
| `revision` | `"major.minor"`, major 1 to 127 |
| `serial_number` | Serial number |
| `product_name` | 1 to 32 characters |

The state is always 3 (operational). `--product-name`, `--vendor-id`, `--product-code`, `--serial` and `--revision` override the model's values.

### `assemblies`

| Field | Meaning |
|-------|---------|
| `instance` | Instance number, unique, not 0 |
| `kind` | `input` (produced by the adapter, T->O), `output` (consumed, O->T; one exclusive owner at a time), `config` (receives the configuration data of a Forward_Open), `input-only` and `listen-only` (heartbeat connection points; size 0) |
| `size` | Size in octets (0 to 65000) |
| `data` | Initial data as hex, exactly `size` octets; zeros when omitted |

Without `assemblies` (and without `--config`) the adapter has OpENer's sample set: input 100 and output 150 of 32 octets, configuration 151 of 10, heartbeats 152 (input-only) and 153 (listen-only). `--assembly instance:kind[:size]` (repeatable) replaces the model's assemblies.

Every write of an output or configuration assembly, by an I/O connection or by Set_Attribute_Single, is logged as an `assembly-write` event with its data and run/idle state. Assembly data is read with `ethernetipctl assembly read` or with an I/O connection.

### `echo`

Each entry copies the data written to output assembly `output` into input assembly `input` (same size), so that an originator sees its outputs come back as inputs. `--echo` pairs each output assembly with the input assembly of the same size with the lowest instance.

### `logix`

An emulated Rockwell Logix tag database (vendor specific, not CIP) for the `tag` commands: symbolic Read Tag and Write Tag with their fragmented forms, Read-Modify-Write, the tag list, templates and the program name. `--logix` adds a demo database (tags `Count`, `Speed`, `Flags`, `Values[10]`, `Name`, `Motors[2]` of a `Motor` UDT, and `Program:Main.Local`) when the model has none.

| Field | Meaning |
|-------|---------|
| `program_name` | The project name the Program Name object reports (default `OTFabric`) |
| `templates` | Structure (UDT) definitions: `id` (1 to 0x0FFF), `name`, `handle` (unique), `size` in octets, `members` |
| `templates[].members` | `name`, `type` (an atomic type, `STRING`, or `struct` with `template`), `array` (the length of an array member), `bit` (the bit of a BOOL member in the octet at `offset`), `offset` |
| `tags` | `name` (controller scope, or `Program:<name>.<tag>`), `type` (`BOOL`, `SINT`, `INT`, `DINT`, `LINT`, `USINT`, `UINT`, `UDINT`, `ULINT`, `REAL`, `LREAL`, `BYTE`, `WORD`, `DWORD`, `LWORD`, `STRING`, or `struct` with `template`), `dims` (up to three array dimensions), `value` |
| `tags[].value` | The initial value: one value per element, comma-separated (decimal or 0x-prefixed hex for integers); for `STRING` the text, several separated by `\|` for an array. A structure tag starts zeroed |

A Logix BOOL member is a bit of a hidden host member, as the controllers lay them out: declare the host (`SINT` at the same offset) as in the example. Every write of a tag by an originator is logged as a `tag-write` event.

## Events

The adapter writes one record per event to stdout (`--format text`, `json` for JSON Lines, or `table`): `listening` (with the TCP, UDP and I/O addresses), `connected` and `disconnected` (TCP sessions), `request` (every explicit request and its status, with `--requests`), `connection-opened` and `connection-closed` (CIP connections, with their IDs, triad and path), `assembly-write`, `tag-write`, `reset`, `stopping`. See [json-output.md](json-output.md#streams).

## Listening

`--listen host[:port]` (default every interface, port `--port` or 44818; port 0 picks a free one, which the `listening` event names) and `--io-address` (default UDP 2222 on the listen host; `:0` picks a free port, which Forward_Open replies announce to the originator). ListIdentity over UDP is served at the TCP port of the same host when that UDP port is free; the `listening` event says when it is not.
