package settingswindow

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/majianyu2007/nwafu-connect/internal/desktopconfig"
)

type Window struct {
	window         fyne.Window
	store          desktopconfig.Store
	onSave         func(desktopconfig.Preferences) error
	tabs           *container.AppTabs
	advancedTab    *container.TabItem
	server         *widget.Entry
	port           *widget.Entry
	authType       *widget.Select
	loginDomain    *widget.Entry
	username       *widget.Entry
	phone          *widget.Entry
	password       *widget.Entry
	totpSecret     *widget.Entry
	browserPath    *widget.Entry
	browserURL     *widget.Entry
	launchAtLogin  *widget.Check
	advancedConfig *widget.Entry
	status         *widget.Label

	reloading     bool
	advancedDirty bool
}

func New(application fyne.App, store desktopconfig.Store, icon fyne.Resource, onSave func(desktopconfig.Preferences) error) (*Window, error) {
	instance := &Window{window: application.NewWindow("NWAFU Connect 设置"), store: store, onSave: onSave}
	instance.window.SetIcon(icon)
	instance.window.Resize(fyne.NewSize(780, 700))
	instance.window.SetCloseIntercept(instance.window.Hide)

	instance.server = widget.NewEntry()
	instance.port = widget.NewEntry()
	instance.loginDomain = widget.NewEntry()
	instance.username = widget.NewEntry()
	instance.phone = widget.NewEntry()
	instance.password = widget.NewPasswordEntry()
	instance.totpSecret = widget.NewPasswordEntry()
	instance.browserPath = widget.NewEntry()
	instance.browserURL = widget.NewEntry()
	instance.launchAtLogin = widget.NewCheck("登录系统后在托盘后台运行", nil)
	instance.advancedConfig = widget.NewMultiLineEntry()
	instance.advancedConfig.Wrapping = fyne.TextWrapWord
	instance.advancedConfig.OnChanged = func(string) {
		if !instance.reloading {
			instance.advancedDirty = true
		}
	}
	instance.status = widget.NewLabel("")
	instance.status.Wrapping = fyne.TextWrapWord

	instance.authType = widget.NewSelect([]string{"用户名与密码", "短信验证码", "企业微信扫码"}, func(selected string) {
		if instance.reloading {
			return
		}
		current := strings.TrimSpace(instance.loginDomain.Text)
		if current == "" || isDefaultLoginDomain(current) {
			instance.loginDomain.SetText(defaultLoginDomainForAuthType(authTypeValue(selected)))
		}
	})

	instance.server.SetPlaceHolder("vpn.nwafu.edu.cn")
	instance.port.SetPlaceHolder("443")
	instance.loginDomain.SetPlaceHolder("LDAP / sms73926 / wechat")
	instance.username.SetPlaceHolder("学号或工号")
	instance.phone.SetPlaceHolder("86-13800138000（短信登录时填写）")
	instance.browserPath.SetPlaceHolder("留空自动查找 Chrome、Edge、Chromium 或 Brave")
	instance.browserURL.SetPlaceHolder("留空显示学校下发的完整资源导航页")
	instance.password.SetPlaceHolder("留空保留已保存密码")
	instance.advancedConfig.SetPlaceHolder("# 以 TOML 形式直接编辑 config.toml；保存即重新连接。常见字段见 README。")
	instance.totpSecret.SetPlaceHolder("留空保留已保存 TOTP 密钥（Base32）")

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
	campusHint := widget.NewCard("校园网提示", "仅校外访问需要连接 VPN", container.NewVBox(
		widget.NewLabel("在校园网内部无法直接访问 vpn.nwafu.edu.cn，且无需开启 VPN。"),
		widget.NewLabel("切换至校外网络后，托盘将自动检测网关可达状态并恢复连接。"),
	))
	browserForm := widget.NewForm(
		widget.NewFormItem("Chromium 可执行文件", instance.browserPath),
		widget.NewFormItem("浏览器起始页", instance.browserURL),
	)
	policy := widget.NewCard("受管浏览器策略", "独立隔离 · 仅代理校内授权资源", container.NewVBox(
		widget.NewLabel("• 浏览器 HTTP/HTTPS 请求仅允许访问网关授权资源，绝不泄漏到本机直连"),
		widget.NewLabel("• 校内域名优先使用网关 DNS，缺失时自动切换固定端点加密 DoH"),
		widget.NewLabel("• 不会改动系统代理设置，也不污染日常浏览器的历史与 Cookie"),
	))

	gatewayTab := container.NewTabItem("学校网关", container.NewVScroll(container.NewPadded(container.NewVBox(gatewayForm, campusHint))))
	browserTab := container.NewTabItem("浏览器与后台", container.NewVScroll(container.NewPadded(container.NewVBox(browserForm, policy, instance.launchAtLogin))))
	instance.advancedTab = container.NewTabItem("高级设置", container.NewPadded(container.NewBorder(
		widget.NewLabel("直接编辑 config.toml（TOML 格式）。在此页点击“保存并重新连接”将直接保留高级配置。"),
		widget.NewButtonWithIcon("放弃修改并重新加载", theme.ViewRefreshIcon(), instance.reloadAdvanced),
		nil, nil, container.NewVScroll(instance.advancedConfig),
	)))

	instance.tabs = container.NewAppTabs(gatewayTab, browserTab, instance.advancedTab)
	instance.tabs.SetTabLocation(container.TabLocationTop)

	saveButton := widget.NewButton("保存并重新连接", instance.save)
	saveButton.Importance = widget.HighImportance
	closeButton := widget.NewButton("隐藏窗口", instance.window.Hide)
	actions := container.NewPadded(container.NewHBox(layout.NewSpacer(), closeButton, saveButton))

	title := widget.NewLabelWithStyle("NWAFU Connect", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel("西北农林科技大学 aTrust 受管浏览器与后台连接设置")
	header := container.NewPadded(container.NewVBox(title, subtitle))

	instance.window.SetContent(container.NewBorder(
		container.NewVBox(header, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), container.NewPadded(instance.status), actions),
		nil, nil, instance.tabs,
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
	configuration, preferences, err := w.store.Load()
	if err != nil {
		dialog.ShowError(err, w.window)
		return
	}

	onAdvancedTab := w.tabs != nil && w.tabs.Selected() == w.advancedTab
	if onAdvancedTab {
		raw := strings.TrimSpace(w.advancedConfig.Text)
		if raw != "" {
			configuration, err = w.store.MergeRaw(configuration, []byte(raw))
			if err != nil {
				dialog.ShowError(err, w.window)
				return
			}
		}
		preferences.LaunchAtLogin = w.launchAtLogin.Checked
	} else {
		port, err := strconv.Atoi(strings.TrimSpace(w.port.Text))
		if err != nil {
			dialog.ShowError(fmt.Errorf("端口必须是数字"), w.window)
			return
		}
		if w.advancedDirty {
			if raw := strings.TrimSpace(w.advancedConfig.Text); raw != "" {
				configuration, err = w.store.MergeRaw(configuration, []byte(raw))
				if err != nil {
					dialog.ShowError(err, w.window)
					return
				}
			}
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
	_ = w.reload()
	w.status.SetText("配置已保存，后台连接正在重新建立。")
	dialog.ShowInformation("设置已保存", "NWAFU Connect 正在使用新配置重新连接。", w.window)
}

func (w *Window) reload() error {
	configuration, preferences, err := w.store.Load()
	if err != nil {
		return err
	}
	w.reloading = true
	defer func() {
		w.reloading = false
		w.advancedDirty = false
	}()
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
	} else {
		w.password.SetPlaceHolder("请输入密码")
	}
	if editable.HasTOTP {
		w.totpSecret.SetPlaceHolder("TOTP 密钥已保存；留空不修改")
	} else {
		w.totpSecret.SetPlaceHolder("留空保留已保存 TOTP 密钥（Base32）")
	}
	if raw, err := w.store.LoadRaw(); err == nil {
		w.advancedConfig.SetText(string(raw))
	} else {
		w.advancedConfig.SetText("")
	}
	return nil
}

// reloadAdvanced discards unsaved editor changes and reloads config.toml.
func (w *Window) reloadAdvanced() {
	if err := w.reload(); err != nil {
		dialog.ShowError(err, w.window)
		return
	}
	w.status.SetText("已从 config.toml 重新加载配置。")
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

func defaultLoginDomainForAuthType(authType string) string {
	switch authType {
	case "auth/smsCheckCode":
		return "sms73926"
	case "auth/qywechat":
		return "wechat"
	default:
		return "LDAP"
	}
}

func isDefaultLoginDomain(domain string) bool {
	return domain == "LDAP" || domain == "sms73926" || domain == "wechat"
}
