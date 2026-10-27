"""Classify only empty/Other categories, retaining all artwork and local choices.

python .github/scripts/classify-library.py --upstream UPSTREAM --report REPORT
Add --apply --backup BACKUP_DIRECTORY after reviewing the proposed report.
"""

import argparse
import collections
import copy
import hashlib
import json
import os
from pathlib import Path
from datetime import datetime, timedelta, timezone
import subprocess
from urllib.parse import unquote, urlparse

from icon_categories import base_candidates, infer_category

ROOT = Path(__file__).resolve().parents[2]


def identifiers(slug, group):
    result = [slug + ("-" + group["key"] if group.get("key") else "")]
    for icon in group.get("icons", []):
        source = icon.get("source", "")
        if "homarr-labs/dashboard-icons/" in source and "/svg/" in source:
            result.append(Path(unquote(urlparse(source).path)).stem)
    return list(dict.fromkeys(result))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--upstream", type=Path)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--backup", type=Path)
    args = parser.parse_args()
    if args.apply and not args.backup:
        parser.error("--apply requires --backup")
    upstream = {}
    if args.upstream:
        flat = args.upstream / "metadata.json"
        if flat.is_file():
            upstream = json.loads(flat.read_text(encoding="utf-8-sig"))
    # Flat metadata includes recently added lower-case functional categories
    # that the older meta/<icon>.json records do not contain.
    variant_parent = {}
    for key, meta in upstream.items():
        for kind in ("colors", "wordmark"):
            for variant in (meta.get(kind) or {}).values():
                if isinstance(variant, str):
                    variant_parent[variant] = key
    originals, metas, records, known = {}, {}, [], {}
    for path in sorted((ROOT / "library").glob("*/meta.json")):
        if path.is_symlink() or path.parent.is_symlink():
            raise ValueError(f"Metadata must be a regular local file: {path}")
        raw = path.read_bytes()
        meta = json.loads(raw.decode("utf-8-sig"))
        originals[path] = raw
        metas[path] = meta
        for group in [meta, *meta.get("products", [])]:
            ids = identifiers(path.parent.name, group)
            labels = list(group.get("tags", []))
            for identifier in ids[:]:
                ids.extend(base_candidates(identifier))
                if identifier in variant_parent:
                    ids.append(variant_parent[identifier])
            ids = list(dict.fromkeys(ids))
            for identifier in ids:
                labels.extend(upstream.get(identifier, {}).get("categories", []))
                if args.upstream:
                    old_meta = args.upstream / "meta" / (identifier + ".json")
                    if old_meta.is_file():
                        labels.extend(json.loads(old_meta.read_text(encoding="utf-8-sig")).get("categories", []))
                if group.get("category") not in (None, "", "其他"):
                    known.setdefault(identifier, group["category"])
            records.append((path, group, ids, labels))
    report = {"mode": "apply" if args.apply else "preview", "generated_at": datetime.now(timezone(timedelta(hours=8))).isoformat(timespec="seconds"), "basis": ["upstream metadata.json and meta/*.json", ".github/scripts/icon_categories.py exact product rules", "same product and wordmark/color variants"], "before": {}, "after": {}, "changes": [], "unresolved": []}
    if args.upstream:
        revision = subprocess.run(["git", "-C", str(args.upstream), "rev-parse", "HEAD"], capture_output=True, text=True)
        if revision.returncode == 0:
            report["upstream_commit"] = revision.stdout.strip()
    before = collections.Counter(group.get("category") or "其他" for _, group, _, _ in records)
    pending = [(path, group, ids, labels) for path, group, ids, labels in records if group.get("category") in (None, "", "其他")]
    while pending:
        next_pending = []
        for path, group, ids, labels in pending:
            category, reason = infer_category(ids, labels, known)
            if not category:
                next_pending.append((path, group, ids, labels))
                continue
            report["changes"].append({"collection": path.parent.name, "product": group.get("key", ""), "title": group["title"], "from": group.get("category") or "其他", "to": category, "reason": reason})
            group["category"] = category
            for identifier in ids:
                known.setdefault(identifier, category)
        if len(next_pending) == len(pending):
            pending = next_pending
            break
        pending = next_pending
    report["unresolved"] = [{"collection": path.parent.name, "product": group.get("key", ""), "title": group["title"], "tags": group.get("tags", [])} for path, group, _, _ in pending]
    report["before"] = dict(sorted(before.items()))
    report["after"] = dict(sorted(collections.Counter(group.get("category") or "其他" for _, group, _, _ in records).items()))
    allowed = set(json.loads((ROOT / "library/categories.json").read_text(encoding="utf-8-sig")))
    for path, meta in metas.items():
        original = json.loads(originals[path].decode("utf-8-sig"))
        stripped = copy.deepcopy(meta)
        for old_group, new_group in zip([original, *original.get("products", [])], [stripped, *stripped.get("products", [])]):
            if new_group.get("category") not in allowed:
                raise ValueError(f"Unknown category: {path}")
            if "category" in old_group:
                new_group["category"] = old_group["category"]
            else:
                new_group.pop("category", None)
        if stripped != original:
            raise ValueError(f"Refusing changes outside category fields: {path}")
        if args.apply and meta != original:
            backup = args.backup / path.relative_to(ROOT)
            backup.parent.mkdir(parents=True, exist_ok=True)
            with backup.open("xb") as file:
                file.write(originals[path])
            raw = (json.dumps(meta, ensure_ascii=False, indent=2) + "\n").encode("utf-8")
            temporary = path.with_name(".tmp-classify-" + hashlib.sha256(raw).hexdigest()[:12])
            temporary.write_bytes(raw)
            os.replace(temporary, path)
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"changed_groups": len(report["changes"]), "remaining_other": len(report["unresolved"]), "before": report["before"], "after": report["after"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
