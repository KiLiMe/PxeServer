//go:build linux

package single

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAcquireSecondInstanceBlocked 覆盖「第二个实例拿不到锁」这条主路径：
// 第一次抢到，第二次 already=true，释放之后又能抢到。
func TestAcquireSecondInstanceBlocked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	release, already, err := Acquire("PXE Server")
	if err != nil {
		t.Fatalf("首次 Acquire 失败：%v", err)
	}
	if already {
		t.Fatal("首次 Acquire 不应报已有实例")
	}
	defer release()

	if _, already, err := Acquire("PXE Server"); err != nil {
		t.Fatalf("第二次 Acquire 失败：%v", err)
	} else if !already {
		t.Fatal("第二次 Acquire 应报已有实例")
	}

	release()
	if _, already, err := Acquire("PXE Server"); err != nil {
		t.Fatalf("释放后 Acquire 失败：%v", err)
	} else if already {
		t.Fatal("释放后应能重新抢到锁")
	}
}

// TestAcquireWritesPID 保证锁文件里有占用者 PID：窗口弹不出提示时靠它排查。
func TestAcquireWritesPID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	release, already, err := Acquire("PXE Server")
	if err != nil || already {
		t.Fatalf("Acquire 失败：err=%v already=%v", err, already)
	}
	defer release()

	data, err := os.ReadFile(filepath.Join(dir, "pxe-server.lock"))
	if err != nil {
		t.Fatalf("读锁文件失败：%v", err)
	}
	if got, want := strings.TrimSpace(string(data)), strconv.Itoa(os.Getpid()); got != want {
		t.Errorf("锁文件内容 = %q，期望 PID %q", got, want)
	}
}

// TestFocusUnsupported Linux 上拉不动已有窗口，界面只提示一句就退出。
func TestFocusUnsupported(t *testing.T) {
	if Focus("PXE 启动服务器") {
		t.Error("Linux 上 Focus 应返回 false")
	}
}
