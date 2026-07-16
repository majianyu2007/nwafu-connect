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
				application.SendNotification(fyne.NewNotification("NWAFU Connect", err.Error()))
			}
		}()
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = controller.Stop()
		os.Exit(1)
	}

	statusItem := fyne.NewMenuItem("○ 尚未连接", nil)
	statusItem.Disabled = true
	openBrowserItem := fyne.NewMenuItemWithIcon("打开受管浏览器", theme.ComputerIcon(), func() {
		if err := controller.OpenBrowser(); err != nil {
			showError(application, statusItem, err)
		}
	})
	openBrowserItem.Disabled = true
	reconnectItem := fyne.NewMenuItemWithIcon("重新连接", theme.ViewRefreshIcon(), func() {
		go func() {
			if err := controller.Restart(); err != nil {
				fyne.Do(func() { showError(application, statusItem, err) })
			}
		}()
	})
	settingsItem := fyne.NewMenuItemWithIcon("设置…", theme.SettingsIcon(), settings.Show)
	dataItem := fyne.NewMenuItem("打开私有数据目录", func() {
		if err := openPath(paths.Root); err != nil {
			showError(application, statusItem, err)
		}
	})
	logItem := fyne.NewMenuItem("查看运行日志", func() {
		if err := openPath(paths.Log); err != nil {
			showError(application, statusItem, err)
		}
	})
	quitItem := fyne.NewMenuItem("退出 NWAFU Connect", application.Quit)
	quitItem.IsQuit = true
	trayMenu := fyne.NewMenu("NWAFU Connect", statusItem, fyne.NewMenuItemSeparator(), openBrowserItem, reconnectItem, settingsItem, fyne.NewMenuItemSeparator(), dataItem, logItem, fyne.NewMenuItemSeparator(), quitItem)

	desktopApplication, ok := application.(desktop.App)
	if !ok {
		fmt.Fprintln(os.Stderr, "当前图形环境不支持系统托盘")
		_ = controller.Stop()
		os.Exit(1)
	}
	desktopApplication.SetSystemTrayIcon(trayIcon)
	desktopApplication.SetSystemTrayMenu(trayMenu)
	systray.SetOnTapped(func() {
		fmt.Fprintln(os.Stderr, "托盘已点击：连接并启动受管浏览器")
		go func() {
			if err := controller.Activate(); err != nil {
				fyne.Do(func() { showError(application, statusItem, err) })
			}
		}()
	})

	go func() {
		for status := range controller.Status() {
			status := status
			fyne.Do(func() {
				if status.Connected {
					statusItem.Label = "● " + status.Message
					openBrowserItem.Disabled = false
				} else {
					statusItem.Label = "○ " + status.Message
					openBrowserItem.Disabled = true
				}
				desktopApplication.SetSystemTrayMenu(trayMenu)
			})
		}
	}()

	configured, err := controller.Configured()
	if err != nil {
		showError(application, statusItem, err)
	}
	if !configured {
		settings.Show()
	} else if !*noAutoConnect {
		go func() {
			if err := controller.Start(); err != nil {
				fyne.Do(func() { showError(application, statusItem, err) })
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

func showError(application fyne.App, statusItem *fyne.MenuItem, err error) {
	statusItem.Label = "! " + err.Error()
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
	return command.Start()
}
