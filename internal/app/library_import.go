package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"
)

const maxLibraryZIP int64 = 64 << 20
const maxLibraryExpanded int64 = 256 << 20

func libraryLicenseTarget(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] != "licenses" || !fs.ValidPath(name) {
		return false
	}
	base := parts[1]
	return len(base) <= 180 && !strings.HasPrefix(base, ".") && !strings.ContainsAny(base, "\\:") && !reservedName(base) && (strings.HasSuffix(strings.ToLower(base), ".txt") || strings.HasSuffix(strings.ToLower(base), ".md"))
}

// Read ZIP entries without extracting paths to disk. Bound both compressed and
// actual expanded bytes, and reject duplicate names, links and special files.
func readLibraryZIP(reader io.ReaderAt, size int64) (map[string][]byte, error) {
	if size <= 0 || size > maxLibraryZIP {
		return nil, errors.New("素材 ZIP 不能为空且不能超过 64 MB")
	}
	z, err := zip.NewReader(reader, size)
	if err != nil {
		return nil, errors.New("无法读取素材 ZIP")
	}
	if len(z.File) > 50000 {
		return nil, errors.New("ZIP 文件条目过多")
	}
	files, seen := map[string][]byte{}, map[string]bool{}
	var total int64
	for _, entry := range z.File {
		name := strings.TrimPrefix(entry.Name, "library/")
		checked := strings.TrimSuffix(name, "/")
		if checked == "" && entry.FileInfo().IsDir() && entry.Name == "library/" {
			continue
		}
		if !fs.ValidPath(checked) || strings.ContainsAny(checked, "\\:") || entry.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("ZIP 路径或类型无效: %s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if !entry.Mode().IsRegular() || seen[strings.ToLower(name)] {
			return nil, fmt.Errorf("ZIP 包含重复路径或特殊文件: %s", entry.Name)
		}
		seen[strings.ToLower(name)] = true
		if !libraryTarget(name) && name != "README.txt" {
			return nil, fmt.Errorf("ZIP 包含不支持的文件: %s；请使用素材库导出的 ZIP", entry.Name)
		}
		limit := int64(16 << 20)
		if strings.HasSuffix(name, ".svg") {
			limit = maxSVG
		}
		limit = min(limit, maxLibraryExpanded-total)
		if entry.UncompressedSize64 > uint64(limit) {
			return nil, errors.New("ZIP 解压大小超出限制")
		}
		f, err := entry.Open()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(f, limit+1))
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if int64(len(raw)) > limit {
			return nil, errors.New("ZIP 解压大小超出限制")
		}
		total += int64(len(raw))
		if strings.HasSuffix(name, ".svg") {
			if _, err = validateSVG(raw); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
		}
		if name != "README.txt" {
			files[name] = raw
		}
	}
	return files, nil
}

type libraryImportResult struct{ Added, Skipped, Categorized int }

