# Process exit codes

`ethernetipctl` uses stable exit codes for scripting. A fatal error is rendered once: as one `error:` line on stderr, or as a single JSON `error` object on stdout when `--format json` is active. A usage error (exit code 2: an unknown command or flag, a missing argument, a value a flag cannot hold) follows `--format` as well: ethernetipctl reads `--format` from the command line, or `ETHERNETIPCTL_OUTPUT_FORMAT`, even when the rest of the invocation cannot be parsed, and writes `{"error": {"kind": "usage", "code": "invalid_input", "message": ...}}` on stdout.

| Code | Meaning |
|------|---------|
| 0 | Success (`ExitOK`) |
| 2 | Usage (`KindUsage`): unknown command or flag, a missing or extra argument, a value a flag cannot hold |
| 3 | Invalid input (`KindInvalidInput`): a target, path, value, flag combination, environment variable, adapter model or EDS file that does not make sense, or a command that changes a device without `--yes`. Nothing was sent. Also `eds validate` of a file with errors (after the report) |
| 4 | Transport (`KindTransport`): the TCP connection cannot be made (refused, unreachable), is lost, or the adapter or the I/O endpoint cannot listen |
| 5 | Timeout (`KindTimeout`): the target did not answer RegisterSession or a request in `--timeout`, or an I/O connection's input stopped arriving |
| 6 | Refused or protocol error (`KindRefused`, `KindProtocol`): the target answered with a CIP general status other than success (the JSON error carries `cip_status` and `extended_status`), refused a Forward_Open, does not support the command, or violated the protocol |
| 7 | Partial (`ExitPartial`): `diagnose` wrote its report, and at least one check failed or was skipped |
| 10 | Output / internal (`KindOutput`, `KindInternal`): the result could not be written, the command was interrupted before it had a result, or something unexpected happened |

**Kind vs code:** `kind` drives the exit code; `code` in the JSON error object is stable for machines. Do not parse the human `message`.

| `kind` | `code` values |
|--------|---------------|
| `usage` | always `invalid_input` |
| `invalid_input` | `invalid_input`, `invalid_format`, `invalid_address`, `invalid_path`, `invalid_value`, `invalid_flag_combination`, `invalid_config`, `confirmation_required` (a change without `--yes`), `invalid_eds`, `input_file_open_failed`, `output_file_create_failed` (an `--output` file exists without `--force`) |
| `transport` | `transport_connect_failed`, `connection_lost`, `listen_failed` |
| `timeout` | `request_timeout` |
| `refused` | by CIP general status: `connection_failure` (0x01, a Forward_Open or Forward_Close refused by the Connection Manager; `extended_status` holds its extended status such as 0x0107 connection not found or 0x0106 ownership conflict), `resource_unavailable` (0x02), `path_segment_error` (0x04), `path_destination_unknown` (0x05, also an unknown Logix tag), `service_not_supported` (0x08), `invalid_attribute_value` (0x09), `state_conflict` (0x0C, 0x10), `attribute_not_settable` (0x0E), `privilege_violation` (0x0F), `not_enough_data` (0x13), `attribute_not_supported` (0x14), `too_much_data` (0x15), `cip_error` (any other status); `unsupported` (an encapsulation command the target does not support) |
| `protocol` | `protocol_violation`, `request_busy`, `session_refused` (encapsulation status: invalid session handle, unsupported protocol version) |
| `output` | `json_encode_failed`, `table_render_failed`, `output_write_failed`, `output_file_write_failed`, `output_file_create_failed` |
| `internal` | `context_canceled`, `internal_unexpected` |

**Commands that change a device:** `write`, `reset`, `assembly write`, `tag write` and `io` with output data (`--output-size` above 0) need `--yes`. Without it they validate the invocation, open no connection, send nothing and exit `3` with code `confirmation_required`. A refusal by the target exits `6`, a missing answer `5`; after a timeout a write may or may not have been executed. Requests are never repeated.

**Partial exit:** only `diagnose` uses `7`, and only after its report was written. A device that refuses to talk at all still gets a report (the failed `tcp-connect` or `register-session` check and the skipped rest), so `diagnose` never exits `4` or `5`.

**`eds validate`:** writes the report in every case; exits `0` when the file has no error (warnings are allowed) and `3` when it has one. A file that cannot be read or parsed at all is a fatal `3` (`input_file_open_failed`, `invalid_eds`) without report.

**Streaming commands:** `io` and `adapter` write one record at a time. Stopping them with Ctrl-C (SIGINT) or SIGTERM, `--duration` or `--count` ends the stream normally: exit `0`. `io` exits `5` when the target stops producing (the connection times out) and `4` when the I/O endpoint cannot listen. `adapter` exits `4` when it cannot listen on its port.

**`--debug`:** adds go-ethernetip's protocol log and, after a fatal error, the chain of causes (`cause: …` lines) on stderr. It does not change stdout or the exit code.
