package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/majianyu2007/nwafu-connect/configs"
	"github.com/majianyu2007/nwafu-connect/internal/appdata"
	"github.com/majianyu2007/nwafu-connect/internal/desktopconfig"
	"github.com/majianyu2007/nwafu-connect/internal/managedbrowser"
)

type Status struct {
	Connected bool
	Message   string
}

type Controller struct {
	paths       appdata.Paths
	store       desktopconfig.Store
	corePath    string
	ctx         context.Context
	cancel      context.CancelFunc
	mutex       sync.Mutex
	command     *exec.Cmd
	commandDone chan struct{}
	generation  uint64
	status      chan Status
	logFile     *os.File
}

func NewController(paths appdata.Paths, store desktopconfig.Store, configuredCore string) (*Controller, error) {
	corePath, err := findCoreExecutable(configuredCore)
	if err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(paths.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open desktop log: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	controller := &Controller{paths: paths, store: store, corePath: corePath, ctx: ctx, cancel: cancel, status: make(chan Status, 16), logFile: logFile}
	go controller.monitorWakeAndNetwork()
	return controller, nil
}

func (c *Controller) Status() <-chan Status { return c.status }

func (c *Controller) Configured() (bool, error) {
	configuration, _, err := c.store.Load()
	if err != nil {
		return false, err
	}
	switch stringValue(configuration.AuthType, "auth/psw") {
	case "auth/psw":
		return stringValue(configuration.Username, "") != "" && stringValue(configuration.Password, "") != "", nil
	case "auth/smsCheckCode":
		return stringValue(configuration.Phone, "") != "", nil
	case "auth/qywechat":
		return true, nil
	default:
		return false, nil
	}
}

func (c *Controller) Start() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.command != nil {
		return nil
	}
	return c.startLocked()
}

func (c *Controller) Restart() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.stopLocked()
	return c.startLocked()
}

func (c *Controller) Stop() error {
	c.cancel()
	c.mutex.Lock()
	c.stopLocked()
	c.mutex.Unlock()
	if err := c.logFile.Close(); err != nil {
		return fmt.Errorf("close desktop log: %w", err)
	}
	return nil
}

func (c *Controller) OpenBrowser() error {
	state, err := managedbrowser.ReadState(c.paths.BrowserState)
	if err != nil {
		return fmt.Errorf("连接尚未就绪，请稍后重试: %w", err)
	}
	process, err := managedbrowser.Start(c.ctx, managedbrowser.Options{
		Executable: state.Executable, ProxyAddress: state.ProxyAddress, StartURL: state.StartURL, ProfileDir: state.ProfileDir,
	})
	if err != nil {
		return err
	}
	go func() { _ = process.Wait() }()
	return nil
}

// Activate implements the default tray action: connect when stopped, otherwise
// open another managed browser window once the private proxy is ready.
func (c *Controller) Activate() error {
	c.mutex.Lock()
	running := c.command != nil
	c.mutex.Unlock()
	if !running {
		return c.Start()
	}
	if _, err := managedbrowser.ReadState(c.paths.BrowserState); err != nil {
		return nil
	}
	return c.OpenBrowser()
}

func (c *Controller) startLocked() error {
	if c.ctx.Err() != nil {
		return errors.New("桌面控制器已停止")
	}
	configured, err := c.configuredLocked()
	if err != nil {
		return err
	}
	if !configured {
		return errors.New("请先完成学校网关与认证配置")
	}
	_ = managedbrowser.RemoveState(c.paths.BrowserState)
	arguments := []string{
		"--config", c.paths.Config,
		"--browser-mode",
		"--browser-profile-dir", c.paths.BrowserProfile,
		"--browser-stay-running",
		"--browser-state-file", c.paths.BrowserState,
	}
	command := exec.Command(c.corePath, arguments...)
	command.Stdout = io.MultiWriter(c.logFile)
	command.Stderr = io.MultiWriter(c.logFile)
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动 NWAFU Connect 后台连接: %w", err)
	}
	c.generation++
	generation := c.generation
	c.command = command
	done := make(chan struct{})
	c.commandDone = done
	c.publish(Status{Message: "正在连接学校网关…"})
	go c.waitForCommand(command, generation, done)
	go c.waitForReady(generation)
	return nil
}

