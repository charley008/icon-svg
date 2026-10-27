"""Preserve download counts after reviewed artwork moves; run with app stopped."""
import argparse
import json
import sqlite3
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('database', type=Path)
p.add_argument('backup', type=Path)
p.add_argument('report', type=Path)
args = p.parse_args()
if not args.database.is_file() or args.backup.exists():
    raise SystemExit('missing database or backup already exists')
report = json.loads(args.report.read_text(encoding='utf-8'))
with sqlite3.connect(args.database) as db:
    with sqlite3.connect(args.backup) as backup:
        db.backup(backup)
    before = db.execute('SELECT COALESCE(SUM(count),0) FROM downloads').fetchone()[0]
    for move in report['artwork_moves']:
        row = db.execute('SELECT count FROM downloads WHERE key=?', (move['old'],)).fetchone()
        if row:
            db.execute('INSERT INTO downloads(key,count) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET count=count+excluded.count', (move['new'], row[0]))
            db.execute('DELETE FROM downloads WHERE key=?', (move['old'],))
    assert db.execute('SELECT COALESCE(SUM(count),0) FROM downloads').fetchone()[0] == before
    assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
print('Database backed up; download totals preserved:', before)
