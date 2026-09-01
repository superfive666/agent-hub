// Package blobstore 是附件的磁盘存储：内容进来，sha256 出去。
//
// **它不认识文件名。** 整个包的 API 里没有一处接受调用方给的路径片段 ——
// 磁盘上的位置完全由内容的 sha256 算出来（ADR-0011 第二条）。
// 文件名是元数据，归 store 层的 attachment 表管，不归这里。
//
// 这不是洁癖。文件名来自 agent，是不受信输入；一旦它能参与拼路径，
// 就得跟 `../`、`..%2f`、`....//`、Unicode 规范化、符号链接这一长串
// 已知绕过手法赛跑。按内容寻址把「过滤得够不够干净」换成「结构上不可能」：
// 路径里每一个字节都是我们自己算出来的十六进制。
//
// api 写和读，worker 删（GC）。两个服务都挂同一个目录，所以这个包在 internal/。
package blobstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrNotConfigured 是「这台 hub 不收附件」。
//
// **留空 ATTACHMENT_DIR 是完全正常的部署**，和 ANDROID_APK_PATH 一样。
// 所以它是一个正常的返回值，不是错误状态 —— 调用方据此给出一个说得清楚的
// 503，而不是 500，也不是假装端点不存在的 404。
var ErrNotConfigured = errors.New("未配置 ATTACHMENT_DIR，这台 hub 不收附件")

// ErrEmpty 是「传上来的是个空文件」。
//
// 它是**调用方的错**，不是部署的错 —— 必须和「目录不可写」区分得开，
// 否则日志里会写着「多半是 ATTACHMENT_DIR 不可写」，而运维照着去查权限，
// 查半天发现权限一点问题都没有。
var ErrEmpty = errors.New("附件是空的")

// ErrTooLarge 是内容超过了单个附件的上限。
//
// 上传是边写边算的流式过程，超限时**已经有半个文件落在临时文件里了** ——
// Put 负责把它删掉再返回这个错误。调用方不需要自己清理。
var ErrTooLarge = errors.New("附件超过大小上限")

// Store 是一个目录。零值（Dir 为空）代表「没配置」，所有方法都返回 ErrNotConfigured。
//
// 做成零值可用，是为了让「没开这个功能」的部署不需要在每个调用点写 if。
type Store struct {
	// Dir 是 ATTACHMENT_DIR。空 = 功能关闭。
	Dir string
	// MaxBytes 是单个附件的上限。<=0 视为不限，但调用方应当总是给一个值。
	MaxBytes int64
}

// Enabled 报告这台 hub 收不收附件。
func (s Store) Enabled() bool { return strings.TrimSpace(s.Dir) != "" }

