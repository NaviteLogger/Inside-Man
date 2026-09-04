#!/usr/bin/env bash
# Every dependency in Chart.yaml must have a matching vendored tarball.
#
# Helm renders from charts/*.tgz when they are present and ignores the version
# in Chart.yaml, so a bump that updates Chart.yaml without the tarball is a
# silent no-op that still passes lint and template. That is how grafana 13.1.0
# landed while the cluster kept running 13.0.1.
#
# See docs/decisions/0009-grafana-helm-chart-repo-split.md for why the tarballs
# are vendored at all.
set -euo pipefail

CHART_DIR="${1:-charts/inside-man}"

python3 - "${CHART_DIR}" <<'PY'
import pathlib
import re
import sys

chart_dir = pathlib.Path(sys.argv[1])
chart_yaml = (chart_dir / "Chart.yaml").read_text()
vendored = {p.name for p in (chart_dir / "charts").glob("*.tgz")}

# The dependency block is regular enough to read without a YAML parser, which
# keeps this script dependency-free.
declared = re.findall(
    r"- name:\s*(\S+)\s*\n\s*version:\s*(\S+)", chart_yaml
)

problems = []
for name, version in declared:
    expected = f"{name}-{version}.tgz"
    if expected not in vendored:
        actual = sorted(f for f in vendored if f.startswith(f"{name}-"))
        problems.append(
            f"  {name}: Chart.yaml wants {version}, vendored is "
            f"{', '.join(actual) if actual else 'nothing'}"
        )

expected_all = {f"{n}-{v}.tgz" for n, v in declared}
for orphan in sorted(vendored - expected_all):
    problems.append(f"  {orphan} is vendored but not declared in Chart.yaml")

if problems:
    print("Vendored charts are out of step with Chart.yaml:", file=sys.stderr)
    print("\n".join(problems), file=sys.stderr)
    print("\nRun: helm dependency update " + str(chart_dir), file=sys.stderr)
    sys.exit(1)

print(f"vendored charts match Chart.yaml ({len(declared)} dependencies)")
PY
