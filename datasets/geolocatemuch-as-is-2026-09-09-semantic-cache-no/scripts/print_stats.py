#!/usr/bin/env python3
"""Print reproducible stats for the 2026-09-09 geolocatemuch RDAP run."""

from __future__ import annotations

import argparse
import collections
import csv
import gzip
import ipaddress
import json
import statistics
from pathlib import Path
from typing import Any

RIR_ORDER = ["afrinic", "apnic", "arin", "lacnic", "ripe", "unknown"]
RIR_LABELS = {
    "afrinic": "AFRINIC",
    "apnic": "APNIC",
    "arin": "ARIN",
    "lacnic": "LACNIC",
    "ripe": "RIPE NCC",
    "unknown": "Unknown/error",
}


def bucket() -> dict[str, Any]:
    return {
        "successfulRows": 0,
        "errorRows": 0,
        "prefixes": set(),
        "networkNames": collections.Counter(),
        "finalHosts": collections.Counter(),
        "hostBits": collections.Counter(),
    }


def sort_rir(rir: str) -> tuple[int, str]:
    if rir in RIR_ORDER:
        return RIR_ORDER.index(rir), rir
    return 999, rir


def pct(part: int | float, whole: int | float) -> float:
    if whole == 0:
        return 0.0
    return float(part) / float(whole) * 100.0


def parse_host_bits(prefix: str) -> str:
    network = ipaddress.ip_network(prefix, strict=False)
    return str(network.max_prefixlen - network.prefixlen)


def read_jsonl(path: Path) -> tuple[dict[str, dict[str, Any]], dict[str, Any]]:
    per_rir = {rir: bucket() for rir in RIR_ORDER}
    bad_json_lines = 0
    json_rows = 0
    error_examples = []
    cross_rir_transitions = collections.Counter()
    cross_rir_examples = []

    opener = gzip.open if path.suffix == ".gz" else open
    with opener(path, "rt") as handle:
        for line_number, line in enumerate(handle, 1):
            json_rows += 1
            try:
                row = json.loads(line)
            except Exception as exc:
                bad_json_lines += 1
                if len(error_examples) < 20:
                    error_examples.append({"line": line_number, "error": str(exc), "raw": line[:300]})
                continue

            if row.get("error"):
                rir = (row.get("rir") or row.get("finalRir") or row.get("bootstrapRir") or "unknown").lower()
                per_rir.setdefault(rir, bucket())["errorRows"] += 1
                if len(error_examples) < 20:
                    error_examples.append(row)
                continue

            rir = (row.get("rir") or row.get("finalRir") or row.get("bootstrapRir") or "unknown").lower()
            data = per_rir.setdefault(rir, bucket())
            data["successfulRows"] += 1
            prefix = row.get("prefix") or ""
            if prefix:
                data["prefixes"].add(prefix)
                try:
                    data["hostBits"][parse_host_bits(prefix)] += 1
                except ValueError:
                    data["hostBits"]["invalid"] += 1
            network_name = row.get("networkName") or ""
            if network_name:
                data["networkNames"][network_name] += 1
            final_host = row.get("finalRdapHost") or ""
            if final_host:
                data["finalHosts"][final_host] += 1

            bootstrap_rir = (row.get("bootstrapRir") or "").lower()
            final_rir = (row.get("finalRir") or "").lower()
            if bootstrap_rir and final_rir and bootstrap_rir != final_rir:
                transition = f"{bootstrap_rir}->{final_rir}"
                cross_rir_transitions[transition] += 1
                if len(cross_rir_examples) < 20:
                    cross_rir_examples.append(row)

    metadata = {
        "badJsonLines": bad_json_lines,
        "crossRirExamples": cross_rir_examples,
        "crossRirTransitions": dict(cross_rir_transitions),
        "errorExamples": error_examples,
        "jsonRows": json_rows,
        "source": str(path),
    }
    return per_rir, metadata


