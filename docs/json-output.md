# `--format json` field conventions

These conventions are a **stable contract** for scripting. Field names are snake_case. A breaking change to a name or shape is called out in the release notes. The golden files in [`cmd/testdata/golden`](../cmd/testdata/golden) pin the output of representative commands (with timings, session handles, connection IDs and addresses replaced by placeholders); the tests fail when a field changes.

## General

| Concept | JSON fields |
|--------|-------------|
| Device | `target`: `host:port` as given (with the default port filled in) |
| Messaging | `route` (the `--route`, omitted when direct), `connected` (`true` over a Class 3 connection) |
| Paths | `path`: `@class/instance[/attribute]` with decimal numbers (`"@245/1/8"`); other paths as their segments in brackets (`"[class 0x04] [instance 151] [point 150] [point 100]"`) |
| Classes | `class` (number), `class_name` (`"Identity"`, `"TCP/IP Interface"`, `"Symbol (Rockwell)"`, or `"class 0x1234"` for one the catalog does not name) |
| Raw data | `data`: the octets as the device encodes them, lower-case hex without separators (`"0c74657374"`); `length` its size |
| Decoded values | `type` (the CIP type name, `"DINT"`, `"SHORT_STRING"`) and `value`: a number, a boolean (BOOL), a string (string types), or an array when the data holds several values. A REAL is the shortest decimal it holds (`1.1`); NaN and the infinities are the strings `"NaN"`, `"+Inf"`, `"-Inf"` |
| Durations | `duration_ms`, `rtt_ms`, `held_ms`, `ot_rpi_ms`, `to_rpi_ms`, `wait_ms` (numbers, milliseconds with fractions) |
| Times | RFC 3339 in UTC with milliseconds (`received_at`, `timestamp`) |
| Repeated rows | `devices`, `services`, `interfaces`, `objects`, `class_attributes`, `counters`, `checks`, `tags`, `members`, `params`, `assemblies`, `connections`, `issues`: **arrays of objects**, empty arrays rather than null |
| A failure inside a result | `error` (or `identity_error`, `instance_error`) object: `kind`, `code`, `message`, and for a CIP refusal `cip_status` (number) and `extended_status` (array of numbers); omitted when there is none |
| Fatal error | one object `{"error": {"kind", "code", "message"[, "cip_status", "extended_status"]}}` on stdout, nothing else; see [exitcodes.md](exitcodes.md) |

## Identity

Used by `identify`, `status` and `discover`.

| Field | Meaning |
|-------|---------|
| `vendor_id`, `vendor_name` | the vendor ID and ODVA's name for it (`""` when the public list has none) |
| `device_type`, `device_type_name` | the device type (`12`, `"Communications Adapter"`) |
| `product_code`, `revision` (`"3.7"`), `serial_number` (number), `product_name` | |
| `status`, `status_flags`, `extended_device_status` | the status word (number), its set flags joined with `\|` (`"OWNED\|CONFIGURED\|MAJOR_RECOVERABLE_FAULT"`, `""` when none), and bits 4-7 |
| `state`, `state_name` | attribute 8, where it was reported (ListIdentity): `"nonexistent"`, `"self-testing"`, `"standby"`, `"operational"`, `"major-recoverable-fault"`, `"major-unrecoverable-fault"`, `"default"`, or `"state-<n>"`. Omitted for Get_Attributes_All |

## Results

