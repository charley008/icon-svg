"""Append missing Dashboard Icons SVGs from a local upstream checkout.

Usage: python .github/scripts/sync-dashboard-icons.py UPSTREAM --report PATH
Existing defaults, metadata and files are never overwritten. Re-running is safe.
"""

import argparse
import hashlib
import json
import re
import xml.etree.ElementTree as ET
from pathlib import Path
from urllib.parse import unquote, urlparse

from icon_categories import infer_category, upstream_category
from importlib.util import spec_from_file_location, module_from_spec

# Share the reviewed artwork suffix rules with the one-time catalog migration.
_spec = spec_from_file_location("brand_organization", Path(__file__).with_name("organize-library.py"))
_organization = module_from_spec(_spec)
_spec.loader.exec_module(_organization)

ROOT = Path(__file__).resolve().parents[2]
REPOSITORY = "https://github.com/homarr-labs/dashboard-icons"
SOURCE = "https://raw.githubusercontent.com/homarr-labs/dashboard-icons/main/svg/"
SVG_NS = "http://www.w3.org/2000/svg"
ET.register_namespace("", SVG_NS)
ET.register_namespace("xlink", "http://www.w3.org/1999/xlink")


def vector_svg(raw):
    """Remove editor metadata without changing vector geometry or styles."""
    root = ET.fromstring(raw)
    if root.tag not in ("svg", f"{{{SVG_NS}}}svg"):
        raise ValueError("not an SVG root")
    for element in root.iter():
        local = element.tag.rsplit("}", 1)[-1].lower()
        if local in {"image", "foreignobject", "animate", "animatetransform", "animatemotion", "set"}:
            raise ValueError(f"contains raster, external content or animation: {local}")
    for parent in list(root.iter()):
        for child in list(parent):
            local = child.tag.rsplit("}", 1)[-1].lower()
            if local in {"metadata", "script", "color-profile"} or (child.tag.startswith("{") and not child.tag.startswith(f"{{{SVG_NS}}}")):
                parent.remove(child)
    for element in root.iter():
        if "}" not in element.tag:
            element.tag = f"{{{SVG_NS}}}{element.tag}"
        local = element.tag.rsplit("}", 1)[-1].lower()
        if local == "a":
            element.tag = f"{{{SVG_NS}}}g"
        for name, value in list(element.attrib.items()):
            attribute = name.rsplit("}", 1)[-1].lower()
            foreign = name.startswith("{") and not name.startswith(("{http://www.w3.org/1999/xlink}", "{http://www.w3.org/XML/1998/namespace}"))
            if foreign or attribute.startswith("on") or (local == "a" and attribute in {"href", "target"}):
                del element.attrib[name]
            elif "/*" in value:
                element.attrib[name] = re.sub(r"/\*.*?\*/", "", value, flags=re.S)
        if local == "style" and element.text:
            element.text = re.sub(r"/\*.*?\*/", "", element.text, flags=re.S)
            if not any(child.tag.rsplit("}", 1)[-1] in {"text", "tspan"} for child in root.iter()):
                # Browser-exported SVGs can contain unused remote web fonts.
                element.text = re.sub(r"@font-face\s*\{[^}]*\}", "", element.text, flags=re.S | re.I)
    return ET.tostring(root, encoding="utf-8", xml_declaration=True)


def base_name(stem, stems):
    return _organization.artwork_base(stem, stems)


def category_for(categories):
    return upstream_category(categories, generic=True) or "其他"