// Check 在启动时验证这个目录真的能写。
//
// **为什么不能只 Stat 一下就完事。** 容器里跑的是 nonroot，宿主机上那个
// bind mount 十有八九是 root 属主 —— 目录存在、能读、`ls` 得到，
// 但一写就 EACCES。只查存在性的自检会在这种最常见的配错上给出「一切正常」。
//
// 所以这里真的写一个文件再删掉。失败时返回的错误里带着 uid，
// 因为运维要的下一步就是 chown 成那个 uid。
//
// **它不该让服务起不来。** 附件是可选功能，为它拒绝启动会把
// 「附件传不了」升级成「整个平台没了」。调用方应当把这个错误记成 ERROR 日志
// 并让上传端点返回 503，而不是 os.Exit。
func (s Store) Check() error {
	if !s.Enabled() {
		return ErrNotConfigured
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("建附件目录 %s 失败（当前 uid=%d）: %w", s.Dir, os.Getuid(), err)
	}
	f, err := os.CreateTemp(s.Dir, ".writecheck-*")
	if err != nil {
		return fmt.Errorf("附件目录 %s 不可写（当前 uid=%d，多半是宿主机上那个目录的属主不对，"+
			"用 install -d -m 0750 -o %d -g %d 建它）: %w", s.Dir, os.Getuid(), os.Getuid(), os.Getgid(), err)
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// Put 把 r 里的内容落盘，返回内容的 sha256 与字节数。
//
// 先写临时文件、边写边算 sha、最后 rename 到最终位置 —— rename 在同一个文件系统里
// 是原子的，所以**最终路径上永远不会出现半个文件**。这一点很重要：GC 和下载
// 都只看最终路径，看到就认为内容完整。
//
// 内容已经存在时（去重）直接丢掉临时文件。同一份产物被两个 agent 各发一次，
// 磁盘上只有一份。
func (s Store) Put(r io.Reader) (sha string, size int64, err error) {
	if !s.Enabled() {
		return "", 0, ErrNotConfigured
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", 0, fmt.Errorf("建附件目录: %w", err)
	}

	tmp, err := os.CreateTemp(s.Dir, ".upload-*")
	if err != nil {
		return "", 0, fmt.Errorf("建临时文件: %w", err)
	}
	tmpName := tmp.Name()
	// 任何一条失败路径都不能把半个文件留在目录里 —— GC 是按 mtime 兜底的，
	// 留下来的垃圾要等一个 TTL 才会被清掉。
	defer func() {
		tmp.Close()
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	h := sha256.New()
	var src io.Reader = r
	if s.MaxBytes > 0 {
		// 多读一个字节：读满 MaxBytes+1 才说明真的超了，正好等于上限是合法的。
		src = io.LimitReader(r, s.MaxBytes+1)
	}
	size, err = io.Copy(io.MultiWriter(tmp, h), src)
	if err != nil {
		return "", 0, fmt.Errorf("写附件: %w", err)
	}
	if s.MaxBytes > 0 && size > s.MaxBytes {
		err = ErrTooLarge
		return "", 0, err
	}
	if size == 0 {
		// 空文件没有意义，而且 attachment 表上 size_bytes > 0 是硬约束 ——
		// 在这里拦住比让 INSERT 撞 CHECK 报 500 好懂得多。
		err = ErrEmpty
		return "", 0, err
	}
	if err = tmp.Sync(); err != nil {
		return "", 0, fmt.Errorf("刷盘: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("关闭临时文件: %w", err)
	}

	sha = hex.EncodeToString(h.Sum(nil))
	dst := s.pathOf(sha)
	if err = os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", 0, fmt.Errorf("建分片目录: %w", err)
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		// 已经有了（去重）。临时文件由 defer 删掉 —— 但 err 此刻是 nil，
		// defer 不会删，所以这里显式删。
		_ = os.Remove(tmpName)
		return sha, size, nil
	}
	if err = os.Rename(tmpName, dst); err != nil {
		return "", 0, fmt.Errorf("落盘: %w", err)
	}
	// rename 之后 tmpName 已经不存在了，defer 里的 Remove 是无害的空操作。
	return sha, size, nil
}

// Open 打开一份内容。返回的 *os.File 交给 http.ServeContent，
// 好处是免费拿到 Range 断点续传与 Content-Length。
func (s Store) Open(sha string) (*os.File, fs.FileInfo, error) {
	if !s.Enabled() {
		return nil, nil, ErrNotConfigured
	}
	if !validSHA(sha) {
		// 走到这里说明库里的 sha256 不合法，或者调用方把别的东西当 sha 传了进来。
		// 无论哪种都不该去碰文件系统。
		return nil, nil, fs.ErrNotExist
	}
	f, err := os.Open(s.pathOf(sha))
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		f.Close()
		return nil, nil, fs.ErrNotExist
	}
	return f, info, nil
}

// Remove 删掉一份内容。已经不在了不算错误 —— GC 会重复跑，
// 把「本来就没有」当成失败只会在日志里刷一堆没意义的告警。
func (s Store) Remove(sha string) error {
	if !s.Enabled() {
		return ErrNotConfigured
	}
	if !validSHA(sha) {
		return nil
	}
	if err := os.Remove(s.pathOf(sha)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// List 列出磁盘上所有**修改时间早于 before** 的内容的 sha256。
//
// GC 用它找「磁盘上有、库里没有」的失联 blob。
//
// **before 这个参数不是优化，是正确性。** 上传是「先落盘、后写库行」，
// 中间那一瞬间磁盘上就有一个还没有任何行引用它的文件。不看时间的扫盘
// 会把正在上传的文件删掉，而且是静默的 —— 上传方拿到 201，下载时 404。
//
// 临时文件（`.upload-*` / `.writecheck-*`）也一并返回不了：它们的名字不是
// 合法 sha，会被跳过。它们由 Put 自己的 defer 清理；真漏了的话
// SweepStale 负责收尾。
func (s Store) List(before time.Time) ([]string, error) {
	if !s.Enabled() {
		return nil, ErrNotConfigured
	}
	var out []string
	err := filepath.WalkDir(s.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// 单个目录读不了不该让整轮 GC 失败 —— 少清几个 blob 是可接受的，
			// 一轮都不跑不是。
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !validSHA(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.ModTime().Before(before) {
			return nil
		}
		out = append(out, name)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return out, nil
}

// SweepStale 清掉遗留的临时文件（进程在 Put 中途被杀会留下它们）。
// 同样只动早于 before 的，理由和 List 一样。返回删掉的个数。
func (s Store) SweepStale(before time.Time) (int, error) {
	if !s.Enabled() {
		return 0, ErrNotConfigured
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(before) {
			continue
		}
		if os.Remove(filepath.Join(s.Dir, e.Name())) == nil {
			n++
		}
	}
	return n, nil
}

// pathOf 是「内容 → 位置」这条唯一的映射。
//
// 分两层子目录是为了别让一个目录塞进几十万个文件 —— ext4 撑得住，
// 但 `ls` 撑不住，而出事时人是要进去 ls 的。
func (s Store) pathOf(sha string) string {
	return filepath.Join(s.Dir, sha[0:2], sha[2:4], sha)
}

// validSHA 是这个包唯一的守门人：只有 64 个小写十六进制字符能变成路径。
//
// 不用正则，不是为了快，是为了这段判断本身足够短、短到不需要再去信任别的东西。
func validSHA(v string) bool {
	if len(v) != 64 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
