"""Stage reviewed brand/product and artwork-variant consolidation.

Never edits the live library. The report records artwork paths for download-count
migration and retired detail IDs for permanent redirects. Keep the original tree
as a recoverable backup when installing the staged library.
"""
import argparse
import copy
import hashlib
import json
import re
import shutil
from pathlib import Path

SUFFIX = re.compile(r"-(?:wordmark|light|dark|logo|symbol|icon|full|black|white|alt|flat)$")
FAMILIES = ('adobe', 'amazon', 'apache', 'apple', 'asus', 'atlassian',
            'cloudflare', 'google', 'hashicorp', 'libreoffice', 'microsoft',
            'nextcloud', 'proton', 'qnap', 'vmware')


def artwork_base(stem, available):
    candidate, best = stem, stem
    while True:
        shorter = SUFFIX.sub('', candidate)
        if shorter == candidate:
            return best
        candidate = shorter
        if candidate in available:
            best = candidate


def filename(slug, key, version):
    return slug + ('-' + key if key else '') + ('-' + str(version) if version else '') + '.svg'


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('source', type=Path)
    p.add_argument('stage', type=Path)
    p.add_argument('--report', required=True, type=Path)
    args = p.parse_args()
    if args.stage.exists():
        raise SystemExit('stage already exists')
    shutil.copytree(args.source, args.stage, ignore=shutil.ignore_patterns('.library.lock', '.pending.json', '.tmp-*'))
    metas = {f.parent.name: json.loads(f.read_text(encoding='utf-8-sig')) for f in args.stage.glob('*/meta.json')}
    report = {'deleted': [], 'merges': [], 'products': [], 'artwork_moves': [], 'renames': []}
    original_icons = {}
    original_groups = set()
    original_defaults = {}

    def groups():
        for slug, meta in metas.items():
            for g in [meta, *meta.get('products', [])]:
                yield slug, g

    for slug, g in groups():
        original_groups.add(g['id'])
        original_groups.update(g.get('redirect_from', []))
        for i in g['icons']:
            original_icons[i['id']] = hashlib.sha256((args.stage / slug / i['file']).read_bytes()).hexdigest()
            if i.get('is_default') and not i.get('deleted'):
                original_defaults[g['id']] = i['id']

    def move_icons(src_slug, src, dst_slug, dst, merge):
        for i in src['icons']:
            old = src_slug + '/' + i['file']
            version = max([dst.get('next_version', 0), *(x['version'] + 1 for x in dst['icons'])]) if merge else i['version']
            new_file = filename(dst_slug, dst.get('key', ''), version)
            new = dst_slug + '/' + new_file
            if old != new:
                dest = args.stage / new
                dest.parent.mkdir(parents=True, exist_ok=True)
                if dest.exists():
                    raise ValueError('artwork path collision: ' + new)
                shutil.move(str(args.stage / old), str(dest))
                report['artwork_moves'].append({'id': i['id'], 'old': old, 'new': new})
            i['file'], i['version'] = new_file, version
            if merge:
                i['is_default'] = False
            dst['icons'].append(i)
            dst['next_version'] = max(dst.get('next_version', 0), version + 1)

    def retire(slug, dst_slug):
        for other in metas.values():
            if other.get('parent_brand') == slug:
                other['parent_brand'] = dst_slug
        # All SVGs have been moved; retain any unexpected files by failing.
        folder = args.stage / slug
        (folder / 'meta.json').unlink()
        folder.rmdir()
        del metas[slug]

    def merge(src_slug, dst_slug, dst=None):
        src = metas[src_slug]
        dst = metas[dst_slug] if dst is None else dst
        if src.get('products') or src.get('deleted') or dst.get('deleted'):
            raise ValueError('unsafe merge: ' + src_slug)
        move_icons(src_slug, src, dst_slug, dst, True)
        dst['aliases'] = list(dict.fromkeys([*dst.get('aliases', []), src_slug, src['title'], *src.get('aliases', [])]))
        dst['redirect_from'] = sorted(set([*dst.get('redirect_from', []), src['id'], *src.get('redirect_from', [])]))
        report['merges'].append({'source': src_slug, 'old_id': src['id'], 'brand': dst_slug, 'target_id': dst['id']})
        retire(src_slug, dst_slug)

    def product(src_slug, dst_slug, key=None):
        src, parent = metas[src_slug], metas[dst_slug]
        if src.get('products') or src.get('deleted'):
            return
        key = key or src_slug.removeprefix(dst_slug + '-')
        existing = next((x for x in parent.get('products', []) if x['key'] == key), None)
        if existing:
            merge(src_slug, dst_slug, existing)
            return
        dst = {k: copy.deepcopy(v) for k, v in src.items() if k not in ('schema_version', 'collection_id', 'parent_brand', 'products', 'icons')}
        dst.update(key=key, icons=[])
        dst['aliases'] = list(dict.fromkeys([*dst.get('aliases', []), src_slug]))
        move_icons(src_slug, src, dst_slug, dst, False)
        parent.setdefault('products', []).append(dst)
        report['products'].append({'source': src_slug, 'id': dst['id'], 'brand': dst_slug, 'key': key})
        retire(src_slug, dst_slug)

    deleted = set(range(1016, 1028)) | {147}
    for slug, meta in metas.items():
        if meta['id'] in deleted:
            meta['deleted'] = True
            report['deleted'].append({'slug': slug, 'id': meta['id'], 'title': meta['title']})
    assert len(report['deleted']) == 13

    # Existing Microsoft Azure is the canonical collection, including defaults.
    azure = next(g for g in metas['microsoft']['products'] if g['key'] == 'azure')
    new = {k: copy.deepcopy(v) for k, v in azure.items() if k not in ('key', 'icons')}
    new.update(schema_version=1, collection_id=max(m['collection_id'] for m in metas.values()) + 1,
               parent_brand='microsoft', title='Azure', category='云服务', icons=[], products=[])
    metas['azure'] = new
    move_icons('microsoft', azure, 'azure', new, False)
    metas['microsoft']['products'].remove(azure)
    report['renames'].append({'old': 'microsoft/azure', 'new': 'azure', 'id': new['id']})

    # Collapse artwork variants before moving products, including chained suffixes.
    for slug in sorted(list(metas), key=lambda s: (-len(s), s)):
        if slug not in metas or metas[slug].get('deleted') or metas[slug].get('products'):
            continue
        base = artwork_base(slug, metas)
        if base != slug and not metas[base].get('deleted'):
            merge(slug, base)

    # Office.eu's product family uses EU-prefixed upstream names.
    old = metas.pop('office-eu')
    old['title'] = 'EU / Office.eu'
    old['aliases'] = list(dict.fromkeys([*old.get('aliases', []), 'office-eu', 'Office.eu', 'EU']))
    old['url'] = 'https://office.eu/'
    metas['eu'] = old
    for g in [old, *old.get('products', [])]:
        icons = g['icons']
        g['icons'] = []
        move_icons('office-eu', {'icons': icons}, 'eu', g, False)
    (args.stage / 'office-eu' / 'meta.json').unlink()
    (args.stage / 'office-eu').rmdir()
    report['renames'].append({'old': 'office-eu', 'new': 'eu', 'id': old['id']})
    for slug in sorted(list(metas)):
        if slug.startswith('azure-'):
            product(slug, 'azure')
        elif slug.startswith('eu-'):
            product(slug, 'eu')
    product('adguard-home-sync', 'adguard', 'home-sync')
    product('docker-engine', 'docker', 'engine')
    product('docker-moby', 'docker', 'moby')
    merge('font-awesome-icons', 'fontawesome')
    # User explicitly rejected this visually identical alternative. Retain bytes
    # and the asset ID in the recycle bin rather than destroy source artwork.
    next(i for i in metas['fontawesome']['icons'] if i['id'] == 2699)['deleted'] = True
    for family in FAMILIES:
        if family not in metas:
            continue
        for slug in sorted(list(metas)):
            if slug.startswith(family + '-') and not metas[slug].get('deleted'):
                product(slug, family)

    ids, aliases, icon_ids, defaults = set(), set(), set(), {}
    for slug, g in groups():
        assert g['id'] not in ids
        ids.add(g['id'])
        for alias in g.get('redirect_from', []):
            assert alias not in aliases
            aliases.add(alias)
        active_defaults = []
        for i in g['icons']:
            assert i['id'] not in icon_ids
            icon_ids.add(i['id'])
            assert filename(slug, g.get('key', ''), i['version']) == i['file']
            assert hashlib.sha256((args.stage / slug / i['file']).read_bytes()).hexdigest() == original_icons[i['id']]
            if i.get('is_default') and not i.get('deleted'):
                active_defaults.append(i['id'])
        active_icons = [i for i in g['icons'] if not i.get('deleted')]
        assert len(active_defaults) == (1 if active_icons else 0), (slug, g.get('key'), active_defaults)
        if active_defaults:
            defaults[g['id']] = active_defaults[0]
    assert not ids.intersection(aliases)
    assert ids | aliases == original_groups
    assert icon_ids == set(original_icons)
    assert all(defaults[gid] == default for gid, default in original_defaults.items() if gid in defaults)
    for slug, meta in metas.items():
        assert not meta.get('parent_brand') or meta['parent_brand'] in metas
        (args.stage / slug / 'meta.json').write_text(json.dumps(meta, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    report['summary'] = {'brands': len(metas), 'products': sum(len(m.get('products', [])) for m in metas.values()),
                         'icons_preserved': len(icon_ids), 'merged_collections': len(report['merges']),
                         'products_reassigned': len(report['products']), 'deleted_collections': 13}
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(report['summary']))


if __name__ == '__main__':
    main()