| Command | Shape |
|---------|-------|
| `discover` | `address` (where the request went), `wait_ms`, `found`, `duration_ms`, `devices`: `address` (the sender of the reply), `socket` (the address the device reports for its TCP port), `protocol_version`, `identity` |
| `identify` | `target`, `list_identity` (`protocol_version`, `socket`, `identity` with `state`), `identity` (Get_Attributes_All; omitted with `identity_error` when that failed), `duration_ms` |
| `status` | `target`, `healthy`, `faults` (`"major-recoverable-fault"`, …, `"state standby"`), `identity`, `rtt_ms` |
| `services` | `target`, `count`, `services`: `name`, `version`, `capabilities`, `tcp`, `udp` |
| `interfaces` | `target`, `count`, `interfaces`: `type`, `type_name`, `data` |
| `read`, `write`, `assembly read`, `assembly write` | `target`, `route`, `connected`, `service` (`"Get_Attribute_Single"`, `"Get_Attributes_All"`, `"Set_Attribute_Single"`), `path`, `class`, `class_name`, `instance`, `attribute` (omitted for Get_Attributes_All), `length`, `data`, `type` and `value` (with `--type`), `rtt_ms` |
| `objects` | `target`, `route`, `connected`, `count`, `objects`: `class`, `hex` (`"0xF5"`), `name`, `vendor_specific` |
| `object` | `target`, `route`, `connected`, `class`, `class_name`, `class_attributes` (`attribute`, `name`: `revision`, `max_instance`, `number_of_instances`, `max_class_attribute`, `max_instance_attribute`; `data` and `value`, or `error`), `instance`, `instance_data` or `instance_error` |
| `reset` | `target`, `route`, `type`, `rtt_ms` |
| `connections` | `target`, `max_connections` and `active_connections` (omitted where the device lacks them), `counters`: `attribute`, `name` (`open_requests`, `open_format_rejects`, `open_resource_rejects`, `open_other_rejects`, `close_requests`, `close_format_rejects`, `close_other_rejects`, `connection_timeouts`), `value` or `error` |
| `session` | `target`, `route`, `local`, `session` (the handle), `connection` (with `--connected`), `held_ms`, `duration_ms` |
| `forward-open` | `target`, `route`, `service` (`"Forward_Open"`, `"Large_Forward_Open"`), `transport` (`"class 1 cyclic"`), `ot_size`, `to_size` (connection sizes), `connection`, `closed`, `rtt_ms` |
| `forward-close` | `target`, `route`, `connection_serial`, `originator_vendor`, `originator_serial`, `path`, `rtt_ms` |
| `diagnose` | `target`, `ok`, `passed`, `failed`, `skipped`, `duration_ms`, `checks`: `name` (`tcp-connect`, `register-session`, `list-identity`, `list-services`, `identity`, `tcpip-interface`, `ethernet-link`, `connection-manager`), `status` (`"ok"`, `"error"`, `"skipped"`), `duration_ms`, `detail` (an object of the check's findings, see below) or `error` |
| `tag read`, `tag decode` | `target`, `route`, `connected`, `tag`, `type` (`"DINT"`, `"STRING"`, `"struct"`), `handle` (structures), `count`, `data`, `values` (atomic types and STRING), `fields` (decoded structures: `name`, `type`, `value` or `data` or nested `fields`), `rtt_ms` |
| `tag write` | `target`, `route`, `connected`, `tag`, `type`, `count`, `values`, `bit` (a bit written with Read-Modify-Write), `rtt_ms` |
| `tag list` | `target`, `route`, `connected`, `program` (`""` for controller scope), `program_name` (the project, where reported), `count`, `tags`: `name`, `type` (`"DINT"`, `"STRING"`, `"struct 0x0123"`), `dims` (array lengths), `instance`, `template` (structures), `system` |
| `tag types` | `target`, `route`, `connected`, `id`, `name`, `handle`, `size`, `members`: `name`, `type` (`"DINT[4]"`), `offset`, `info`, `hidden` |
| `eds inspect` | `file`, `device` (`vendor_id`, `vendor_name`, `device_type`, `type_name`, `product_code`, `revision`, `product_name`, `catalog`), `sections`, `params` (`number`, `name`, `type`, `size`, `units`, `min`, `max`, `default`, `path`, `enum_values`), `assemblies` (`number`, `name`, `instance`, `size`, `members`), `connections` (`number`, `name`, `class`, `ot_rpi`, `to_rpi`, `ot_size`, `to_size`, `path`), `errors`, `warnings` |
| `eds validate` | `file`, `valid`, `errors`, `warnings`, `issues`: `severity` (`"error"`, `"warning"`), `line`, `section`, `message` |
| `eds generate`, `eds codegen` with `--output` | `kind` (`"eds"`, `"go"`), `output`, `bytes`, `sha256`. Without `--output` the EDS text or Go source itself goes to stdout, whatever `--format` says |

A `connection` object has `class`, `ot_connection_id`, `to_connection_id`, `connection_serial`, `originator_vendor`, `originator_serial` (the triad forward-close needs), `ot_rpi_ms`, `to_rpi_ms`, `path`.

The `detail` of the `diagnose` checks: `tcp-connect` `address`; `register-session` `session` (`"0x…"`); `list-identity` `product_name`, `vendor_id`, `revision`, `state`, `status`; `list-services` `services` (names, comma-separated); `identity` `product_name`, `product_code`, `serial_number`; `tcpip-interface` `status`, `ip`, `mask`, `gateway`, `host_name` (when set); `ethernet-link` `speed_mbps`, `link_active`, `full_duplex`, `mac`; `connection-manager` the eight counters by name.

## Streams

| Command | Records (JSON Lines: one compact object per line) |
|---------|-------|
| `io` | one per input sample, as it arrives: `received_at`, `seq` (1-based), `run`, `size`, `data`. A sample is new input data (a new sequence count of the producer); with `--changes` only data or a run state that differs from the previous sample. The connection's IDs and the summary (samples, packets sent and received, drops, duplicates, state) go to stderr as text |
| `adapter` | one per event: `timestamp`, `event`, and where they apply `address` (listening), `remote`, `service` and `status` (request), `instance`, `size`, `data`, `run` (assembly-write), `tag` (tag-write), `connection` (connection-opened, connection-closed), `detail`. Events: `listening`, `connected`, `disconnected`, `request` (with `--requests`), `connection-opened`, `connection-closed`, `assembly-write`, `tag-write`, `reset`, `stopping` |

## Streams and stderr

- A streaming command writes **compact** JSON, one object per line; every other command writes **one** indented JSON document.
- Progress, connection state, summaries and the `--debug` trace go to **stderr**, as text, in every format. stdout holds nothing but the result.
