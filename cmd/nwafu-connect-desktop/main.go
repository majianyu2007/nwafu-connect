package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/systray"
	"github.com/majianyu2007/nwafu-connect/internal/appdata"
	"github.com/majianyu2007/nwafu-connect/internal/autostart"
	desktopcontroller "github.com/majianyu2007/nwafu-connect/internal/desktop"
	"github.com/majianyu2007/nwafu-connect/internal/desktopconfig"
	"github.com/majianyu2007/nwafu-connect/internal/dockhide"
	"github.com/majianyu2007/nwafu-connect/internal/settingswindow"
)

//go:embed assets/icon.png
var trayIconPNG []byte

//go:embed assets/app-icon.png
var appIconPNG []byte

//go:embed assets/NotoSansSC-Regular.otf
var notoSansSC []byte

func main() {
	corePath := flag.String("core", "", "path to the nwafu-connect core executable")
	noAutoConnect := flag.Bool("no-auto-connect", false, "start the tray without connecting (for diagnostics)")
	flag.Parse()
	dockhide.Hide()

	paths, err := appdata.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	store := desktopconfig.Store{ConfigPath: paths.Config, PreferencesPath: filepath.Join(paths.Root, "desktop.json"), ClientDataPath: paths.ClientData}
	controller, err := desktopcontroller.NewController(paths, store, *corePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	application := app.NewWithID("com.nwafu.connect.desktop")
	application.Settings().SetTheme(newCJKTheme())
	application.Lifecycle().SetOnStarted(dockhide.Hide)
	application.Lifecycle().SetOnEnteredForeground(dockhide.Hide)
	appIcon := fyne.NewStaticResource("NWAFUConnect.png", appIconPNG)
	trayIcon := theme.NewThemedResource(fyne.NewStaticResource("NWAFUConnectTray.png", trayIconPNG))
	application.SetIcon(appIcon)
	executable, _ := os.Executable()
	settings, err := settingswindow.New(application, store, appIcon, func(preferences desktopconfig.Preferences) error {
		if err := autostart.Set(preferences.LaunchAtLogin, executable); err != nil {
			return fmt.Errorf("更新开机启动设置: %w", err)
		}
		go func() {
			if err := controller.Restart(); err != nil {
				fyne.Do(func() {
					application.SendNotification(fyne.NewNotification("NWAFU Connect", err.Error()))
				})
			}
		}()
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = controller.Stop()
		os.Exit(1)
	}

	desktopApplication, ok := application.(desktop.App)
	if !ok {
		fmt.Fprintln(os.Stderr, "当前图形环境不支持系统托盘")
		_ = controller.Stop()
		os.Exit(1)
	}

	var proxyMu sync.Mutex
	currentProxy := ""

	var trayMenu *fyne.Menu
	statusItem := fyne.NewMenuItem("○ 尚未连接", nil)
	statusItem.Disabled = true

	notifyError := func(err error) {
		showError(application, desktopApplication, trayMenu, statusItem, err)
	}

	openBrowserItem := fyne.NewMenuItemWithIcon("打开受管浏览器", theme.ComputerIcon(), func() {
		if err := controller.OpenBrowser(); err != nil {
			notifyError(err)
		}
	})
	openBrowserItem.Disabled = true

	copyProxyItem := fyne.NewMenuItem("复制本地代理地址", func() {
		proxyMu.Lock()
		addr := currentProxy
		proxyMu.Unlock()
		if addr == "" {
			return
		}
		application.Clipboard().SetContent("http://" + addr)
		application.SendNotification(fyne.NewNotification("NWAFU Connect", "已复制代理地址 http://"+addr))
	})
	copyProxyItem.Disabled = true

	reconnectItem := fyne.NewMenuItemWithIcon("重新连接", theme.ViewRefreshIcon(), func() {
		go func() {
			if err := controller.Restart(); err != nil {
				fyne.Do(func() { notifyError(err) })
			}
		}()
	})

	disconnectItem := fyne.NewMenuItem("断开连接", func() {
		go func() {
			if err := controller.Disconnect(); err != nil {
				fyne.Do(func() { notifyError(err) })
			}
		}()
	})
	disconnectItem.Disabled = true

	settingsItem := fyne.NewMenuItemWithIcon("设置…", theme.SettingsIcon(), settings.Show)
	dataItem := fyne.NewMenuItem("打开私有数据目录", func() {
		if err := openPath(paths.Root); err != nil {
			notifyError(err)
		}
	})
	logItem := fyne.NewMenuItem("查看运行日志", func() {
		if err := openPath(paths.Log); err != nil {
			notifyError(err)
		}
	})
	quitItem := fyne.NewMenuItem("退出 NWAFU Connect", application.Quit)
	quitItem.IsQuit = true

	trayMenu = fyne.NewMenu(
		"NWAFU Connect",
		statusItem,
		fyne.NewMenuItemSeparator(),
		openBrowserItem,
		copyProxyItem,
		reconnectItem,
		disconnectItem,
		settingsItem,
		fyne.NewMenuItemSeparator(),
		dataItem,
		logItem,
		fyne.NewMenuItemSeparator(),
		quitItem,
	)

	desktopApplication.SetSystemTrayIcon(trayIcon)
	desktopApplication.SetSystemTrayMenu(trayMenu)
	systray.SetOnTapped(func() {
		fmt.Fprintln(os.Stderr, "托盘已点击：连接并启动受管浏览器")
		go func() {
			if err := controller.Activate(); err != nil {
				fyne.Do(func() { notifyError(err) })
			}
		}()
	})

	go func() {
		for status := range controller.Status() {
			status := status
			proxyMu.Lock()
			currentProxy = status.ProxyAddress
			proxyMu.Unlock()
			fyne.Do(func() {
				if status.Connected {
					statusItem.Label = "● " + status.Message
					openBrowserItem.Disabled = false
					copyProxyItem.Disabled = status.ProxyAddress == ""
					if status.ProxyAddress != "" {
						copyProxyItem.Label = "复制代理 (" + status.ProxyAddress + ")"
					} else {
						copyProxyItem.Label = "复制本地代理地址"
					}
					disconnectItem.Disabled = false
				} else {
					statusItem.Label = "○ " + status.Message
					openBrowserItem.Disabled = true
					copyProxyItem.Disabled = true
					copyProxyItem.Label = "复制本地代理地址"
					disconnectItem.Disabled = status.Message == "连接已停止" || status.Message == "连接已结束"
				}
				desktopApplication.SetSystemTrayMenu(trayMenu)
			})
		}
	}()

	configured, err := controller.Configured()
	if err != nil {
		notifyError(err)
	}
	if !configured {
		settings.Show()
	} else if !*noAutoConnect {
		go func() {
			if err := controller.Start(); err != nil {
				fyne.Do(func() { notifyError(err) })
			}
		}()
	}

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-shutdownSignal
		fyne.Do(application.Quit)
	}()
	application.Run()
	signal.Stop(shutdownSignal)
	_ = controller.Stop()
}

func showError(application fyne.App, desktopApplication desktop.App, trayMenu *fyne.Menu, statusItem *fyne.MenuItem, err error) {
	statusItem.Label = "! " + err.Error()
	if desktopApplication != nil && trayMenu != nil {
		desktopApplication.SetSystemTrayMenu(trayMenu)
	}
	application.SendNotification(fyne.NewNotification("NWAFU Connect", err.Error()))
}

func openPath(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", path)
	case "windows":
		command = exec.Command("explorer.exe", path)
	case "linux":
		command = exec.Command("xdg-open", path)
	default:
		return fmt.Errorf("当前系统不支持打开路径")
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
