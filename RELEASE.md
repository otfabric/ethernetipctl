# ethernetipctl Releases

## v0.9.2

**Date:** 2026-11-20
**Previous release:** v0.9.1

## Summary

Built on go-ethernetip v0.9.0 (was v0.8.3), with three more fixes. No flag or JSON field changes; the text of the invalid tag name error is shorter.

## Changes

### Fixes

- **tag**: `tag read`, `write`, `list` and `decode` accept the tags Logix gives I/O modules, one controller-scoped name with colons (`Local:1:I.Data[0]`, `Remote:2:C`); they were refused as invalid tag names (exit status 3, `invalid_path`). The adapter's model takes them as tag names too. The parsing is go-ethernetip's (RELEASE.md, v0.9.0); not verified against a controller, as no recording or interop peer has module tags.
- **io**: the first datagrams carry `--output-data` and, with `--idle`, the idle state. The output data and the run state were set after the connection opened, and it produces as soon as it is open: its first datagram(s) could carry zeros in run, even with `--idle`, so a device could act on outputs before they were set. They are now part of the connection's configuration (`scanner.IOConfig.Output` and `Idle`).
- **adapter**: a model's ULINT and LWORD tags take values above the int64 range (up to 18446744073709551615), as `tag write` does; they were refused as invalid values.
- **tag**: an invalid tag name is named once in the message (`invalid tag name "1x": symbol "1x"`), without the library's `logix: invalid tag name: "1x":` repeated after it.

### Tests

- `TestAdapterFromModel`: a module tag (`Local:2:O`) in the model is listed, read, written and bit-written by its name.
- `TestScannerConfigStartsWithOutputAndIdle`, `TestIOStartsWithOutputAndRunState`: the output data and the idle state are in the connection's configuration; the target's first datagram carries them.
- `TestModelUnsignedValues`: unsigned model values up to their type's maximum; above it, or negative, refused.
- `TestCheckTagMessage`: the message of a refused tag name.

---

## v0.9.1

**Date:** 2026-11-16
**Previous release:** v0.9.0

## Summary

Built on go-ethernetip v0.8.3 (was v0.8.2). No flag or JSON output changes.

## Changes

- **io**: a connection lost between the Forward_Open reply and the start of the I/O exchange is reported as a connection error (`connection_lost`, exit status 4). Before, it came out as invalid input (`invalid_input`, exit status 3, "production without an interval or destination"), which pointed at the flags. The fix is in the scanner of go-ethernetip v0.8.3 (go-ethernetip RELEASE.md, v0.8.3).

---

## v0.9.0

**Date:** 2026-11-15
**Previous release:** v0.8.1

## Summary

Two changes for scripts that change devices, under Breaking: `--config-data` needs `--yes`, and `write`, `reset`, `assembly write` and `tag write` take a route only from `--route`. Besides them: bounds on the events and samples that wait for stdout and on the adapter model, device strings escaped in text and table output, and the connection path of `forward-open` and `forward-close` as `io` sends it. Built on go-ethernetip v0.8.1 (was v0.8.0). No exit code is added or redefined. `forward-open` gains `--yes`; the JSON output gains the adapter's `dropped` event, and with `--config 0` the `path` of `forward-open` and `forward-close` names `[instance 0]`.

## Changes

### Breaking

- **forward-open, io: `--config-data` needs `--yes`.** The target writes the configuration data to its configuration assembly when it accepts the connection, so it is a change like the others. `forward-open` has a new `--yes` flag, with no environment variable, as for the other commands; `io` asks for `--yes` when `--config-data` is given, as it does for output data. What a script sees now: without `--yes`, both exit 2 with kind `usage` and code `confirmation_required` (`forward-open with --config-data changes the target: add --yes to confirm (nothing was sent)`, `an I/O connection with --config-data changes the target: …`) before any connection is opened, where they used to open the connection and write the configuration. To keep the old handling, add `--yes` to the invocations that pass `--config-data`. Without `--config-data` nothing changes.
- **write, reset, assembly write, tag write: the route comes only from `--route`.** `ETHERNETIPCTL_ROUTE` no longer applies to these four; the reading commands keep taking it. What a script sees now: with the variable set and no `--route`, the change goes to the host on the command line, without a route, and stderr has one line, `ETHERNETIPCTL_ROUTE=<route> is not used: a change takes its route only from --route`; the output and the JSON name no route. To keep the old handling, pass the route on the command line: `--route "$ETHERNETIPCTL_ROUTE"`. Their help no longer names the variable for `--route`.

### Other changes

