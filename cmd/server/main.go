// Command server starts the NginxPanel-Lite backend.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/xzdmycbx/nginxpanel-lite/internal/app"
	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/config"
	"github.com/xzdmycbx/nginxpanel-lite/internal/database"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "探测本地服务健康后退出（容器健康检查用）")
	resetTOTP := flag.String("reset-totp", "", "清除指定用户名的两步验证后退出（应急解锁，丢失验证器时使用）")
	flag.Parse()
	if *healthcheck {
		os.Exit(probeHealth())
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	if *resetTOTP != "" {
		os.Exit(resetUserTOTP(cfg, *resetTOTP))
	}

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}

	application.Scheduler.Start()

	go func() {
		log.Printf("NginxPanel-Lite 监听 %s", cfg.ListenAddr)
		if err := application.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭...")

	application.Scheduler.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := application.Server.Shutdown(ctx); err != nil {
		log.Printf("关闭出错: %v", err)
	}
}

// resetUserTOTP clears a user's TOTP directly in the DB (break-glass for a lost
// authenticator on the only admin). Run inside the container/host with DB access.
func resetUserTOTP(cfg *config.Config, username string) int {
	db, err := database.Open(cfg.DBPath)
	if err != nil {
		log.Printf("打开数据库失败: %v", err)
		return 1
	}
	var user models.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		log.Printf("未找到用户 %q: %v", username, err)
		return 1
	}
	user.TOTPEnabled = false
	user.TOTPSecret = ""
	user.TokenEpoch++
	if err := db.Save(&user).Error; err != nil {
		log.Printf("保存失败: %v", err)
		return 1
	}
	_ = db.Create(&models.AuditLog{
		ActorUsername: "system:cli",
		Action:        audit.ActUserResetTOTP,
		TargetType:    audit.TargetUser,
		TargetID:      username,
		Detail:        "应急重置两步验证 " + username,
		Result:        audit.ResultOK,
		IP:            "local",
		CreatedAt:     time.Now(),
	}).Error
	log.Printf("已重置用户 %q 的两步验证，下次登录将重新绑定", username)
	return 0
}

// probeHealth performs a local health request and returns a process exit code.
func probeHealth() int {
	addr := os.Getenv("PANEL_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + addr + "/api/system/ready")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return 0
	}
	return 1
}