def token(value):
    return re.sub(r"[^a-z0-9]", "", value.lower())


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def source_file(source):
    if "homarr-labs/dashboard-icons/" in source and "/svg/" in source:
        return unquote(urlparse(source).path.rsplit("/", 1)[-1])
    return ""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("upstream", type=Path)
    parser.add_argument("--report", required=True, type=Path)
    args = parser.parse_args()
    flat_path = args.upstream / "metadata.json"
    flat_metadata = json.loads(flat_path.read_text(encoding="utf-8-sig")) if flat_path.is_file() else {}
    rules_path = Path(__file__).with_name("library-rules.json")
    reviewed_rules = json.loads(rules_path.read_text(encoding="utf-8")) if rules_path.is_file() else {"merges": [], "products": []}
    canonical_groups = {source.replace("/", "-"): target for source, target in [*reviewed_rules["merges"], *reviewed_rules["products"]]}
    excluded_stems = set(reviewed_rules.get("excluded_stems", []))
    excluded_prefixes = tuple(reviewed_rules.get("excluded_prefixes", []))
    library = ROOT / "library"
    metadata, groups, aliases, origins, hashes = {}, {}, {}, {}, set()
    original = {}

    def register(slug, meta):
        for group in [meta, *meta.get("products", [])]:
            key = group.get("key", "") if group is not meta else ""
            identity = (slug, key)
            groups[identity] = group
            for label in [slug if not key else f"{slug}-{key}", group.get("title", ""), *group.get("aliases", [])]:
                aliases.setdefault(token(label), set()).add(identity)
            for icon in group.get("icons", []):
                origin = source_file(icon.get("source", ""))
                if origin:
                    origins[origin] = identity
                file = library / slug / icon["file"]
                if file.is_file():
                    hashes.add(sha(file.read_bytes()))
                upstream_hash = icon.get("provenance", {}).get("upstream_sha256", "")
                if re.fullmatch(r"[0-9a-f]{64}", upstream_hash):
                    hashes.add(upstream_hash)

    for file in sorted(library.glob("*/meta.json")):
        if file.parent.name == "licenses":
            continue
        meta = json.loads(file.read_text(encoding="utf-8-sig"))
        metadata[file.parent.name] = meta
        original[file.parent.name] = json.dumps(meta, ensure_ascii=False)
        register(file.parent.name, meta)

    svg_files = sorted((args.upstream / "svg").glob("*.svg"))
    stems = {file.stem for file in svg_files}
    svg_files.sort(key=lambda file: (base_name(file.stem, stems), file.stem != base_name(file.stem, stems), file.name))
    report = {"upstream": REPOSITORY, "examined": len(svg_files), "added": [], "skipped_source": [], "skipped_content": [], "invalid_names": [], "excluded": []}
    operating_systems = {"android", "arch-linux", "debian-linux", "ubuntu-linux", "kubuntu-linux", "lubuntu-linux", "xubuntu-linux", "fedora", "centos", "freebsd", "openbsd", "netbsd", "gentoo", "nixos", "nix-os", "linux", "linux-mint", "alpine-linux", "almalinux", "alma-linux", "rocky-linux", "opensuse", "opensuse-leap", "opensuse-tumbleweed", "manjaro", "pop-os", "zorin-os", "macos", "microsoft-windows", "windows-10", "windows-11", "windows-95", "windows-retro", "raspbian"}
    frameworks = {"reactjs", "vue", "vuejs", "angular", "svelte", "nextjs", "nuxt", "nuxtjs", "astro", "django", "flask", "fastapi", "laravel", "spring", "spring-boot", "express", "nodejs", "node-js", "typescript", "javascript", "vite", "webpack", "tailwind-css", "bootstrap", "dotnet", "electron"}
    preferred = {"reactjs": ("react", "React"), "debian-linux": ("debian", "Debian"), "ubuntu-linux": ("ubuntu", "Ubuntu"), "arch-linux": ("arch-linux", "Arch Linux")}

    for file in svg_files:
        # Brand families explicitly excluded by the library owner.
        stem = file.stem.lower()
        if stem in excluded_stems or (excluded_prefixes and stem.startswith(excluded_prefixes)) or stem.startswith(("pvy", "avm-")) or stem == "avm" or "fritz" in stem or token(stem).startswith("repeater1200ax"):
            report.setdefault("skipped_excluded_brand", []).append(file.name)
            continue
        if file.name in origins:
            report["skipped_source"].append(file.name)
            continue
        raw = file.read_bytes()
        digest = sha(raw)
        if digest in hashes:
            report["skipped_content"].append(file.name)
            continue
        try:
            cleaned = vector_svg(raw)
        except (ET.ParseError, ValueError) as error:
            report["excluded"].append({"source": file.name, "reason": str(error)})
            continue
        if sha(cleaned) in hashes:
            report["skipped_content"].append(file.name)
            continue
        base = base_name(file.stem, stems)
        upstream_meta_path = args.upstream / "meta" / f"{base}.json"
        upstream_meta = flat_metadata.get(base)
        if upstream_meta is None:
            upstream_meta = json.loads(upstream_meta_path.read_text(encoding="utf-8-sig")) if upstream_meta_path.is_file() else {}
        candidates = set()
        candidates.update(aliases.get(token(base), set()))
        # A fully qualified product name (microsoft-copilot, google-maps, ...)
        # belongs inside that existing collection, even if a broader alias also
        # matches. Never create a standalone collection in this exact case.
        product_matches = {(slug, key) for slug, key in groups if key and token(slug + "-" + key) == token(base)}
        if len(product_matches) == 1:
            candidates = product_matches
        # Upstream uses generic aliases such as 'Linux Distribution' across
        # unrelated brands. Only an exact name identifies a merge destination.
        # Variants already present in the catalogue identify their own group.
        if f"{base}.svg" in origins:
            candidates = {origins[f"{base}.svg"]}
        # Persist reviewed moves, including release channels merged as versions.
        # Resolve chains such as an old product name that later moved brands.
        canonical = canonical_groups.get(base)
        visited = set()
        while canonical and canonical.replace("/", "-") in canonical_groups and canonical not in visited:
            visited.add(canonical)
            canonical = canonical_groups[canonical.replace("/", "-")]
        if canonical:
            owner, _, product = canonical.partition("/")
            if (owner, product) in groups:
                candidates = {(owner, product)}
        # These official YouTube services already belong to Google's products.
        # youtube-dl and other third-party tools intentionally remain distinct.
        if base in {"youtube", "youtube-music", "youtube-kids", "youtube-studio", "youtube-tv"} and ("google", base) in groups:
            candidates = {("google", base)}
        suggested, title = preferred.get(base, (base, base.replace("-", " ").title()))
        if not candidates and (suggested, "") in groups:
            candidates = {(suggested, "")}
        if not candidates and base.endswith("-linux"):
            candidates = aliases.get(token(base.removesuffix("-linux")), set())
        if len(candidates) == 1:
            identity = next(iter(candidates))
            slug, key = identity
            group = groups[identity]
        else:
            slug, key = suggested.rstrip("-"), ""
            if not re.fullmatch(r"[a-z0-9]+(?:[.-][a-z0-9]+)*", slug) or len(slug) > 80 or slug in {"con", "prn", "aux", "nul", "licenses"}:
                report["invalid_names"].append(file.name)
                continue
            if slug in metadata:
                # Ambiguous alias matches never overwrite an existing collection.
                group = metadata[slug]
            else:
                upstream_categories = upstream_meta.get("categories", [])
                inferred, _ = infer_category([base, suggested], upstream_categories, {})
                category = inferred or category_for(upstream_categories)
                if base in operating_systems or base.endswith("-linux") or "Operating-Systems" in upstream_categories or "Linux-Distributions" in upstream_categories:
                    category = "系统工具"
                elif base in frameworks or "Development" in upstream_categories:
                    category = "开发编程"
                elif "Network" in upstream_categories:
                    category = "网络工具"
                elif "Media" in upstream_categories:
                    category = "影音娱乐"
                group = {"schema_version": 1, "title": title, "url": "", "domains": [], "description": "", "category": category, "tags": ["Dashboard Icons", *upstream_categories], "aliases": list(dict.fromkeys([base, *upstream_meta.get("aliases", [])])), "notes": [], "icons": [], "next_version": 0, "products": []}
                if base in operating_systems or base.endswith("-linux"):
                    group["tags"].extend(["操作系统", "Linux" if "linux" in base else title])
                if base == "reactjs":
                    group.update(url="https://react.dev/", domains=["react.dev"], description="用于构建用户界面的 React 库。")
                    group["aliases"].extend(["React", "react.js", "react-js", "React 框架"])
                metadata[slug] = group
                register(slug, group)
            identity = (slug, key)

        directory = library / slug
        directory.mkdir(exist_ok=True)
        icons = group.setdefault("icons", [])
        version = max([group.get("next_version", 0), *(icon["version"] + 1 for icon in icons)])
        stem = slug + (f"-{key}" if key else "")
        while True:
            name = f"{stem}{f'-{version}' if version else ''}.svg"
            target = directory / name
            if not target.exists():
                break
            version += 1
        with target.open("xb") as output:
            output.write(cleaned)
        icons.append({"file": name, "version": version, "label": file.stem, "is_default": not any(not icon.get("deleted") for icon in icons), "sha256": sha(cleaned), "bytes": len(cleaned), "source": SOURCE + file.name, "license": "Apache-2.0；商标归各品牌权利人", "method": "downloaded-svg", "collection": "Dashboard Icons", "provenance": {"repository": REPOSITORY, "original_file": file.name, "upstream_sha256": digest, "processing": "removed editor metadata, comments and non-rendering scripts; preserved vector geometry"}})
        group["next_version"] = version + 1
        origins[file.name] = identity
        hashes.add(digest)
        hashes.add(sha(cleaned))
        report["added"].append({"source": file.name, "target": f"{slug}/{name}"})

    for slug, meta in metadata.items():
        if json.dumps(meta, ensure_ascii=False) != original.get(slug):
            (library / slug / "meta.json").write_text(json.dumps(meta, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({key: len(value) if isinstance(value, list) else value for key, value in report.items()}, ensure_ascii=False))


if __name__ == "__main__":
    main()