func (c *Controller) stopLocked() {
	if c.command == nil {
		return
	}
	command := c.command
	done := c.commandDone
	c.command = nil
	c.generation++
	if command.Process != nil {
		terminateProcess(command, done)
	}
	_ = managedbrowser.RemoveState(c.paths.BrowserState)
	c.publish(Status{Message: "连接已停止"})
}

func (c *Controller) waitForCommand(command *exec.Cmd, generation uint64, done chan struct{}) {
	err := command.Wait()
	close(done)
	c.mutex.Lock()
	if c.generation != generation || c.command != command {
		c.mutex.Unlock()
		return
	}
	c.command = nil
	c.commandDone = nil
	c.mutex.Unlock()
	if err != nil {
		c.publish(Status{Message: "连接已中断，可从托盘重新连接"})
	} else {
		c.publish(Status{Message: "连接已结束"})
	}
}

func (c *Controller) waitForReady(generation uint64) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(45 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-timeout.C:
			c.publish(Status{Message: "连接耗时较长，请检查日志或认证信息"})
			return
		case <-ticker.C:
			if _, err := managedbrowser.ReadState(c.paths.BrowserState); err != nil {
				continue
			}
			c.mutex.Lock()
			current := c.generation == generation && c.command != nil
			c.mutex.Unlock()
			if current {
				c.publish(Status{Connected: true, Message: "已连接 · 受管浏览器可用"})
			}
			return
		}
	}
}

func (c *Controller) monitorWakeAndNetwork() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	lastTick := time.Now()
	wasOnline := true
	for {
		select {
		case <-c.ctx.Done():
			return
		case now := <-ticker.C:
			elapsed := now.Sub(lastTick)
			lastTick = now
			online := c.gatewayReachable()
			if c.ctx.Err() != nil {
				return
			}
			if shouldReconnect(elapsed, wasOnline, online) {
				_ = c.Restart()
			}
			wasOnline = online
		}
	}
}

func shouldReconnect(elapsed time.Duration, wasOnline, online bool) bool {
	return elapsed > 45*time.Second || (!wasOnline && online)
}

func (c *Controller) gatewayReachable() bool {
	configuration, _, err := c.store.Load()
	if err != nil {
		return false
	}
	address := net.JoinHostPort(stringValue(configuration.ServerAddress, "vpn.nwafu.edu.cn"), fmt.Sprint(intValue(configuration.ServerPort, 443)))
	connection, err := net.DialTimeout("tcp", address, 4*time.Second)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}

func (c *Controller) configuredLocked() (bool, error) {
	configuration, _, err := c.store.Load()
	if err != nil {
		return false, err
	}
	return configured(configuration), nil
}

func configured(configuration configs.ConfigTOML) bool {
	if stringValue(configuration.ServerAddress, "") == "" {
		return false
	}
	switch stringValue(configuration.AuthType, "auth/psw") {
	case "auth/psw":
		return stringValue(configuration.Username, "") != "" && stringValue(configuration.Password, "") != ""
	case "auth/smsCheckCode":
		return stringValue(configuration.Phone, "") != ""
	case "auth/qywechat":
		return true
	}
	return false
}

func (c *Controller) publish(status Status) {
	select {
	case c.status <- status:
	default:
		select {
		case <-c.status:
		default:
		}
		c.status <- status
	}
}

func findCoreExecutable(configured string) (string, error) {
	if configured != "" {
		return exec.LookPath(configured)
	}
	name := "nwafu-connect"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	current, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(current), name)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("找不到核心程序 %s；请重新安装 NWAFU Connect", name)
}

func stringValue(pointer *string, fallback string) string {
	if pointer == nil || *pointer == "" {
		return fallback
	}
	return *pointer
}
func intValue(pointer *int, fallback int) int {
	if pointer == nil {
		return fallback
	}
	return *pointer
}
