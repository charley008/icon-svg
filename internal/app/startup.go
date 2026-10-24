package app

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

func env(key, fallback string) string {
	if s, ok := os.LookupEnv(key); ok && s != "" {
		return s
	}
	return fallback
}
func loadEnv(file string) error {
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("无效 .env 行")
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	return nil
}
func applicationBase(executable, workingDir string) string {
	if strings.Contains(executable, "go-build") {
		return workingDir
	}
	base := filepath.Dir(executable)
	if strings.EqualFold(filepath.Base(base), "bin") {
		return filepath.Dir(base)
	}
	return base
}

// Run starts the application using the command entry point version.
func Run(version string) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	base := applicationBase(executable, workingDir)
	if err := loadEnv(filepath.Join(base, ".env")); err != nil {
		return err
	}
	address := flag.String("listen", env("LISTEN_ADDR", "127.0.0.1:8000"), "监听地址")
	data := flag.String("data", env("DATA_DIR", filepath.Join(base, "data")), "数据目录")
	libraryPath := flag.String("library", env("LIBRARY_DIR", filepath.Join(base, "library")), "素材库目录")
	scanEvery, e := time.ParseDuration(env("SCAN_INTERVAL", "5m"))
	if e != nil {
		return fmt.Errorf("SCAN_INTERVAL 无效: %w", e)
	}
	scanInterval := flag.Duration("scan-interval", scanEvery, "素材库定期扫描间隔，0 关闭")
	webPath := flag.String("web", env("WEB_DIR", filepath.Join(base, "web")), "页面和静态资源目录")
	browser := flag.Bool("open", runtime.GOOS == "windows" && env("OPEN_BROWSER", "true") != "false", "启动后打开浏览器")
	health := flag.Bool("healthcheck", false, "检查已运行的服务")
	showVersion := flag.Bool("version", false, "显示版本号")
	flag.Parse()
	if *showVersion {
		fmt.Println(strings.TrimSpace(version))
		return nil
	}
	if *health {
		client := http.Client{Timeout: 4 * time.Second}
		r, e := client.Get("http://127.0.0.1:8000/healthz")
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("health status %d", r.StatusCode)
		}
		return nil
	}
	if err := os.MkdirAll(*data, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(*data, "cache"), 0700); err != nil {
		return err
	}
	if *scanInterval < 0 {
		return fmt.Errorf("扫描间隔不能为负数")
	}
	username := env("ADMIN_USERNAME", "admin")
	password := env("ADMIN_PASSWORD", "")
	// A fresh installation works on double-click. Persist generated credentials
	// before creating the database, so a failed initialization can be retried.
	credentialFile := filepath.Join(*data, "initial-admin.txt")
	dbFile := filepath.Join(*data, "runtime.sqlite3")
	if password == "" {
		if existing, e := os.ReadFile(credentialFile); e == nil {
			parts := strings.Split(strings.TrimSpace(string(existing)), "\n")
			if len(parts) == 2 {
				username = parts[0]
				password = parts[1]
			}
		}
	}
	if password == "" {
		password = randomToken()[:24]
		if _, e := os.Stat(dbFile); os.IsNotExist(e) {
			if e = os.WriteFile(credentialFile, []byte(username+"\n"+password+"\n"), 0600); e != nil {
				return e
			}
			log.Printf("首次管理员凭据已保存到 %s；登录后请修改密码", credentialFile)
		}
	}
	store, err := openLibraryStore(dbFile, *libraryPath, username, password)
	if err != nil {
		return err
	}
	defer store.close()
	app, err := newApp(store, *data, env("PUBLIC_ORIGIN", ""), env("COOKIE_SECURE", "false") == "true", AppResources{Web: os.DirFS(*webPath), Library: libraryInput{store.library.root.FS()}, Root: os.DirFS(base)})
	if err != nil {
		return err
	}
	app.scanInterval = *scanInterval
	scanDone := make(chan struct{})
	var scanWG sync.WaitGroup
	defer func() { close(scanDone); scanWG.Wait() }()
	if *scanInterval > 0 {
		scanWG.Add(1)
		go func() {
			defer scanWG.Done()
			ticker := time.NewTicker(*scanInterval)
			defer ticker.Stop()
			for {
				select {
				case <-scanDone:
					return
				case <-ticker.C:
					if e := store.reloadLibrary(); e != nil && e != errLibraryScanRunning {
						log.Printf("素材库扫描失败，保留当前索引: %v", e)
					}
				}
			}
		}()
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return fmt.Errorf("监听失败，请检查程序是否已启动或更改 LISTEN_ADDR: %w", err)
	}
	server := &http.Server{Handler: app.routes(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 40 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopped)
	go func() {
		<-stopped
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	target := "http://" + listener.Addr().String()
	if strings.HasPrefix(target, "http://[::]") {
		target = strings.Replace(target, "[::]", "127.0.0.1", 1)
	}
	if strings.Contains(target, "0.0.0.0") {
		target = strings.Replace(target, "0.0.0.0", "127.0.0.1", 1)
	}
	log.Printf("icon-svg 已启动：%s\n后台：%s/admin\n数据：%s", target, target, *data)
	if *browser {
		go func() {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "windows":
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
			case "darwin":
				cmd = exec.Command("open", target)
			default:
				cmd = exec.Command("xdg-open", target)
			}
			if e := cmd.Run(); e != nil {
				log.Printf("请手动打开 %s", target)
			}
		}()
	}
	if err = server.Serve(listener); err != http.ErrServerClosed {
		return err
	}
	return nil
}
