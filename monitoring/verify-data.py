#!/usr/bin/env python3
"""Compare Grafana's current metric values with independently computed report totals."""
import argparse
import base64
import datetime
import json
import math
import os
from pathlib import Path
import urllib.parse
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--reports', default=str(Path(__file__).resolve().parent.parent / 'gremlin-reports.json'))
parser.add_argument('--grafana', default='http://localhost:3001')
parser.add_argument('--output', help='Optional JSON audit output path')
args = parser.parse_args()
rows = json.loads(Path(args.reports).read_text())
user = os.environ.get('GRAFANA_USER', 'admin')
password = os.environ.get('GRAFANA_PASSWORD', 'admin')
auth = 'Basic ' + base64.b64encode(f'{user}:{password}'.encode()).decode()

def get(path):
    req = urllib.request.Request(args.grafana.rstrip('/') + path, headers={'Authorization': auth})
    with urllib.request.urlopen(req, timeout=10) as response:
        return json.load(response)

def query(expression):
    result = get('/api/datasources/proxy/uid/PBFA97CFB590B2093/api/v1/query?' + urllib.parse.urlencode({'query': expression}))
    assert result['status'] == 'success', result
    return result['data']['result']

def outcome(row):
    status = row.get('recovery_status')
    if status in ('verified', 'skipped', 'unverified'):
        return status
    return 'verified' if row['recovery_ms'] > 0 else 'unknown'

def valid(row):
    return outcome(row) == 'verified' and row['recovery_ms'] >= 0

def timestamp(row):
    return datetime.datetime.fromisoformat(row['started_at'].replace('Z', '+00:00')).timestamp()

n = len(rows)
failed = sum(not r['success'] for r in rows)
verified = sum(valid(r) for r in rows)
latest = max(rows, key=timestamp) if rows else None
expected = {
    'gremlin_attacks_total': n,
    'gremlin_attacks_failed_total': failed,
    'gremlin_success_rate_percent': 100 * (n - failed) / n if n else math.nan,
    'gremlin_recovery_verified_total': verified,
    'gremlin_recovery_unverified_total': n - verified,
    'gremlin_recovery_coverage_percent': 100 * verified / n if n else math.nan,
    'gremlin_recovery_skipped_total': sum(outcome(r) == 'skipped' for r in rows),
    'gremlin_recovery_unknown_total': sum(outcome(r) == 'unknown' for r in rows),
    'gremlin_last_recovery_ms': latest['recovery_ms'] if latest and valid(latest) else math.nan,
    'gremlin_last_duration_ms': latest['duration_ms'] if latest else math.nan,
    'gremlin_last_report_timestamp_seconds': math.floor(timestamp(latest)) if latest else math.nan,
}
checks = []

def check(expression, want):
    result = query(expression)
    assert len(result) == 1, f'{expression}: expected one series, got {len(result)}'
    value = float(result[0]['value'][1])
    assert (math.isnan(want) and math.isnan(value)) or math.isclose(value, want, rel_tol=1e-9, abs_tol=1e-9), f'{expression}: {value} != {want}'
    checks.append({'expression': expression, 'value': None if math.isnan(value) else value, 'matches_reports': True})

for expression, want in expected.items():
    check(expression, want)

for attack in sorted({r['attack'] for r in rows}):
    group = [r for r in rows if r['attack'] == attack]
    measured = [r for r in group if valid(r)]
    label = '{attack=' + json.dumps(attack) + '}'
    check('gremlin_attacks_by_type_total' + label, len(group))
    check('gremlin_attacks_failed_by_type_total' + label, sum(not r['success'] for r in group))
    check('gremlin_recovery_verified_by_type' + label, len(measured))
    check('gremlin_avg_duration_ms_by_type' + label, sum(r['duration_ms'] for r in group) / len(group))
    check('gremlin_avg_recovery_ms_by_type' + label, sum(r['recovery_ms'] for r in measured) / len(measured) if measured else math.nan)

check('up{job="gremlin"}', 1)
dashboard = get('/api/dashboards/uid/eebaf189-7d05-426b-8ce7-9a18b251988d')['dashboard']
for panel in dashboard['panels']:
    for target in panel.get('targets', []):
        assert query(target['expr']), f'No series for panel: {panel["title"]}'

# Fail the audit if the file changed while the snapshot was being checked.
assert json.loads(Path(args.reports).read_text()) == rows, 'Reports changed during verification; run the audit again.'
audit = {'checked_at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'reports': n, 'verified_recovery': verified, 'unverified_recovery': n - verified, 'dashboard': dashboard['title'], 'checks': checks}
if args.output:
    Path(args.output).write_text(json.dumps(audit, indent=2) + '\n')
print(f'PASS: {len(checks)} metric comparisons match {n} saved reports.')
print(f'Recovery evidence: {verified} measured; {n - verified} unverified.')
print('All displayed metric queries return series through Grafana.')
