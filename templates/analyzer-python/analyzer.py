#!/usr/bin/env python3
"""Dependency-free RLViz analyzer template."""
import argparse
import json
import os
import sys

API_VERSION = "rlviz.dev/analyzer/v1alpha1"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("operation", choices=("analyze",))
    parser.add_argument("--request", required=True)
    args = parser.parse_args()
    with open(args.request, "r", encoding="utf-8") as handle:
        request = json.load(handle)
    if request.get("api_version") != API_VERSION or request.get("operation") != args.operation:
        raise ValueError("unsupported analyzer request")

    output = {
        "api_version": API_VERSION,
        "provenance": {
            "name": os.environ["RLVIZ_ANALYZER_NAME"],
            "version": os.environ["RLVIZ_ANALYZER_VERSION"],
            "digest": os.environ["RLVIZ_ANALYZER_DIGEST"],
            "input_digest": os.environ["RLVIZ_ANALYZER_INPUT_DIGEST"],
        },
        "findings": [],
        "signals": [],
    }
    print(json.dumps(output, separators=(",", ":"), ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