def summarize_per_rir(per_rir: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    rows = []
    for rir in sorted(per_rir, key=sort_rir):
        data = per_rir[rir]
        counts = list(data["networkNames"].values())
        successful = int(data["successfulRows"])
        top10 = sum(count for _, count in data["networkNames"].most_common(10))
        top_host = ""
        top_host_rows = 0
        if data["finalHosts"]:
            top_host, top_host_rows = data["finalHosts"].most_common(1)[0]
        rows.append(
            {
                "rir": rir,
                "successfulRows": successful,
                "errorRows": int(data["errorRows"]),
                "distinctPrefixes": len(data["prefixes"]),
                "distinctNetworkNames": len(data["networkNames"]),
                "networkNamesWithOneRow": sum(1 for count in counts if count == 1),
                "top10NetworkNameRows": top10,
                "top10NetworkNameSharePct": round(pct(top10, successful), 2),
                "medianRowsPerNetworkName": statistics.median(counts) if counts else 0,
                "topFinalRdapHost": top_host,
                "topFinalRdapHostRows": top_host_rows,
            }
        )
    return rows


def write_csv(path: Path, rows: list[dict[str, Any]], fields: list[str]) -> None:
    with path.open("w", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)


def write_outputs(per_rir: dict[str, dict[str, Any]], metadata: dict[str, Any], output_dir: Path) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    per_rows = summarize_per_rir(per_rir)
    successful_total = sum(row["successfulRows"] for row in per_rows if row["rir"] != "unknown")
    distinct_total = sum(row["distinctNetworkNames"] for row in per_rows if row["rir"] != "unknown")
    error_rows = sum(row["errorRows"] for row in per_rows)

    for row in per_rows:
        row["successfulRowSharePct"] = round(pct(row["successfulRows"], successful_total), 2) if row["rir"] != "unknown" else 0
        row["distinctNetworkNameSharePct"] = round(pct(row["distinctNetworkNames"], distinct_total), 2) if row["rir"] != "unknown" else 0

    write_csv(
        output_dir / "per-rir-distribution.csv",
        per_rows,
        [
            "rir",
            "successfulRows",
            "successfulRowSharePct",
            "errorRows",
            "distinctPrefixes",
            "distinctNetworkNames",
            "distinctNetworkNameSharePct",
            "networkNamesWithOneRow",
            "top10NetworkNameRows",
            "top10NetworkNameSharePct",
            "medianRowsPerNetworkName",
            "topFinalRdapHost",
            "topFinalRdapHostRows",
        ],
    )

    host_bit_rows = []
    for rir in sorted(per_rir, key=sort_rir):
        if rir == "unknown":
            continue
        rir_successful = int(per_rir[rir]["successfulRows"])
        for host_bits, rows in sorted(per_rir[rir]["hostBits"].items(), key=lambda item: (item[0] == "invalid", int(item[0]) if item[0].isdigit() else 9999)):
            host_bit_rows.append(
                {
                    "rir": rir,
                    "hostBits": host_bits,
                    "rows": rows,
                    "sharePct": round(pct(rows, rir_successful), 2),
                }
            )
    write_csv(output_dir / "network-size-by-host-bits.csv", host_bit_rows, ["rir", "hostBits", "rows", "sharePct"])

    top_rows = []
    for row in per_rows:
        rir = row["rir"]
        if rir == "unknown":
            continue
        successful = row["successfulRows"]
        for rank, (name, rows) in enumerate(per_rir[rir]["networkNames"].most_common(25), 1):
            top_rows.append({"rir": rir, "rank": rank, "networkName": name, "rows": rows, "sharePct": round(pct(rows, successful), 2)})
    write_csv(output_dir / "top-networknames-by-rir.csv", top_rows, ["rir", "rank", "networkName", "rows", "sharePct"])

    summary = {
        **metadata,
        "errorRows": error_rows,
        "successfulRows": successful_total,
        "distinctNetworkNames": distinct_total,
        "perRir": per_rows,
    }
    (output_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
    (output_dir / "error-examples.json").write_text(json.dumps({"errorRows": error_rows, "examples": metadata["errorExamples"]}, indent=2, sort_keys=True) + "\n")
    (output_dir / "SUMMARY.md").write_text(render_markdown(summary, per_rows, host_bit_rows) + "\n")


def render_markdown(summary: dict[str, Any], per_rows: list[dict[str, Any]], host_bit_rows: list[dict[str, Any]]) -> str:
    rows = [row for row in per_rows if row["rir"] != "unknown"]
    host_bits_by_rir: dict[str, list[dict[str, Any]]] = collections.defaultdict(list)
    for row in host_bit_rows:
        host_bits_by_rir[row["rir"]].append(row)
    lines = [
        "# 2026-09-09 geolocatemuch RDAP/RIR dataset summary",
        "",
        "Input was `geolocatemuch-as-is-09-Sep-2026_shuffled.csv`; the CSV input was shuffled before collection. The `rir-rfc8805` run used `semantic-cache=false`.",
        "",
        "## Run totals",
        "",
        "| Metric | Value |",
        "| --- | ---: |",
        f"| JSONL rows | {summary['jsonRows']:,} |",
        f"| Successful rows | {summary['successfulRows']:,} |",
        f"| RDAP row errors | {summary['errorRows']:,} |",
        f"| Error rate | {pct(summary['errorRows'], summary['jsonRows']):.2f}% |",
        f"| Bad JSON lines | {summary['badJsonLines']:,} |",
        f"| Distinct RDAP network IDs | {summary['distinctNetworkNames']:,} |",
        "",
        "## Per-RIR distribution",
        "",
        "| RIR | Successful rows | Row share | Distinct network IDs | Network ID share | Top-10 networkName row share | One-row network IDs |",
        "| --- | ---: | ---: | ---: | ---: | ---: | ---: |",
    ]
    for row in rows:
        one_row_share = pct(row["networkNamesWithOneRow"], row["distinctNetworkNames"])
        lines.append(
            f"| {RIR_LABELS.get(row['rir'], row['rir'])} | "
            f"{row['successfulRows']:,} | {row['successfulRowSharePct']:.2f}% | "
            f"{row['distinctNetworkNames']:,} | {row['distinctNetworkNameSharePct']:.2f}% | "
            f"{row['top10NetworkNameSharePct']:.2f}% | {row['networkNamesWithOneRow']:,} ({one_row_share:.2f}%) |"
        )
    lines.extend(
        [
            "",
            "## Network size by host bits",
            "",
            "Top host-bit buckets per RIR, where host bits are `32 - prefix length` for IPv4 and `128 - prefix length` for IPv6.",
            "",
            "| RIR | Top host-bit buckets by row count |",
            "| --- | --- |",
        ]
    )
    for row in rows:
        buckets = host_bits_by_rir.get(row["rir"], [])
        buckets.sort(key=lambda item: int(item["rows"]), reverse=True)
        top_buckets = ", ".join(f"{item['hostBits']} host bits: {int(item['rows']):,} ({float(item['sharePct']):.2f}%)" for item in buckets[:8])
        lines.append(f"| {RIR_LABELS.get(row['rir'], row['rir'])} | {top_buckets} |")
    lines.extend(
        [
            "",
            "## Reproducibility notes",
            "",
            "- The raw JSONL contains one object per processed geofeed prefix row.",
            "- Per-row RDAP failures are present as JSON objects with an `error` field and are excluded from successful-row metrics.",
            "- `network-size-by-host-bits.csv` computes host bits from the emitted `prefix`: IPv4 uses `32 - prefix length`; IPv6 uses `128 - prefix length`.",
            "- `networkName` is RDAP-derived and may reflect registry-specific naming conventions rather than a normalized organization identity.",
        ]
    )
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, default=Path("data/rir-rfc8805-output.jsonl.gz"))
    parser.add_argument("--output-dir", type=Path, default=Path("generated"))
    args = parser.parse_args()

    per_rir, metadata = read_jsonl(args.input)
    write_outputs(per_rir, metadata, args.output_dir)
    print((args.output_dir / "SUMMARY.md").read_text())


if __name__ == "__main__":
    main()
