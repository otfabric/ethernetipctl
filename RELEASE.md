# ethernetipctl Releases

## v0.1.0

**Date:** unreleased (released once go-ethernetip v0.1.0 is tagged)  
**Previous release:** none

### Summary

The first release of ethernetipctl: an EtherNet/IP and CIP command-line originator, diagnostics tool and adapter simulator, built on go-ethernetip and following the structure and conventions of dnp3ctl (`--format text|json|table`, a JSON error object, stable exit codes, `ETHERNETIPCTL_*` environment variables).

### Before tagging

- go-ethernetip is not published yet: `go.mod` requires `github.com/otfabric/go-ethernetip v0.0.0` and replaces it with `../go-ethernetip`. When go-ethernetip v0.1.0 is tagged, require `v0.1.0`, **remove the replace directive**, run `go mod tidy`, and remove the temporary `sibling` job of `.github/workflows/ci.yml` (the `cross` job then needs no second checkout either). The release workflow builds from this repository alone and cannot resolve the replace.

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
