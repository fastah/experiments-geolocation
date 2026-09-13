# experiments-geolocation

Experiments in IP-based geolocation.

## Goals

This repository is intended to share reproducible geolocation stats, including:

- Per-RIR distribution of RDAP network IDs.
- Per-RIR network size by host bits.
- Additional per-RIR comparative stats as the experiments evolve.

## Tools

- [cmd/rir-rfc8805](cmd/rir-rfc8805): enrich RFC 8805-style geofeed CSV rows with RDAP-derived RIR and network metadata.

## Datasets

- [datasets/geolocatemuch-as-is-2026-09-09-semantic-cache-no](datasets/geolocatemuch-as-is-2026-09-09-semantic-cache-no): public JSONL output and reproducible stats for a shuffled geolocatemuch RDAP/RIR run with `semantic-cache=false`.

## Build

```bash
go build ./cmd/rir-rfc8805
```

## Test

```bash
go test ./...
```
