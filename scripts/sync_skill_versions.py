#!/usr/bin/env python3
"""Keep bundled skill CLI ranges in sync with the release version."""

from pathlib import Path
import re


ROOT = Path(__file__).resolve().parent.parent
SKILLS = (
    ROOT / "skills/semantic-grouping/SKILL.md",
    ROOT / "skills/answer-semdiff/SKILL.md",
)
VERSION_PATTERN = re.compile(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)")
RANGE_PATTERN = re.compile(r">=\d+\.\d+\.\d+, <\d+\.\d+\.\d+")
PLUGIN_LINE_PATTERN = re.compile(r"plugin \d+\.\d+\.x")


def main() -> None:
    version = (ROOT / "VERSION").read_text(encoding="utf-8").strip()
    match = VERSION_PATTERN.fullmatch(version)
    if match is None:
        raise ValueError(f"invalid product version: {version!r}")

    major, minor = int(match[1]), int(match[2])
    expected_range = f">={major}.{minor}.0, <{major}.{minor + 1}.0"
    expected_plugin_line = f"plugin {major}.{minor}.x"
    for path in SKILLS:
        body = path.read_text(encoding="utf-8")
        if len(RANGE_PATTERN.findall(body)) != 1 or len(PLUGIN_LINE_PATTERN.findall(body)) != 1:
            raise ValueError(f"expected one CLI range and plugin line in {path}")
        updated = RANGE_PATTERN.sub(expected_range, body)
        updated = PLUGIN_LINE_PATTERN.sub(expected_plugin_line, updated)
        if updated != body:
            path.write_text(updated, encoding="utf-8")


if __name__ == "__main__":
    main()
