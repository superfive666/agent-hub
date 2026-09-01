package blobstore_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/superfive666/agent-hub/internal/blobstore"
)

// 用例按**需求**写，不按实现写。这个包的需求就三条：
//   1. 同样的内容进去，同样的位置出来，磁盘上只有一份（ADR-0011 第二条）
//   2. 不受信的输入永远变不成路径
//   3. 任何一条失败路径都不许在目录里留下半个文件

func newStore(t *testing.T, max int64) blobstore.Store {
	t.Helper()
	return blobstore.Store{Dir: t.TempDir(), MaxBytes: max}
}

// 目录里所有「看起来像 blob」的文件。用它判断磁盘上到底留下了什么。
func blobs(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("扫目录: %v", err)
	}
	return out
}

func TestPut内容相同时只落一份(t *testing.T) {
	s := newStore(t, 1<<20)

	sha1, n1, err := s.Put(strings.NewReader("同一份产物"))
	if err != nil {
		t.Fatalf("第一次 Put: %v", err)
	}
	sha2, n2, err := s.Put(strings.NewReader("同一份产物"))
	if err != nil {
		t.Fatalf("第二次 Put: %v", err)
	}

	if sha1 != sha2 {
		t.Errorf("同样的内容应该得到同样的 sha，得到 %s 和 %s", sha1, sha2)
	}
	if n1 != n2 {
		t.Errorf("同样的内容应该得到同样的字节数，得到 %d 和 %d", n1, n2)
	}
	// 去重是白送的收益，但也是需求：agent 把同一份报告发到两条 thread 里，
	// 不该占两份磁盘。
	if got := blobs(t, s.Dir); len(got) != 1 {
		t.Errorf("磁盘上应该只有 1 份内容，实际有 %d 份: %v", len(got), got)
	}
}

