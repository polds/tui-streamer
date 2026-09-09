#!/usr/bin/env python3
"""Print BundleSet metadata.name, or the first Bundle metadata.name.

Matches internal/bundle.File.Name so the packaged .app name matches the window title.
"""
import sys


def _field(line, key):
    if not line.startswith(key + ":"):
        return None
    return line.split(":", 1)[1].strip().strip("\"'")


def bundle_name(path):
    first_bundle = ""
    kind = ""
    in_metadata = False
    with open(path, encoding="utf-8") as f:
        for line in f:
            stripped = line.strip()
            if stripped == "---" or stripped.startswith("#"):
                kind = ""
                in_metadata = False
                continue
            value = _field(stripped, "kind")
            if value is not None:
                kind = value
                in_metadata = False
                continue
            if stripped.startswith("metadata:"):
                in_metadata = True
                continue
            if not in_metadata:
                continue
            name = _field(stripped, "name")
            if name is None:
                continue
            if kind == "BundleSet":
                return name
            if kind == "Bundle" and not first_bundle:
                first_bundle = name
            in_metadata = False
    return first_bundle


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("usage: bundle-name.py <bundle.yaml>")
    print(bundle_name(sys.argv[1]), end="")
