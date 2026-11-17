//go:build windows

package single

import (
	"fmt"
	"os"
	"testing"
)

// TestAcquireRejectsSecond：同名互斥体第二次获取要报告「已有实例」。
func TestAcquireRejectsSecond(t *testing.T) {
	name := fmt.Sprintf("Local\\pxeserv-test-%d", os.Getpid())
	release, already, err := Acquire(name)
	if err != nil {
		t.Fatalf("首次获取失败：%v", err)
	}
	if already {
		t.Fatal("首次获取不该报告已有实例")
	}
	defer release()

	release2, already2, err := Acquire(name)
	if err != nil {
		t.Fatalf("第二次获取不该报错：%v", err)
	}
	defer release2()
	if !already2 {
		t.Fatal("第二次获取应报告已有实例")
	}
}

// TestAcquireAfterRelease：前一个实例释放后，同名互斥体可以再次获取。
func TestAcquireAfterRelease(t *testing.T) {
	name := fmt.Sprintf("Local\\pxeserv-test-release-%d", os.Getpid())
	release, already, err := Acquire(name)
	if err != nil || already {
		t.Fatalf("首次获取：already=%v err=%v", already, err)
	}
	release()

	release2, already2, err := Acquire(name)
	if err != nil {
		t.Fatalf("释放后获取失败：%v", err)
	}
	defer release2()
	if already2 {
		t.Fatal("释放后应能重新获取")
	}
}
