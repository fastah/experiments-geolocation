# experiments-geolocation

Experiments in IP-based geolocation.

## Goals

This repository is intended to share reproducible geolocation stats, including:

- Per-RIR distribution of RDAP network IDs.
- Per-RIR network size by host bits.
- Additional per-RIR comparative stats as the experiments evolve.

## Tools

- [cmd/rir-rfc8805](cmd/rir-rfc8805): enrich RFC 8805-style geofeed CSV rows with RDAP-derived RIR and network metadata.

## Build

```bash
go build ./cmd/rir-rfc8805
```

## Test

```bash
go test ./...
```
