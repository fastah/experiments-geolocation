# geolocatemuch as-is RDAP/RIR dataset, 2026-09-09

This dataset contains `rir-rfc8805` output for a shuffled `geolocatemuch-as-is-09-Sep-2026.csv` input. The run used `semantic-cache=false`, so every emitted successful row reflects its own RDAP lookup path rather than a semantic trie reuse of a wider RDAP network response.

The data is intended for public scrutiny and reproducibility of per-RIR geofeed/RDAP observations, especially:

- Per-RIR distribution of RDAP `networkName` values.
- Per-RIR network size by host bits.
- Other per-RIR comparative stats derived from the JSONL.

## Files

- `data/rir-rfc8805-output.jsonl.gz`: compressed raw command output, one JSON object per processed prefix row.
- `data/rir-rfc8805-stderr.log`: progress and cache/error stats from the run.
- `scripts/print_stats.py`: derives CSV, JSON, and Markdown summaries from the raw JSONL.
- `generated/`: reproducible outputs created by `make stats`.

## Reproduce Stats

Run from this dataset directory:

```bash
make stats
```

To regenerate stats and checksums:

```bash
make all
```

This regenerates:

- `generated/SUMMARY.md`
- `generated/summary.json`
- `generated/per-rir-distribution.csv`
- `generated/network-size-by-host-bits.csv`
- `generated/top-networknames-by-rir.csv`
- `generated/error-examples.json`
- `generated/SHA256SUMS`

Use `make clean` to remove generated outputs.

## Collection Notes

The original input CSV was shuffled with the Unix `shuf` tool before running `rir-rfc8805`. The input CSV is not included here; the included compressed JSONL carries the emitted row number, prefix, query IP, RDAP-derived RIR fields, final RDAP host, and RDAP `networkName` for each successful row.

The run completed with 621,297 JSONL rows and 6,402 per-row RDAP errors. Error rows are included in the JSONL as records with an `error` field and are excluded from successful-row distribution metrics.

The collection command was equivalent to:

```bash
./rir-rfc8805 \
	-input geolocatemuch-as-is-09-Sep-2026_shuffled.csv \
	-semantic-cache=false \
	-max-concurrent-rdap 8 \
	-rdap-timeout 5s \
	-progress-every 1000 \
	> full_slow_stdout.jsonl \
	2> full_slow_stderr.log
```

The final stderr cache lines were:

```text
row errors: 6402
semantic cache: disabled
rdap cache: requests=675909 memoryHits=0 diskHits=40872 misses=635037 hitRate=6.0%
```

## Caveats

- This is observational RDAP/geofeed analysis, not a formal census of all networks in any RIR region.
- `networkName` is registry-provided RDAP metadata and should not be treated as a normalized organization identity.
- RDAP behavior can vary by endpoint, transport path, timeout, and date of collection.
- Network size by host bits is derived from the geofeed prefix emitted in each JSONL row: IPv4 host bits are `32 - prefix length`; IPv6 host bits are `128 - prefix length`.
