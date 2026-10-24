package app

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestAdminSettingsRequiresLoginAndSeparatesControls(t *testing.T) {
	a, h := testApp(t)
	assertCode(t, send(t, h, "GET", "/admin/settings", nil, nil), 303)
	cookie, token := authenticate(t, a, h)
	w := send(t, h, "GET", "/admin/settings", nil, cookie)
	assertCode(t, w, 200)
	for _, want := range []string{"数据备份", "账号安全", "操作日志", token} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("settings missing %q", want)
		}
	}
	if strings.Contains(w.Body.String(), `class="panel"`) {
		t.Fatal("logs must not be rendered by default")
	}
	logs := send(t, h, "GET", "/admin/settings?view=logs", nil, cookie)
	assertCode(t, logs, 200)
	if !strings.Contains(logs.Body.String(), "暂无操作记录") {
		t.Fatal("explicit logs view must show empty state")
	}
	for _, view := range []string{"data", "account", "logs"} {
		page := send(t, h, "GET", "/admin/settings?view="+view, nil, cookie)
		assertCode(t, page, 200)
		body := page.Body.String()
		if strings.Count(body, `class="panel"`) != 1 || !strings.Contains(body, `id="`+view+`"`) || !strings.Contains(body, `aria-current="page"`) {
			t.Fatalf("%s must display only its selected panel", view)
		}
		if view == "account" && (!strings.Contains(body, `minlength="8"`) || !strings.Contains(body, `action="/admin/password"`)) {
			t.Fatal("account panel must preserve password controls")
		}
		if view == "data" && (!strings.Contains(body, `href="/admin/backup"`) || !strings.Contains(body, `href="/admin/export-library"`)) {
			t.Fatal("data panel must preserve download controls")
		}
	}
	for _, path := range []string{"/admin", "/admin/brands/1"} {
		page := send(t, h, "GET", path, nil, cookie)
		assertCode(t, page, 200)
		if !strings.Contains(page.Body.String(), `href="/admin/settings"`) {
			t.Fatalf("%s missing settings navigation", path)
		}
		if strings.Contains(page.Body.String(), `action="/admin/password"`) || strings.Contains(page.Body.String(), `href="/admin/backup"`) {
			t.Fatalf("settings controls still appear on %s", path)
		}
	}
}

func TestAdminSettingsPaginatesLogsNewestFirst(t *testing.T) {
	a, h := testApp(t)
	cookie, _ := authenticate(t, a, h)
	for n := 1; n <= 31; n++ {
		if _, err := a.store.db.Exec(`INSERT INTO audit(action,target,created_at) VALUES(?,?,?)`, "icon_uploaded", fmt.Sprintf("log-%02d.svg", n), stamp()); err != nil {
			t.Fatal(err)
		}
	}
	first := send(t, h, "GET", "/admin/settings?view=logs", nil, cookie)
	assertCode(t, first, 200)
	body := first.Body.String()
	if !strings.Contains(body, "共 31 条") || strings.Count(body, "<time>") != 30 || strings.Contains(body, "log-01.svg") || strings.Index(body, "log-31.svg") > strings.Index(body, "log-30.svg") || !strings.Contains(body, `/admin/settings?page=2&amp;view=logs#logs`) {
		t.Fatal("incorrect first page log order or pagination")
	}
	for _, page := range []string{"2", "999"} {
		last := send(t, h, "GET", "/admin/settings?view=logs&page="+page, nil, cookie)
		assertCode(t, last, 200)
		if strings.Count(last.Body.String(), "<time>") != 1 || !strings.Contains(last.Body.String(), "log-01.svg") {
			t.Fatal("last page must show only the oldest entry")
		}
	}
}

func TestPasswordConfirmationRejectsMissingAndMismatch(t *testing.T) {
	a, h := testApp(t)
	cookie, token := authenticate(t, a, h)
	page := send(t, h, "GET", "/admin/settings?view=account", nil, cookie)
	assertCode(t, page, 200)
	if !strings.Contains(page.Body.String(), `name="password_confirm"`) {
		t.Fatal("account panel missing password confirmation")
	}
	var before string
	if err := a.store.db.QueryRow(`SELECT password FROM admins LIMIT 1`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, confirmation := range []string{"", "different-password"} {
		form := url.Values{"csrf": {token}, "current": {testPassword}, "password": {"newpass8"}, "password_confirm": {confirmation}}
		response := send(t, h, "POST", "/admin/password", form, cookie)
		assertCode(t, response, 400)
		if !strings.Contains(response.Body.String(), "两次输入的新密码不一致") {
			t.Fatal("missing confirmation feedback")
		}
		var after string
		if err := a.store.db.QueryRow(`SELECT password FROM admins LIMIT 1`).Scan(&after); err != nil || after != before {
			t.Fatalf("rejected confirmation changed password: %v", err)
		}
		assertCode(t, send(t, h, "GET", "/admin", nil, cookie), 200)
	}
	var changes int
	if err := a.store.db.QueryRow(`SELECT COUNT(*) FROM audit WHERE action='password_changed'`).Scan(&changes); err != nil || changes != 0 {
		t.Fatalf("rejected confirmation created password audit: %d %v", changes, err)
	}
}
