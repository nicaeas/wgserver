package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wgserver/internal/config"
)

var (
	once sync.Once
	cfg  *config.Config

	connLogger *log.Logger
	roleLogger *log.Logger
	mapLogger  *log.Logger
	eqLogger   *log.Logger
	taskLogger *log.Logger

	currentDay string
	mu         sync.Mutex
)

func initLoggers() {
	cfg = config.Load()
	rotateIfNeeded()
}

func rotateIfNeeded() {
	mu.Lock()
	defer mu.Unlock()
	// 使用 UTC+8 日期进行日志切割
	loc := time.FixedZone("UTC+8", 8*60*60)
	day := time.Now().In(loc).Format("20060102")
	if day == currentDay && connLogger != nil {
		return
	}
	currentDay = day
	mk := func(name string) *log.Logger {
		_ = os.MkdirAll(cfg.LogDir, 0755)
		fn := filepath.Join(cfg.LogDir, fmt.Sprintf("log_%s_%s.log", day, name))
		f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("open log file %s error: %v", fn, err)
			return log.Default()
		}
		return log.New(f, "", log.LstdFlags|log.Lmicroseconds)
	}
	connLogger = mk("connection")
	roleLogger = mk("role_info")
	mapLogger = mk("map_allocation")
	eqLogger = mk("equipment_allocation")
	taskLogger = mk("task_queue")
}

func Connection() *log.Logger { once.Do(initLoggers); rotateIfNeeded(); return connLogger }
func RoleInfo() *log.Logger   { once.Do(initLoggers); rotateIfNeeded(); return roleLogger }
func MapAlloc() *log.Logger   { once.Do(initLoggers); rotateIfNeeded(); return mapLogger }
func Equipment() *log.Logger  { once.Do(initLoggers); rotateIfNeeded(); return eqLogger }
func TaskQueue() *log.Logger  { once.Do(initLoggers); rotateIfNeeded(); return taskLogger }