- **forward-open, forward-close**: the connection path always names the configuration instance, `[instance 0]` when `--config` is 0, as the `--config` description says and as `io` (go-ethernetip's scanner) sends it, so the three commands send the same path for the same flags. With `--config 0` the text and the JSON `connection.path` (forward-open) and `path` (forward-close) show `[instance 0]`, and `--config-data` has one word less of room, as with `io`.
- **adapter**: events reach stdout through a queue of 1024. While stdout does not keep up, the adapter goes on serving, the events beyond the queue are left out, and a new `dropped` event counts them (JSON `"event": "dropped"`, `detail` with the number). When it stops, the adapter waits at most a second for stdout.
- **adapter**: a model holds at most 4096 assemblies, 4096 tags and 4096 templates, and 64 MiB of assembly and tag data together; a larger one is refused as `invalid_config` (exit 3) before the adapter starts. The `bit` of a BOOL member is 0 to 7, and a larger one is refused as `invalid_config`; it was taken modulo 8.
- **io**: up to 1024 samples, with at most 4 MiB of data as hex, wait for stdout. When stdout is slower than the device, the oldest are dropped, the summary on stderr counts them (`; N sample(s) not written: the output did not keep up`) and `seq` shows the gap. After Ctrl-C only the samples already waiting are written.
- **text and table output**: names and strings a device sends (product and service names, tag, template and member names, the values of decoded structures, the details of `diagnose`) are written with the characters that do not print escaped as Go writes them (`\x1b`, `\r`, `\u2028`, `\xff`). JSON is unchanged.
- **reset**: text and table output name the route, as the other messaging commands do (`Reset type 0 (power cycle) of 192.168.1.10:44818 route 1/0 accepted in 1.2 ms`).
- **tag read, tag decode**: the room taken for the values of a reply follows the data it holds, not the count it declares.
- **eds codegen**: in the header comment of the generated code, the EDS file name is written with control characters, U+2028 and U+2029, the byte order mark and invalid UTF-8 replaced by `_`.
- **docs**: the help of `diagnose` names exit code 1 (findings) for a failed or skipped check, the code it returns (it said 7). README and SECURITY.md give exit code 2 (`confirmation_required`) for a change without `--yes`, as docs/exitcodes.md does (they said 3). README, SECURITY.md, docs/exitcodes.md and docs/commands.md cover the two breaking changes.
- **CI**: a job runs `make e2e`, the end-to-end tests of the built binary.
- **32-bit builds**: `--type` (reset), `--count` (tag read) and `--max-devices` (discover) read their value in 64 bits. A value beyond the range of a 32-bit int was cut to its low 32 bits (`reset --type 4294967296` was type 0, a power cycle); it is now refused by the flag's range check (exit 3), as on 64-bit builds.
- **deps**: go-ethernetip v0.8.1 (was v0.8.0), whose `eds.GenerateGo` keeps the header comment on one line for any source name, as `eds codegen` already passes it.

---

## v0.8.1

**Date:** 2026-11-14
**Previous release:** v0.8.0

## Summary

Built on go-ethernetip v0.8.0 (was v0.7.0), with v0.7.1 in between. No flag, exit code or JSON output changes.

## Changes

- **adapter**: a request handler or `Assembly.OnWrite` that does not return normally is logged at Error level with its stack, its request is answered with general status 0x10, and the adapter goes on serving (go-ethernetip RELEASE.md, v0.8.0). The inactivity timeout set to 0 at run time holds for open sessions (v0.7.1).
- **io**: the scanner refuses a Forward_Open reply whose O->T connection ID is 0, and `Close` returns from the scanner's callbacks (v0.7.1, v0.8.0). The floors on the O->T actual packet interval keep their values: a quarter of the RPI, and 100 µs.
- **eds**: the EDS parser bounds sections and fields (v0.7.1).

---

## v0.8.0

**Date:** 2026-11-14
**Previous release:** v0.7.0

## Summary

Built on go-ethernetip v0.7.0 (was v0.6.0), which tightens bounds and
lifecycles in the server, the scanner, the Logix client and emulation, and
the EDS package. No flag, exit code or JSON output changes.

## Changes

- **adapter**: a connection whose TLS handshake fails frees its place under
  the TCP connection limit; Multiple Service Packet and attribute-list
  replies stop with 0x11 as soon as they exceed what the packet or the
  connection can carry; a Forward_Close during a multicast Forward_Open
  leaves no producer running.
- **io**: the scanner refuses a Forward_Open reply with a T->O actual packet
  interval of 0 or an O->T interval below a quarter of the requested RPI,
  and sends O->T data to the target's address.
- **tag list**, **tag template**: stop with a protocol error after 65536
  partial replies (or 16 MiB for the tag list).
- **eds**: entries that name the same number (`Param1`, `Param01`) give one
  entry, and validation reports the duplicates; validation runs in linear
  time.
- **Dependencies**: go-ethernetip v0.7.0.

---

## v0.7.0

**Date:** 2026-11-08
**Previous release:** v0.6.0

## Summary

Built on go-ethernetip v0.6.0, which is now tested against recorded traffic
of real Rockwell devices (a 1756-L63 ControlLogix, a 1756-ENBT/A, a Micro850
and a MicroLogix 1400). Three corrections that recordings showed reach the
CLI; no flag, exit code or JSON output changes.

## Changes

- **tag template**, **tag list**, **tag read** of structures: a template's
  definition is read with attribute 4 × 4 − 20 octets, as RSLogix 5000 reads
  a 1756-L63's templates; with × 4 − 23, against such a controller the end
  of the last member's name was lost ("U" for "UN").
- **adapter** (Logix model): a BOOL tag written with any non-zero value reads
  back as 1, as on a recorded Micro850; it read back the written octet
  (0xFF).
- **adapter**: SendRRData replies carry the request's timeout, as the
  recorded ControlLogix module answers; they carried 0.
- **Dependencies**: go-ethernetip v0.6.0.
- **Docs**: the `version` example in docs/commands.md.

---

## v0.6.0

**Date:** 2026-11-07
**Previous release:** v0.5.2

## Summary

`version` prints one line, as every OT Fabric CLI does, or JSON with `--json`
(or `--format json`). Shell completion completes files by type and no longer
offers file names for addresses, numbers and paths.

## Changes

- **version**: one line, `ethernetipctl 0.6.0 (tag v0.6.0, commit <sha>,
  built <date>, go-ethernetip 0.5.2, go1.26.10, linux/amd64)`, instead of one
  line per field, with the OT Fabric libraries of the build.
  No leading `v`; the tag is left out when there is none; the full commit
  (from the build, or from Go's build information with `-dirty` for a
  modified tree); a development build (a pseudo-version too) is `dev`,
  built `unknown`. Scripts that
  read `ethernetipctl version: …` should use `version --json`.
- **version --json** (new), and `version` with `--format json` or
  `ETHERNETIPCTL_FORMAT=json`: `{"name","version","tag","commit","built","libraries","go","platform"}`.
- **completion**: bash, zsh, fish and PowerShell, as before. The EDS argument
  of `eds inspect`, `validate` and `codegen` completes `.eds` files,
  `eds generate --output` `.eds`, `eds codegen --output` `.go`, and
  `adapter --config` YAML or JSON files. Hosts, paths, numbers and durations
  complete nothing instead of file names.
- **Makefile**: injects the full commit SHA, and a version and tag only when
  HEAD is exactly on a tag (otherwise the binary reports `dev`).

---

## v0.5.2

**Date:** 2026-11-07
**Previous release:** v0.5.1

## Summary

go-ethernetip v0.5.2. `io` and `forward-open` without `--config` now name
configuration instance 0 in the Class 1 connection path, as every other
originator does. Exit codes, error codes and output are unchanged.

## Changes

- **go-ethernetip** v0.5.0 → v0.5.2: the scanner's Class 1 path is
  `20 04 24 <config> 2C <O->T> 2C <T->O>`, also with `--config 0` (before,
  instance 0 left the segment out), which cB-Sigma's enip and other targets
  that require the configuration path accept. The library's interop suite
  now covers thirteen independent stacks (ethernetip-interop v0.8.1).
- **io**, **forward-open**: `--config` is described as "0 = none; sent as
  instance 0 in the path".

---

## v0.5.1

**Date:** 2026-11-05
**Previous release:** v0.5.0

## Summary

go-ethernetip v0.5.0. `tag write` now builds its values with the library's
constructors instead of encoding them itself. Exit codes and error codes
are unchanged.

## Changes

- **go-ethernetip** v0.4.0 → v0.5.0 (REAL range check, `logix.Uints`,
  control characters refused by `eds.Build`).
- **tag write**: integer values are built with `logix.Ints` and
  `logix.Uints` (ULINT and LWORD keep their whole unsigned range), and REAL
  and LREAL values with `logix.Floats`, which checks the REAL range. A value
  out of range is still `invalid_value` (exit 3), and its message now names
  the type: `invalid value: 300 does not fit SINT` and `invalid value:
  3.5e+38 does not fit REAL`. Error messages lose the library's
  `logix: wrong type:` prefix.

---

## v0.5.0

**Date:** 2026-11-05
**Previous release:** v0.4.1

## Summary

Hardening after a SAST review (gosec and opengrep at low/low, 19 findings:
16 fixed in code, 3 operator-named file paths recorded as false positives in
sast-triage) and new hostile-device, chaos, signal and fuzz tests, which
found ten defects. Values that wrapped or were silently changed are now
refused before anything is sent. Go 1.26 is the minimum.

## Changes

- **Go 1.26** minimum (CI on 1.26 and 1.27); `go fix` applied;
  go-ethernetip v0.3.4 → v0.4.0; shared CI v3.3.1.
- **forward-open / io**: `--output-size` above 65529 or `--input-size`
  above 65533 wrapped the 16-bit connection size of Forward_Open (65530
  was sent as 0); they are `invalid_value` (exit 3). `io` refuses sizes
  above 65000, the scanner's limit, before connecting instead of after.
  `--config-data` too long for the connection path (255 words) is
  `invalid_value` before connecting; it was a `protocol_violation` (exit
  6) after.
- **io / adapter**: a negative `--duration` is `invalid_value`; it ran
  until interrupted.
- **tag write**: values are encoded as `write` encodes them. A REAL beyond
  float32 (e.g. 3.5e38) is refused; it was written as infinity. ULINT and
  LWORD take their whole unsigned range; values above 2^63-1 were
  refused. A bit beyond the type's width (`Count.32` of a DINT) is refused
  before connecting.
- **eds generate**: a control character (a newline above all) in
  `--product-name`, `--vendor-name`, `--description` or an assembly or
  connection name is `invalid_value`; it produced an EDS that did not
  parse, with exit 0. A device whose product name has one fails with
  `protocol_violation` (exit 6).
- **Output files** (`eds generate --output`, codegen) are created 0600, and
  a file replaced with `--force` is set to 0600 too (it kept its mode).
- **adapter**: a model file above 16 MiB is refused instead of read whole.
- **JSON Lines**: the fatal error of a stream (`io`, `adapter`) with
  `--format json` is one compact line after the records; it was an
  indented object that broke line-by-line readers.
- **Tests**: hostile flags (every numeric and duration flag at its bounds,
  all CIP types), a hostile device fake (about 20 behaviours), a
  delay/cut proxy, SIGINT/SIGTERM against the binary, hostile EDS and
  model files, and fuzz targets for value encoding, tag values, EDS specs,
  checks, host parsing and model loading.

---

## v0.4.1

**Date:** 2026-10-31
**Previous release:** v0.4.0

### Fixed

- Divide-by-zero in `eip.Decode` for a type with no size, found by fuzzing. Not reachable from the CLI today, because `ParseType` filters types.
- Built on go-ethernetip v0.3.4.

---

## v0.4.0

**Date:** 2026-10-30
**Previous release:** v0.3.0

### Dependencies

- `go-ethernetip` v0.3.0 → v0.3.2.

---

## v0.3.0

**Date:** 2026-10-30
**Previous release:** v0.2.2

### Summary

**Breaking for scripts:** the exit codes follow the OT Fabric organisation table (ADR-0006 of otfabric/engineering). Nothing else changes in the commands.

### Exit codes

| Case | Old | New | JSON error |
|---|---|---|---|
| `context_canceled` (interrupted before a result) | 10 | 130 | kind `interrupted`, was `internal` |
| `unsupported` | 6 | 8 | kind `unsupported`, was `refused` |
| `confirmation_required` (no `--yes`) | 3 | 2 | kind `usage`, was `invalid_input` |
| `diagnose` with a failed or skipped check | 7 | 1 | the report is unchanged |

Every other exit code is unchanged. See [docs/exitcodes.md](docs/exitcodes.md).

### Environment

`ETHERNETIPCTL_FORMAT` is now the canonical name of the format variable; `ETHERNETIPCTL_OUTPUT_FORMAT` stays accepted, and `ETHERNETIPCTL_FORMAT` wins when both are set.

---

## v0.2.2

**Date:** 2026-10-26  
**Previous release:** v0.2.1

### Summary

Built on go-ethernetip v0.3.0. The commands and their output are unchanged
(docs/examples.md regenerated: only timings and connection identifiers
differ).

### Dependencies

- Built on [go-ethernetip v0.3.0](https://github.com/otfabric/go-ethernetip/releases/tag/v0.3.0), tested against eleven independent EtherNet/IP stacks.

### Fixed (through go-ethernetip)

- `adapter --logix`: a structure template's definition is sized as Rockwell's 1756-PM020 specifies (attribute 4 × 4 − 23 octets); pylogix, node-ethernet-ip and libplctag read the last member name of a template truncated before, and pylogix looped on a template read past the end.
- `adapter`: replies on a Class 3 connection fit the connection's negotiated size (a large Logix read is a partial transfer), and a Multiple Service Packet may carry up to 1000 requests.

### Upgrade notes

- None.

---

## v0.2.1

**Date:** 2026-10-26  
**Previous release:** v0.2.0

### Summary

Built on go-ethernetip v0.2.1, which fixes the simulated adapter's discovery.

### Dependencies

- Built on [go-ethernetip v0.2.1](https://github.com/otfabric/go-ethernetip/releases/tag/v0.2.1).

### Fixed

- `adapter`: ListIdentity over UDP (what `discover` sends) could be off at random. The adapter's I/O socket could take the number of its explicit port before the UDP socket for ListIdentity was opened (seen on Windows); the adapter then answered ListIdentity over TCP only and `discover` found nothing. The UDP socket is now opened first.

### Upgrade notes

- None.

---

## v0.2.0

**Date:** 2026-10-26  
**Previous release:** v0.1.0

### Summary

Documentation with real output, structure names in the Logix commands, and
go-ethernetip v0.2.0.

### Dependencies

- Built on [go-ethernetip v0.2.0](https://github.com/otfabric/go-ethernetip/releases/tag/v0.2.0): listen-only connections next to their owner on one I/O endpoint, multicast left and joined on the right interface, TLS verified against the dialed host, and the Logix emulation of `adapter --logix` addressing BOOL arrays by DWORD as Logix does.

### Changed

- `tag list` names structure types by their template (`Motor`); JSON `type` was `"struct 0x0123"` (kept when the template cannot be read). BOOL arrays, which Logix lists as DWORD arrays, show as `BOOL`.
- `tag decode` names the structure (`Motors[1] = Motor[1] (handle 0xBEEF)`; JSON field `struct`); STRING values of decoded fields are quoted.
- `diagnose` reports the identity status word (`status=0x0000`) instead of an empty value.
- With `adapter --logix`, an index into a BOOL array counts DWORDs (`Bits[1]` is bits 32 to 63), as on a Logix controller.

### Added

- [docs/examples.md](docs/examples.md): every documented command run against the simulated adapter with its real output, regenerated by `make examples`; the README shows these outputs.

### Upgrade notes

- Scripts that matched `"struct 0x...."` in `tag list` JSON should use the `template` field, which is unchanged.

---

## v0.1.0

**Date:** 2026-10-26  
**Previous release:** none

### Summary

The first release of ethernetipctl: an EtherNet/IP and CIP command-line originator, diagnostics tool and adapter simulator, built on go-ethernetip and following the structure and conventions of dnp3ctl (`--format text|json|table`, a JSON error object, stable exit codes, `ETHERNETIPCTL_*` environment variables).

### Dependencies

- Built on [go-ethernetip v0.1.0](https://github.com/otfabric/go-ethernetip/releases/tag/v0.1.0).

### Added

- **Discovery and identity:** `discover` (one ListIdentity datagram to a broadcast address or a device; never a scan), `identify` (ListIdentity over the session and the Identity object), `status` (health in one line), `services`, `interfaces`.
- **Explicit messaging:** `read` (Get_Attribute_Single, Get_Attributes_All with `--all`; aliases `attribute`, `get`) and `write` (Set_Attribute_Single) of any `@class/instance/attribute`, raw hex or decoded and encoded as CIP data types; `objects` (the Message Router's class list), `object` (class attributes and an instance), `reset`. `--route` (Unconnected_Send) and `--connected` (Class 3) on every explicit command.
- **Connections and I/O:** `session` (alias `connect`), `forward-open` (with `--keep`), `forward-close`, `connections` (Connection Manager counters), `io` (Class 1 I/O connections streaming the inputs), `assembly read` and `assembly write`.
- **Diagnostics:** `diagnose`, eight structured checks with outcome and timing (exit 7 when one fails).
- **Logix tags (Rockwell, vendor specific):** `tag read`, `tag write` (also single bits), `tag list`, `tag types`, `tag decode`.
- **EDS:** `eds inspect`, `eds validate` (exit 3 on errors), `eds generate` (from a device or flags), `eds codegen` (Go code).
- **Adapter simulator:** `adapter` (alias `server`) from flags or a YAML/JSON model (identity, assemblies, echo, Logix templates and tags), streaming its events.
- **Safety:** every command that changes a device needs `--yes` and sends nothing without it.

### Upgrade notes

- First release.

---
