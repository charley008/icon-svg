package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"
)

type cachedLibraryFile struct {
	Info    fs.FileInfo
	Content []byte
}
type scanIOStats struct{ Read, Reused int }
type scanRun struct {
	Files   map[string][]byte
	IO      scanIOStats
	Updated bool
}

type LibraryScanStatus struct {
	Running, Full, Updated                              bool
	Started, Finished, LastSuccess                      time.Time
	Duration                                            string
	Error                                               string
	Files, Collections, Icons, Added, Modified, Removed int
	Read, Reused                                        int
}

// Metadata is always reread. SVG content can be reused after checking identity,
// size and modification time. Full scans bypass this cache, including edits
// made by tools that deliberately preserve the timestamp of an existing inode.
func (l *Library) readScannedFile(name string, useCache bool) ([]byte, fs.FileInfo, bool, error) {
	info, e := l.root.Stat(name)
	if e != nil {
		return nil, nil, false, e
	}
	if !info.Mode().IsRegular() {
		return nil, nil, false, errors.New("素材必须是普通文件")
	}
	if useCache && strings.HasSuffix(name, ".svg") {
		old, ok := l.cache[name]
		if ok && info.Size() == old.Info.Size() && info.ModTime().Equal(old.Info.ModTime()) && os.SameFile(info, old.Info) {
			return old.Content, info, true, nil
		}
	}
	content, e := l.read(name)
	if e != nil {
		return nil, nil, false, fmt.Errorf("%s: %w", name, e)
	}
	after, e := l.root.Stat(name)
	if e != nil {
		return nil, nil, false, e
	}
	if !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) || int64(len(content)) != after.Size() {
		return nil, nil, false, fmt.Errorf("%s 在扫描期间发生变化，请重试", name)
	}
	return content, after, false, nil
}

func (l *Library) scanStatus() LibraryScanStatus {
	l.statusMu.RLock()
	defer l.statusMu.RUnlock()
	return l.status
}

func (s *Store) scanLibrary(full bool) error {
	l := s.library
	// Coalesce simultaneous manual/timer requests. The active scan already
	// refreshes the library; callers can inspect its status on the settings page.
	if full {
		l.scanMu.Lock()
	} else if !l.scanMu.TryLock() {
		return errLibraryScanRunning
	}
	defer l.scanMu.Unlock()
	started := time.Now()
	l.statusMu.Lock()
	status := l.status
	status.Running = true
	status.Full = full
	status.Started = started
	status.Error = ""
	status.Added = 0
	status.Modified = 0
	status.Removed = 0
	status.Updated = false
	l.status = status
	l.statusMu.Unlock()
	run := scanRun{}
	err := s.reloadLibraryIndex(full, &run)
	status.Running = false
	status.Finished = time.Now()
	status.Duration = status.Finished.Sub(started).Round(time.Millisecond).String()
	status.Read, status.Reused = run.IO.Read, run.IO.Reused
	status.Updated = run.Updated
	if err != nil {
		status.Error = err.Error()
	} else {
		status.LastSuccess = status.Finished
	}
	if run.Files != nil && err == nil {
		status.Files = len(run.Files)
		status.Collections = 0
		status.Icons = 0
		next := map[string]string{}
		l.mu.Lock()
		for name, content := range run.Files {
			if strings.HasSuffix(name, "/meta.json") {
				status.Collections++
			}
			if strings.HasSuffix(name, ".svg") {
				status.Icons++
			}
			hash := digest(content)
			next[name] = hash
			if old, ok := l.scanned[name]; !ok {
				status.Added++
			} else if old != hash {
				status.Modified++
			}
		}
		for name := range l.scanned {
			if _, ok := next[name]; !ok {
				status.Removed++
			}
		}
		if err == nil {
			l.scanned = next
		}
		l.mu.Unlock()
	}
	l.statusMu.Lock()
	l.status = status
	l.statusMu.Unlock()
	return err
}

var errLibraryScanRunning = errors.New("素材库正在扫描，请稍后查看结果")

func scanSchedule(interval time.Duration) string {
	if interval == 0 {
		return "定期扫描已关闭，可手动扫描"
	}
	if interval%time.Hour == 0 {
		return fmt.Sprintf("每 %d 小时自动扫描", interval/time.Hour)
	}
	if interval%time.Minute == 0 {
		return fmt.Sprintf("每 %d 分钟自动扫描", interval/time.Minute)
	}
	return fmt.Sprintf("每 %s 自动扫描", interval)
}