func TestPut回来的内容读得出且一字不差(t *testing.T) {
	s := newStore(t, 1<<20)
	const body = "第一行\n第二行\x00带个 NUL"

	sha, size, err := s.Put(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if size != int64(len(body)) {
		t.Errorf("size 应该是 %d，得到 %d", len(body), size)
	}

	f, info, err := s.Open(sha)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if info.Size() != int64(len(body)) {
		t.Errorf("Stat 的 size 应该是 %d，得到 %d", len(body), info.Size())
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("读: %v", err)
	}
	if string(got) != body {
		t.Errorf("读回来的内容不一致：%q", got)
	}
}

// 这条是 ADR-0011 第二条的核心：**不受信的输入永远变不成路径。**
//
// 这个包的 API 里压根没有「文件名」参数，所以能作为路径素材递进来的只有 sha。
// 那就把各种能想到的恶意 sha 都试一遍 —— 一条都不许碰到 Dir 外面的东西。
func TestOpen拒绝一切不是合法sha的东西(t *testing.T) {
	s := newStore(t, 1<<20)

	// 在 Dir 外面放一个「机密文件」，任何一次成功打开它都是穿越成功。
	outside := filepath.Join(filepath.Dir(s.Dir), "secret.txt")
	if err := os.WriteFile(outside, []byte("不该被读到"), 0o600); err != nil {
		t.Fatalf("准备外部文件: %v", err)
	}

	bad := []string{
		"../secret.txt",
		"../../etc/passwd",
		"..%2f..%2fsecret.txt",
		"....//secret.txt",
		"/etc/passwd",
		strings.Repeat("a", 63),                  // 短一位
		strings.Repeat("a", 65),                  // 长一位
		strings.Repeat("A", 64),                  // 大写：pathOf 只认小写
		strings.Repeat("g", 64),                  // 非十六进制
		strings.Repeat("a", 62) + "/x",           // 混进分隔符
		strings.Repeat("a", 60) + "\x00" + "aaa", // NUL 截断
		"",
	}
	for _, v := range bad {
		if _, _, err := s.Open(v); err == nil {
			t.Errorf("Open(%q) 居然成功了 —— 这是一次路径穿越", v)
		}
		// Remove 同样是拿 sha 拼路径的，一起守住
		if err := s.Remove(v); err != nil {
			t.Errorf("Remove(%q) 应该是无害的空操作，得到 %v", v, err)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("Dir 外面的文件被动了: %v", err)
	}
}

func TestPut超限时报错且不留半个文件(t *testing.T) {
	s := newStore(t, 10)

	_, _, err := s.Put(strings.NewReader(strings.Repeat("x", 11)))
	if !errors.Is(err, blobstore.ErrTooLarge) {
		t.Fatalf("超限应该返回 ErrTooLarge，得到 %v", err)
	}
	// 超限是边写边发现的 —— 发现的时候半个文件已经在临时文件里了。
	// 留着它的话，GC 要等一整个 TTL 才收得走。
	if got := blobs(t, s.Dir); len(got) != 0 {
		t.Errorf("超限之后目录里应该什么都不剩，实际有 %v", got)
	}
}

func TestPut正好等于上限是合法的(t *testing.T) {
	s := newStore(t, 10)
	if _, size, err := s.Put(strings.NewReader(strings.Repeat("x", 10))); err != nil {
		t.Fatalf("正好 10 字节应该通过，得到 %v", err)
	} else if size != 10 {
		t.Errorf("size 应该是 10，得到 %d", size)
	}
}

func TestPut拒绝空内容(t *testing.T) {
	s := newStore(t, 1<<20)
	if _, _, err := s.Put(strings.NewReader("")); err == nil {
		t.Error("空附件应该被拒绝 —— 库上 size_bytes > 0 是硬约束，" +
			"在这里拦住比让 INSERT 撞 CHECK 报 500 好懂")
	}
	if got := blobs(t, s.Dir); len(got) != 0 {
		t.Errorf("目录里应该什么都不剩，实际有 %v", got)
	}
}

func TestPut读到一半出错时不留半个文件(t *testing.T) {
	s := newStore(t, 1<<20)
	boom := errors.New("连接断了")

	_, _, err := s.Put(io.MultiReader(
		strings.NewReader(strings.Repeat("x", 4096)),
		errReader{boom},
	))
	if !errors.Is(err, boom) {
		t.Fatalf("应该把读取错误透出来，得到 %v", err)
	}
	if got := blobs(t, s.Dir); len(got) != 0 {
		t.Errorf("上传断在半路，目录里应该什么都不剩，实际有 %v", got)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestRemove是幂等的(t *testing.T) {
	s := newStore(t, 1<<20)
	sha, _, err := s.Put(strings.NewReader("删我"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Remove(sha); err != nil {
		t.Fatalf("第一次 Remove: %v", err)
	}
	// GC 会重复跑。「本来就没有」当成失败只会在日志里刷没意义的告警。
	if err := s.Remove(sha); err != nil {
		t.Errorf("第二次 Remove 应该无害，得到 %v", err)
	}
	if _, _, err := s.Open(sha); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("删掉之后 Open 应该是 ErrNotExist，得到 %v", err)
	}
}

// List 的 before 参数不是优化，是正确性：上传是「先落盘、后写库行」，
// 中间那一瞬间磁盘上有一个还没有任何行引用的文件。不看时间的扫盘会把
// 正在上传的文件删掉 —— 上传方拿到 201，下载时 404。
func TestList只给出早于before的内容(t *testing.T) {
	s := newStore(t, 1<<20)
	sha, _, err := s.Put(strings.NewReader("刚刚传上来的"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// 刚落盘的，一个 TTL 之前的时间点看不到它
	got, err := s.List(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("刚上传的内容不该被 GC 看见，得到 %v", got)
	}

	// 把 mtime 拨老，就该看得见了
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(s.Dir, sha[0:2], sha[2:4], sha), old, old); err != nil {
		t.Fatalf("改 mtime: %v", err)
	}
	got, err = s.List(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0] != sha {
		t.Errorf("应该列出 [%s]，得到 %v", sha, got)
	}
}

func TestSweepStale清掉遗留的临时文件但不动blob(t *testing.T) {
	s := newStore(t, 1<<20)
	sha, _, err := s.Put(strings.NewReader("正经内容"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// 进程在 Put 中途被杀会留下这种东西
	stale := filepath.Join(s.Dir, ".upload-abandoned")
	if err := os.WriteFile(stale, []byte("半个文件"), 0o600); err != nil {
		t.Fatalf("造临时文件: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("改 mtime: %v", err)
	}

	n, err := s.SweepStale(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("SweepStale: %v", err)
	}
	if n != 1 {
		t.Errorf("应该清掉 1 个临时文件，实际 %d", n)
	}
	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Error("临时文件应该被清掉了")
	}
	if _, _, err := s.Open(sha); err != nil {
		t.Errorf("正经内容不该被扫走: %v", err)
	}
}

func TestSweepStale不动还新鲜的临时文件(t *testing.T) {
	s := newStore(t, 1<<20)
	fresh := filepath.Join(s.Dir, ".upload-inflight")
	if err := os.WriteFile(fresh, []byte("正在传"), 0o600); err != nil {
		t.Fatalf("造临时文件: %v", err)
	}
	if n, err := s.SweepStale(time.Now().Add(-time.Hour)); err != nil || n != 0 {
		t.Errorf("正在传的临时文件不该被清掉，清了 %d 个（err=%v）", n, err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("正在传的临时文件被删了: %v", err)
	}
}

// 没配 ATTACHMENT_DIR 是**正常的部署状态**，不是错误状态。
// 每个方法都要给出同一个可识别的信号，调用方才能把它翻译成一个说得清楚的 503。
func TestZero值代表功能关闭(t *testing.T) {
	var s blobstore.Store

	if s.Enabled() {
		t.Error("空 Dir 应该是 Enabled() == false")
	}
	if _, _, err := s.Put(strings.NewReader("x")); !errors.Is(err, blobstore.ErrNotConfigured) {
		t.Errorf("Put 应该返回 ErrNotConfigured，得到 %v", err)
	}
	if _, _, err := s.Open(strings.Repeat("a", 64)); !errors.Is(err, blobstore.ErrNotConfigured) {
		t.Errorf("Open 应该返回 ErrNotConfigured，得到 %v", err)
	}
	if err := s.Remove(strings.Repeat("a", 64)); !errors.Is(err, blobstore.ErrNotConfigured) {
		t.Errorf("Remove 应该返回 ErrNotConfigured，得到 %v", err)
	}
	if _, err := s.List(time.Now()); !errors.Is(err, blobstore.ErrNotConfigured) {
		t.Errorf("List 应该返回 ErrNotConfigured，得到 %v", err)
	}
	if err := s.Check(); !errors.Is(err, blobstore.ErrNotConfigured) {
		t.Errorf("Check 应该返回 ErrNotConfigured，得到 %v", err)
	}
}

func TestCheck会真的写一下(t *testing.T) {
	s := newStore(t, 1<<20)
	if err := s.Check(); err != nil {
		t.Fatalf("可写目录上 Check 应该通过: %v", err)
	}
	// 自检不能留下痕迹
	if got := blobs(t, s.Dir); len(got) != 0 {
		t.Errorf("自检之后目录里应该什么都不剩，实际有 %v", got)
	}
}

// 最常见的配错：容器里跑 nonroot，宿主机上那个 bind mount 是 root 属主。
// 目录存在、能读、ls 得到，**但一写就 EACCES** —— 只查存在性的自检
// 会在这种情况下给出「一切正常」，然后每一次上传都 500。
func TestCheck在只读目录上失败并指出该怎么修(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root 无视文件权限位，这条在 root 下测不出来")
	}
	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatalf("造只读目录: %v", err)
	}
	s := blobstore.Store{Dir: dir, MaxBytes: 1 << 20}

	err := s.Check()
	if err == nil {
		t.Fatal("只读目录上 Check 必须失败 —— 否则这个配错要等到第一次上传才暴露")
	}
	// 运维拿到这条错误之后的下一步就是 chown，所以 uid 必须在里面
	if !strings.Contains(err.Error(), "uid") {
		t.Errorf("错误信息里要带 uid，运维才知道 chown 成谁：%v", err)
	}
}
