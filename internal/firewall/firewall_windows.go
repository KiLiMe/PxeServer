package firewall

// 防火墙放行进程序内：按「程序路径」放行（不是按端口），
// 需要管理员时用 UAC 自己提权自己（同一个 exe 带 -firewall-apply 参数重跑一次）。
//
// 布局：
//   firewall_windows.go  对外策略：放行、撤销、状态、提示
//   rule_windows.go      PowerShell 规则增删查
//   elevate_windows.go   UAC 提权重跑自己 + 结果文件
//   marker_windows.go    「上次放行成功」标记（config.json 的 runtime 段）
//   firewall_other.go    非 Windows 的占位实现
//   main.go              最前面拦截 -firewall-apply，跑完退出，不启动 GUI

import (
	"os"

	"golang.org/x/sys/windows"
)

// Supported 报告本平台是否有「按程序放行防火墙」这回事：Windows 有，其它平台没有。
func Supported() bool { return true }

// HandleCLI 处理提权后的子进程调用；返回 true 表示已经处理完，应直接退出。
func HandleCLI() bool {
	if len(os.Args) < 2 {
		return false
	}
	action := ""
	for _, a := range os.Args[1:] {
		if a == flagApply {
			action = "apply"
		}
		if a == flagRemove {
			action = "remove"
		}
	}
	if action == "" {
		return false
	}
	msg := "ok"
	var err error
	if action == "apply" {
		err = applyRules()
	} else {
		err = removeRules()
	}
	if err != nil {
		msg = err.Error()
	}
	writeResult(msg)
	return true
}

// Apply 按程序路径放行入站（不限端口/协议）。
// 当前进程不是管理员时会弹 UAC，由提权后的自己完成操作。
func Apply() error {
	if isElevated() {
		return applyRules()
	}
	return elevate(flagApply)
}

// Remove 撤销本程序添加的防火墙规则（非管理员时同样走提权）。
func Remove() error {
	if isElevated() {
		return removeRules()
	}
	return elevate(flagRemove)
}

// State 返回放行状态：on / off / unknown。
// 管理员直接查规则；查不动时退回放行标记。
// 非管理员读不到规则（系统拒绝访问），按标记判断，没有标记就按未放行显示。
func State() string {
	if isElevated() {
		if s := stateElevated(); s != "unknown" {
			return s
		}
	}
	return stateFromMarker(markerMatches())
}

// stateFromMarker 在查不到规则时只用放行标记下结论：
// 没标记就当没放行，给出可操作的结论，而不是没有下文的「未知」。
func stateFromMarker(marked bool) string {
	if marked {
		return "on"
	}
	return "off"
}

// Hint 给启动日志用：没放行过就给一句提示。
func Hint() string {
	if markerMatches() {
		return ""
	}
	return "防火墙：尚未按程序路径放行，若客户端拿不到 IP 或文件，点「设置 → 放行程序」"
}

// ExePath 返回当前可执行文件路径（给日志用），取不到时用 "?"。
func ExePath() string {
	exe, err := program()
	if err != nil {
		return "?"
	}
	return exe
}

// isElevated 判断当前进程是否已经是管理员（High/System 完整性）。
func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
