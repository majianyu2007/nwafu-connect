package settingswindow

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/majianyu2007/nwafu-connect/internal/desktopconfig"
)

type Window struct {
	window        fyne.Window
	store         desktopconfig.Store
	onSave        func(desktopconfig.Preferences) error
	server        *widget.Entry
	port          *widget.Entry
	authType      *widget.Select
	loginDomain   *widget.Entry
	username      *widget.Entry
	phone         *widget.Entry
	password      *widget.Entry
	totpSecret    *widget.Entry
	browserPath   *widget.Entry
	browserURL    *widget.Entry
	launchAtLogin *widget.Check
	status        *widget.Label
}

func New(application fyne.App, store desktopconfig.Store, icon fyne.Resource, onSave func(desktopconfig.Preferences) error) (*Window, error) {
	instance := &Window{window: application.NewWindow("NWAFU Connect 设置"), store: store, onSave: onSave}
	instance.window.SetIcon(icon)
	instance.window.Resize(fyne.NewSize(760, 680))
	instance.window.SetCloseIntercept(instance.window.Hide)

	instance.server = widget.NewEntry()
	instance.port = widget.NewEntry()
	instance.authType = widget.NewSelect([]string{"用户名与密码", "短信验证码", "企业微信扫码"}, nil)
	instance.loginDomain = widget.NewEntry()
	instance.username = widget.NewEntry()
	instance.phone = widget.NewEntry()
	instance.password = widget.NewPasswordEntry()
	instance.totpSecret = widget.NewPasswordEntry()
	instance.browserPath = widget.NewEntry()
	instance.browserURL = widget.NewEntry()
	instance.launchAtLogin = widget.NewCheck("登录系统后在托盘后台运行", nil)
	instance.status = widget.NewLabel("")
	instance.status.Wrapping = fyne.TextWrapWord

	instance.browserPath.SetPlaceHolder("留空自动查找 Chrome、Edge、Chromium 或 Brave")
	instance.browserURL.SetPlaceHolder("留空显示学校下发的完整资源导航页")
	instance.password.SetPlaceHolder("留空保留已保存密码")
	instance.totpSecret.SetPlaceHolder("留空保留已保存 TOTP 密钥")

	gatewayForm := widget.NewForm(
		widget.NewFormItem("服务器地址", instance.server),
		widget.NewFormItem("端口", instance.port),
		widget.NewFormItem("认证方式", instance.authType),
		widget.NewFormItem("登录域", instance.loginDomain),
		widget.NewFormItem("用户名", instance.username),
		widget.NewFormItem("手机号", instance.phone),
		widget.NewFormItem("密码", instance.password),
		widget.NewFormItem("TOTP 密钥", instance.totpSecret),
	)
	browserForm := widget.NewForm(
		widget.NewFormItem("Chromium 可执行文件", instance.browserPath),
		widget.NewFormItem("浏览器起始页", instance.browserURL),
	)
	policy := widget.NewCard("受管浏览器策略", "由桌面客户端强制执行", container.NewVBox(
		widget.NewLabel("✓ 所有浏览器 HTTP/HTTPS 流量经私有 NWAFU Connect 代理"),
		widget.NewLabel("✓ DNS 服务器地址由学校网关自动提供（DHCP / auto）"),
		widget.NewLabel("✓ 浏览器关闭后后台连接保持，可从托盘重新打开"),
		widget.NewLabel("✓ 使用独立且持久的 Chromium 资料目录"),
	))
	tabs := container.NewAppTabs(
		container.NewTabItem("学校网关", container.NewVScroll(gatewayForm)),
		container.NewTabItem("浏览器与后台", container.NewVScroll(container.NewVBox(browserForm, policy, instance.launchAtLogin))),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	saveButton := widget.NewButton("保存并重新连接", instance.save)
	saveButton.Importance = widget.HighImportance
	closeButton := widget.NewButton("隐藏窗口", instance.window.Hide)
	actions := container.NewHBox(layout.NewSpacer(), closeButton, saveButton)
	header := widget.NewCard("NWAFU Connect", "西北农林科技大学 aTrust 受管浏览器与后台连接设置", nil)
	instance.window.SetContent(container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), instance.status, actions),
		nil, nil, tabs,
	))
	if err := instance.reload(); err != nil {
		return nil, err
	}
	return instance, nil
}

func (w *Window) Show() {
	if err := w.reload(); err != nil {
		dialog.ShowError(err, w.window)
	}
	w.window.Show()
	w.window.RequestFocus()
}

func (w *Window) Native() fyne.Window {
	return w.window
}

func (w *Window) save() {
	port, err := strconv.Atoi(w.port.Text)
	if err != nil {
		dialog.ShowError(fmt.Errorf("端口必须是数字"), w.window)
		return
	}
	configuration, preferences, err := w.store.Load()
	if err != nil {
		dialog.ShowError(err, w.window)
		return
	}
	editable := desktopconfig.Editable{
		Server: w.server.Text, Port: port, Username: w.username.Text, Password: w.password.Text, TOTPSecret: w.totpSecret.Text,
		AuthType: authTypeValue(w.authType.Selected), Phone: w.phone.Text, LoginDomain: w.loginDomain.Text,
		BrowserPath: w.browserPath.Text, BrowserURL: w.browserURL.Text, LaunchAtLogin: w.launchAtLogin.Checked,
	}
	if err := desktopconfig.Apply(&configuration, &preferences, editable); err != nil {
		dialog.ShowError(err, w.window)
		return
	}
	if err := w.store.Save(configuration, preferences); err != nil {
		dialog.ShowError(err, w.window)
		return
	}
	if w.onSave != nil {
		if err := w.onSave(preferences); err != nil {
			dialog.ShowError(err, w.window)
			return
		}
	}
	w.password.SetText("")
	w.totpSecret.SetText("")
	w.status.SetText("配置已保存，后台连接正在重新建立。")
	dialog.ShowInformation("设置已保存", "NWAFU Connect 正在使用新配置重新连接。", w.window)
}

func (w *Window) reload() error {
	configuration, preferences, err := w.store.Load()
	if err != nil {
		return err
	}
	editable := desktopconfig.FromConfiguration(configuration, preferences)
	w.server.SetText(editable.Server)
	w.port.SetText(strconv.Itoa(editable.Port))
	w.authType.SetSelected(authTypeLabel(editable.AuthType))
	w.loginDomain.SetText(editable.LoginDomain)
	w.username.SetText(editable.Username)
	w.phone.SetText(editable.Phone)
	w.password.SetText("")
	w.totpSecret.SetText("")
	w.browserPath.SetText(editable.BrowserPath)
	w.browserURL.SetText(editable.BrowserURL)
	w.launchAtLogin.SetChecked(editable.LaunchAtLogin)
	if editable.HasPassword {
		w.password.SetPlaceHolder("密码已保存；留空不修改")
	}
	if editable.HasTOTP {
		w.totpSecret.SetPlaceHolder("TOTP 密钥已保存；留空不修改")
	}
	return nil
}

func authTypeValue(label string) string {
	switch label {
	case "短信验证码":
		return "auth/smsCheckCode"
	case "企业微信扫码":
		return "auth/qywechat"
	default:
		return "auth/psw"
	}
}

func authTypeLabel(value string) string {
	switch value {
	case "auth/smsCheckCode":
		return "短信验证码"
	case "auth/qywechat":
		return "企业微信扫码"
	default:
		return "用户名与密码"
	}
}
