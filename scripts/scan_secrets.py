#!/usr/bin/env python3
"""Scan Git history with TruffleHog without printing credential values."""

import argparse
import json
from pathlib import Path
import shutil
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--since-commit")
    parser.add_argument("--branch")
    args = parser.parse_args()

    scanner = shutil.which("trufflehog")
    if scanner is None:
        print("TruffleHog is missing from PATH; push blocked.", file=sys.stderr)
        return 1

    git_dir = subprocess.check_output(
        ["git", "rev-parse", "--absolute-git-dir"], text=True
    ).strip()
    command = [
        scanner, "git", Path(git_dir).as_uri(),
        "--bare", "--trust-local-git-config",
        "--no-verification", "--no-update", "--json",
        "--fail", "--fail-on-scan-errors", "--concurrency=4",
    ]
    if args.since_commit:
        command.extend(["--since-commit", args.since_commit])
    if args.branch:
        command.extend(["--branch", args.branch])

    # This is the user's trusted repository. Scan its object store directly
    # to exclude the working tree and index without making another clone.
    result = subprocess.run(command, capture_output=True, text=True)
    findings = 0
    for line in result.stdout.splitlines():
        finding = json.loads(line)
        metadata = finding["SourceMetadata"]["Data"]["Git"]
        print(json.dumps({
            "detector": finding["DetectorName"],
            "file": metadata["file"],
            "line": metadata.get("line"),
            "commit": metadata["commit"],
        }), flush=True)
        findings += 1

    if result.returncode not in (0, 183):
        print(f"TruffleHog scan failed (exit {result.returncode}); push blocked.",
              file=sys.stderr)
        # Scanner diagnostics can contain sensitive context. Do not echo them.
        return 1
    if findings or result.returncode == 183:
        print(f"TruffleHog: {findings} potential credential findings; push blocked.",
              file=sys.stderr)
        return 183
    print("TruffleHog: no credential findings.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
