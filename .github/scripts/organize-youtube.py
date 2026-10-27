"""Stage YouTube consolidation into the existing Google product collection."""
import argparse
import copy
import hashlib
import json
import shutil
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('library', type=Path)
p.add_argument('stage', type=Path)
p.add_argument('--report', required=True, type=Path)
args = p.parse_args()
if args.stage.exists():
    raise SystemExit('stage exists')
sources = ['youtube', 'youtube-kids', 'youtube-studio', 'youtube-tv']
parent = json.loads((args.library / 'google/meta.json').read_text(encoding='utf-8'))
canonical = next(g for g in parent['products'] if g['key'] == 'youtube')
original_default = [i['id'] for i in canonical['icons'] if i.get('is_default')]
shutil.copytree(args.library / 'google', args.stage / 'google')
report = {'sources': sources, 'artwork_moves': [], 'products': [], 'merged': [],
          'original_default_ids': original_default}
for slug in sources:
    source = json.loads((args.library / slug / 'meta.json').read_text(encoding='utf-8'))
    assert not source.get('products') and not source.get('deleted'), slug
    merge = slug == 'youtube'
    if merge:
        group = canonical
        group['redirect_from'] = sorted(set([*group.get('redirect_from', []), source['id'], *source.get('redirect_from', [])]))
        group['aliases'] = list(dict.fromkeys([*group.get('aliases', []), *source.get('aliases', [])]))
        report['merged'].append({'source': slug, 'old_id': source['id'], 'target_id': group['id']})
    else:
        key = slug
        assert not any(g['key'] == key for g in parent['products']), key
        group = {k: copy.deepcopy(v) for k, v in source.items() if k not in ('schema_version', 'collection_id', 'parent_brand', 'products', 'icons')}
        group.update(key=key, icons=[], title={'youtube-kids': 'YouTube Kids', 'youtube-studio': 'YouTube Studio', 'youtube-tv': 'YouTube TV'}[slug])
        parent['products'].append(group)
        report['products'].append({'source': slug, 'id': group['id'], 'brand': 'google', 'key': key})
    for original in source['icons']:
        icon = copy.deepcopy(original)
        version = max([group.get('next_version', 0), *(i['version'] + 1 for i in group['icons'])]) if merge else icon['version']
        name = 'google-' + group['key'] + (('-' + str(version)) if version else '') + '.svg'
        old, new = args.library / slug / original['file'], args.stage / 'google' / name
        assert not new.exists(), new
        shutil.copy2(old, new)
        assert hashlib.sha256(old.read_bytes()).digest() == hashlib.sha256(new.read_bytes()).digest()
        icon.update(file=name, version=version)
        if merge:
            icon['is_default'] = False
        group['icons'].append(icon)
        group['next_version'] = max(group.get('next_version', 0), version + 1)
        report['artwork_moves'].append({'id': icon['id'], 'old': slug + '/' + original['file'], 'new': 'google/' + name})
assert [i['id'] for i in canonical['icons'] if i.get('is_default')] == original_default
(args.stage / 'google/meta.json').write_text(json.dumps(parent, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print('YouTube versions:', len(canonical['icons']), 'Google products:', len(parent['products']))