// Merge by collection and product key. Keep existing descriptions, IDs,
// visibility and default choices; assign fresh IDs and filenames to additions.
func mergeLibraryFiles(current, incoming map[string][]byte, fillCategories bool) (map[string][]byte, libraryImportResult, error) {
	result := libraryImportResult{}
	merged := map[string][]byte{}
	for name, raw := range current {
		merged[name] = raw
	}
	names := []string{}
	for name := range incoming {
		if strings.HasSuffix(name, "/meta.json") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, result, errors.New("ZIP 中没有合集 meta.json；请导入带有元数据的素材库 ZIP")
	}
	referenced := map[string]bool{}
	for _, name := range names {
		slug := path.Dir(name)
		var src, dst BrandMeta
		_, existingBrand := current[name]
		if err := json.Unmarshal(incoming[name], &src); err != nil || src.Schema != 1 || strings.TrimSpace(src.Title) == "" {
			return nil, result, fmt.Errorf("合集元数据无效: %s", name)
		}
		if raw, exists := current[name]; exists {
			if err := json.Unmarshal(raw, &dst); err != nil {
				return nil, result, err
			}
		} else {
			dst = src
			dst.ID, dst.GroupMeta.ID = 0, 0
			// Detail IDs are installation-local; do not import another server's
			// retired URLs into the destination ID namespace.
			dst.RedirectFrom = nil
			dst.Icons, dst.Products = nil, nil
		}
		groups := append([]GroupMeta{src.GroupMeta}, src.Products...)
		groupKeys := map[string]bool{}
		for index, srcGroup := range groups {
			if index == 0 {
				srcGroup.Key = ""
			} else if !validSlug(srcGroup.Key) {
				return nil, result, errors.New("产品标识无效")
			}
			if groupKeys[srcGroup.Key] {
				return nil, result, errors.New("ZIP 包含重复产品")
			}
			groupKeys[srcGroup.Key] = true
			var target *GroupMeta
			existingGroup := index == 0 && existingBrand
			if index == 0 {
				target = &dst.GroupMeta
			} else {
				for n := range dst.Products {
					if dst.Products[n].Key == srcGroup.Key {
						existingGroup = true
						target = &dst.Products[n]
						break
					}
				}
				if target == nil {
					g := srcGroup
					g.ID, g.Icons = 0, nil
					g.RedirectFrom = nil
					dst.Products = append(dst.Products, g)
					target = &dst.Products[len(dst.Products)-1]
				}
			}
			if fillCategories && existingGroup && (target.Category == "" || target.Category == "其他") && srcGroup.Category != "" && srcGroup.Category != "其他" {
				target.Category = srcGroup.Category
				result.Categorized++
			}
			hashes, sources := map[string]bool{}, map[string]bool{}
			hasDefault := false
			for _, icon := range target.Icons {
				hashes[digest(merged[slug+"/"+icon.File])] = true
				if strings.HasPrefix(icon.Source, "https://") {
					sources[icon.Source] = true
				}
				hasDefault = hasDefault || (icon.Default && !icon.Deleted)
				target.Next = max(target.Next, icon.Version+1)
			}
			hadIcons := len(target.Icons) > 0
			for _, icon := range srcGroup.Icons {
				oldName := slug + "/" + icon.File
				raw, exists := incoming[oldName]
				if !exists || icon.Version < 0 || icon.Version >= 1000000 || icon.File != filename(slug, srcGroup.Key, icon.Version) || referenced[oldName] {
					return nil, result, fmt.Errorf("SVG 引用缺失、重复或命名无效: %s", oldName)
				}
				referenced[oldName] = true
				hash := digest(raw)
				if hashes[hash] || (icon.Source != "" && sources[icon.Source]) || (existingBrand && dst.Deleted) || (existingGroup && target.Deleted) {
					result.Skipped++
					continue
				}
				n := target.Next
				for {
					if n >= 1000000 {
						return nil, result, errors.New("图标版本数量超出限制")
					}
					if _, occupied := merged[slug+"/"+filename(slug, target.Key, n)]; !occupied {
						break
					}
					n++
				}
				icon.ID, icon.Version, icon.File = 0, n, filename(slug, target.Key, n)
				icon.SHA, icon.Bytes = hash, len(raw)
				icon.Default = !hasDefault && !icon.Deleted && (hadIcons || icon.Default)
				hasDefault = hasDefault || icon.Default
				target.Next = n + 1
				target.Icons = append(target.Icons, icon)
				merged[slug+"/"+icon.File] = raw
				hashes[hash] = true
				if strings.HasPrefix(icon.Source, "https://") {
					sources[icon.Source] = true
				}
				result.Added++
			}
		}
		raw, err := json.MarshalIndent(dst, "", "  ")
		if err != nil {
			return nil, result, err
		}
		merged[name] = append(raw, '\n')
	}
	for name := range incoming {
		if strings.HasSuffix(name, ".svg") && !referenced[name] {
			return nil, result, fmt.Errorf("SVG 未在 meta.json 登记: %s", name)
		}
	}
	// Add categories without removing or renaming the destination categories.
	categorySet := map[string]bool{"其他": true}
	for _, files := range []map[string][]byte{current, incoming} {
		if raw, ok := files["categories.json"]; ok {
			var categories []string
			if err := json.Unmarshal(raw, &categories); err != nil {
				return nil, result, errors.New("分类列表无效")
			}
			for _, name := range categories {
				if strings.TrimSpace(name) == "" || len(name) > 240 {
					return nil, result, errors.New("分类名称无效")
				}
				categorySet[name] = true
			}
		}
	}
	categories := []string{}
	for name := range categorySet {
		categories = append(categories, name)
	}
	sort.Strings(categories)
	merged["categories.json"], _ = json.MarshalIndent(categories, "", "  ")
	return merged, result, nil
}

