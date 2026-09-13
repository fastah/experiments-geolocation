.PHONY: all stats checksums clean

all: stats checksums

stats: generated/SUMMARY.md
	@cat generated/SUMMARY.md

checksums: generated/SHA256SUMS
	@cat generated/SHA256SUMS

generated/SUMMARY.md: data/rir-rfc8805-output.jsonl.gz scripts/print_stats.py
	@python3 scripts/print_stats.py --input data/rir-rfc8805-output.jsonl.gz --output-dir generated >/dev/null

generated/SHA256SUMS: data/rir-rfc8805-output.jsonl.gz data/rir-rfc8805-stderr.log generated/SUMMARY.md
	@cd . && shasum -a 256 data/rir-rfc8805-output.jsonl.gz data/rir-rfc8805-stderr.log generated/*.csv generated/*.json generated/SUMMARY.md > generated/SHA256SUMS

clean:
	rm -rf generated
