package app

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

func (a *App) configureScanInterval(fallback time.Duration) error {
	var value string
	err := a.store.db.QueryRow(`SELECT value FROM runtime.settings WHERE key='scan_interval'`).Scan(&value)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		fallback, err = time.ParseDuration(value)
		if err != nil || fallback < 0 {
			return fmt.Errorf("保存的扫描间隔无效")
		}
	}
	a.scanInterval.Store(int64(fallback))
	return nil
}

func (a *App) saveScanSchedule(w http.ResponseWriter, r *http.Request) {
	minutes, err := strconv.Atoi(r.FormValue("minutes"))
	if err != nil || minutes < 0 || minutes > 43200 {
		http.Error(w, "扫描间隔应为 0–43200 分钟的整数，0 表示关闭", 400)
		return
	}
	interval := time.Duration(minutes) * time.Minute
	if err := a.store.transaction(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO runtime.settings(key,value) VALUES('scan_interval',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, interval.String()); err != nil {
			return err
		}
		return audit(tx, "scan_schedule_changed", interval.String())
	}); err != nil {
		renderError(w, err)
		return
	}
	a.scanInterval.Store(int64(interval))
	select {
	case a.scanChanged <- struct{}{}:
	default:
	}
	http.Redirect(w, r, "/admin/settings?view=library&saved=1", 303)
}

// One owner controls the timer; settings updates wake it even when disabled.
func (a *App) runLibraryScanner(done <-chan struct{}) {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		var tick <-chan time.Time
		if interval := time.Duration(a.scanInterval.Load()); interval > 0 {
			timer.Reset(interval)
			tick = timer.C
		}
		select {
		case <-done:
			return
		case <-a.scanChanged:
			continue
		case <-tick:
			if err := a.store.reloadLibrary(); err != nil && err != errLibraryScanRunning {
				log.Printf("素材库扫描失败，保留当前索引: %v", err)
			}
		}
	}
}