func (s *Store) importLibraryFiles(incoming map[string][]byte, fillCategories bool) (libraryImportResult, error) {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	l := s.library
	l.mu.Lock()
	defer l.mu.Unlock()
	result := libraryImportResult{}
	current, err := l.load()
	if err != nil {
		return result, err
	}
	if fingerprint(current) != l.fingerprint {
		return result, errors.New("素材文件已被外部修改，请先扫描素材库")
	}
	merged, result, err := mergeLibraryFiles(current, incoming, fillCategories)
	if err != nil {
		return result, err
	}
	if len(merged) > 50000 {
		return result, errors.New("合并后的素材文件过多")
	}
	total := 0
	for _, raw := range merged {
		total += len(raw)
	}
	if total > int(maxLibraryExpanded) {
		return result, errors.New("合并后的素材库超过 256 MB")
	}
	merged, err = normalizeLibraryFiles(merged)
	if err != nil {
		return result, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM icons; DELETE FROM groups; UPDATE brands SET parent=NULL; DELETE FROM brands; DELETE FROM categories;`); err != nil {
		return result, err
	}
	if err = loadCatalog(tx, libraryInput{snapshotFS(merged)}); err != nil {
		return result, err
	}
	var categories []string
	json.Unmarshal(merged["categories.json"], &categories)
	for _, name := range categories {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO categories(name) VALUES(?)`, name); err != nil {
			return result, err
		}
	}
	if _, err = tx.Exec(`INSERT OR IGNORE INTO categories SELECT DISTINCT json_extract(metadata,'$.category') FROM groups WHERE COALESCE(json_extract(metadata,'$.category'),'')<>''; UPDATE icons SET downloads=COALESCE((SELECT count FROM runtime.downloads WHERE key=(SELECT slug FROM brands WHERE id=icons.brand_id)||'/'||icons.filename),0)`); err != nil {
		return result, err
	}
	if err = audit(tx, "library_imported", fmt.Sprintf("新增 %d，跳过 %d，分类补全 %d", result.Added, result.Skipped, result.Categorized)); err != nil {
		return result, err
	}
	canonical, err := librarySnapshot(tx)
	if err != nil {
		return result, err
	}
	for name, raw := range incoming {
		if !libraryLicenseTarget(name) {
			continue
		}
		original := name
		for n := 0; ; n++ {
			if n > 0 {
				ext := path.Ext(original)
				name = strings.TrimSuffix(original, ext) + "-" + digest(raw)[:12]
				if n > 1 {
					name += fmt.Sprintf("-%d", n)
				}
				name += ext
			}
			if !libraryLicenseTarget(name) {
				return result, errors.New("许可文件名过长")
			}
			old, readErr := l.read(name)
			if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
				return result, readErr
			}
			pending, occupied := canonical[name]
			if (readErr != nil || bytes.Equal(old, raw)) && (!occupied || bytes.Equal(pending, raw)) {
				break
			}
		}
		canonical[name] = raw
	}
	previous := l.fingerprint
	if err = l.saveLocked(canonical); err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		// Force the next scan to recover the disk/index boundary if committing
		// the runtime audit fails after the durable file journal was applied.
		l.fingerprint = previous
	}
	return result, err
}

func (a *App) importLibrary(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "请选择素材库 ZIP", 400)
		return
	}
	defer file.Close()
	files, err := readLibraryZIP(file, header.Size)
	if err != nil {
		renderError(w, err)
		return
	}
	result, err := a.store.importLibraryFiles(files, r.FormValue("fill_categories") == "on")
	if err != nil {
		renderError(w, err)
		return
	}
	// Refresh scan counts and the recent-additions panel after publishing.
	_ = a.store.scanLibrary(false)
	http.Redirect(w, r, fmt.Sprintf("/admin/settings?view=library&imported=1&added=%d&skipped=%d&categorized=%d", result.Added, result.Skipped, result.Categorized), 303)
}

func (a *App) exportFullLibrary(w http.ResponseWriter, r *http.Request) {
	a.archive(w, r, "icon-svg-materials.zip", func(z *zip.Writer) error {
		s := a.store
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		s.library.mu.Lock()
		defer s.library.mu.Unlock()
		files, err := s.library.load()
		if err != nil {
			return err
		}
		if err = addLibraryFiles(z, files); err != nil {
			return err
		}
		return a.libraryLicenses(z)
	})
}
