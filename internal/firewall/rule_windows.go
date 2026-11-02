package firewall

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

// ruleName 是本程序创建的规则显示名，也是删除/查询时的唯一依据。
const ruleName = "PXE Server（程序路径放行）"

// program 返回要放行的程序路径（就是自己）。
func program() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("取不到程序路径：%w", err)
	}
	return exe, nil
}

// psQuote 把路径转义进单引号字符串，避免路径里的 ' 破坏脚本。
func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

func runPowerShell(script string) error {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			text = err.Error()
		}
		return fmt.Errorf("%s", text)
	}
	return nil
}

// applyRules 先删同名旧规则，再按程序路径建一条入站放行规则。
func applyRules() error {
	exe, err := program()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(exe); statErr != nil {
		return fmt.Errorf("程序文件不存在：%s", exe)
	}
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
Get-NetFirewallRule -DisplayName '%s' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName '%s' -Direction Inbound -Program '%s' -Action Allow -Profile Any -Enabled True | Out-Null`,
		psQuote(ruleName), psQuote(ruleName), psQuote(exe))
	if err := runPowerShell(script); err != nil {
		return fmt.Errorf("添加防火墙规则失败：%s", err)
	}
	if err := mark(exe); err != nil {
		// 规则已经加上了，只是标记没落盘；只影响非管理员时的状态显示。
		return fmt.Errorf("规则已添加，但写 config.json 失败：%w", err)
	}
	return nil
}

// removeRules 删除本程序加的规则。
func removeRules() error {
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
Get-NetFirewallRule -DisplayName '%s' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue`,
		psQuote(ruleName))
	if err := runPowerShell(script); err != nil {
		return fmt.Errorf("删除防火墙规则失败：%s", err)
	}
	_ = unmark()
	return nil
}

// stateElevated 管理员直接查规则：规则在、且程序路径就是自己，才算 on。
func stateElevated() string {
	exe, err := program()
	if err != nil {
		return "unknown"
	}
	script := fmt.Sprintf(`$r = Get-NetFirewallRule -DisplayName '%s' -ErrorAction SilentlyContinue
if (-not $r) { 'off'; exit }
$f = Get-NetFirewallApplicationFilter -AssociatedNetFirewallRule $r -ErrorAction SilentlyContinue
if ($f -and $f.Program -eq '%s') { 'on' } else { 'off' }`,
		psQuote(ruleName), psQuote(exe))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	switch strings.TrimSpace(string(out)) {
	case "on":
		return "on"
	case "off":
		return "off"
	}
	return "unknown"
}
