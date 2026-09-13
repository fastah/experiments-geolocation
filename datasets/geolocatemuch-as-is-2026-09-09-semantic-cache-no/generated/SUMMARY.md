# 2026-09-09 geolocatemuch RDAP/RIR dataset summary

Input was `geolocatemuch-as-is-09-Sep-2026_shuffled.csv`; the CSV input was shuffled before collection. The `rir-rfc8805` run used `semantic-cache=false`.

## Run totals

| Metric | Value |
| --- | ---: |
| JSONL rows | 621,297 |
| Successful rows | 614,895 |
| RDAP row errors | 6,402 |
| Error rate | 1.03% |
| Bad JSON lines | 0 |
| Distinct RDAP network IDs | 84,860 |

## Per-RIR distribution

| RIR | Successful rows | Row share | Distinct network IDs | Network ID share | Top-10 networkName row share | One-row network IDs |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| AFRINIC | 8,828 | 1.44% | 1,279 | 1.51% | 52.82% | 810 (63.33%) |
| APNIC | 20,838 | 3.39% | 2,282 | 2.69% | 40.50% | 1,512 (66.26%) |
| ARIN | 213,444 | 34.71% | 18,113 | 21.34% | 39.57% | 14,742 (81.39%) |
| LACNIC | 4,186 | 0.68% | 1,057 | 1.25% | 27.64% | 813 (76.92%) |
| RIPE NCC | 367,599 | 59.78% | 62,129 | 73.21% | 44.57% | 43,515 (70.04%) |

## Network size by host bits

Top host-bit buckets per RIR, where host bits are `32 - prefix length` for IPv4 and `128 - prefix length` for IPv6.

| RIR | Top host-bit buckets by row count |
| --- | --- |
| AFRINIC | 2 host bits: 2,673 (30.28%), 3 host bits: 2,417 (27.38%), 0 host bits: 1,815 (20.56%), 8 host bits: 1,197 (13.56%), 4 host bits: 291 (3.30%), 5 host bits: 105 (1.19%), 10 host bits: 65 (0.74%), 9 host bits: 55 (0.62%) |
| APNIC | 0 host bits: 6,158 (29.55%), 8 host bits: 5,796 (27.81%), 80 host bits: 2,049 (9.83%), 1 host bits: 874 (4.19%), 84 host bits: 871 (4.18%), 9 host bits: 745 (3.58%), 64 host bits: 715 (3.43%), 88 host bits: 447 (2.15%) |
| ARIN | 0 host bits: 105,264 (49.32%), 8 host bits: 29,642 (13.89%), 2 host bits: 20,748 (9.72%), 3 host bits: 15,995 (7.49%), 1 host bits: 8,502 (3.98%), 64 host bits: 5,333 (2.50%), 4 host bits: 5,241 (2.46%), 5 host bits: 4,715 (2.21%) |
| LACNIC | 8 host bits: 3,471 (82.92%), 2 host bits: 144 (3.44%), 9 host bits: 99 (2.37%), 10 host bits: 61 (1.46%), 82 host bits: 59 (1.41%), 80 host bits: 53 (1.27%), 3 host bits: 42 (1.00%), 12 host bits: 30 (0.72%) |
| RIPE NCC | 0 host bits: 88,374 (24.04%), 83 host bits: 79,645 (21.67%), 3 host bits: 49,263 (13.40%), 8 host bits: 30,305 (8.24%), 80 host bits: 26,467 (7.20%), 64 host bits: 25,087 (6.82%), 72 host bits: 9,902 (2.69%), 2 host bits: 9,702 (2.64%) |

## Reproducibility notes

- The raw JSONL contains one object per processed geofeed prefix row.
- Per-row RDAP failures are present as JSON objects with an `error` field and are excluded from successful-row metrics.
- `network-size-by-host-bits.csv` computes host bits from the emitted `prefix`: IPv4 uses `32 - prefix length`; IPv6 uses `128 - prefix length`.
- `networkName` is RDAP-derived and may reflect registry-specific naming conventions rather than a normalized organization identity.
