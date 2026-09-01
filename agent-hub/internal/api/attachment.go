package api

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/superfive666/agent-hub/internal/blobstore"
	"github.com/superfive666/agent-hub/internal/domain"
	"github.com/superfive666/agent-hub/internal/store"
)

// ErrAttachmentsOff 是「这台 hub 不收附件」。
//
// **503 而不是 404**，理由和 APK 那条一样：端点在，只是这个部署没开这个功能。
// 404 会把人引去查路由，而真正要查的是 ATTACHMENT_DIR。
//
// Retryable 是 false：等多久都不会变，要人去改部署。
var ErrAttachmentsOff = Error{
	Code:      "attachments_unavailable",
	Message:   "这台 hub 没有开附件：需要配置 ATTACHMENT_DIR 并保证该目录对服务进程可写",
	Retryable: false,
}

// tooLargeErr 是超过单个附件上限。**消息里必须带上具体的上限值** ——
// 不带的话 agent 只能二分猜，而它完全可以在下一次请求前自己切分或压缩。
// 所以它是个函数而不是包级变量：上限来自配置。
func tooLargeErr(max int64) Error {
	return Error{
		Code:      "attachment_too_large",
		Message:   "附件超过大小上限（最大 " + humanBytes(max) + "）",
		Retryable: false,
	}
}

// humanBytes 把字节数写成人和 agent 都读得顺的样子。
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return itoa(int(n>>20)) + " MiB"
	case n >= 1<<10:
		return itoa(int(n>>10)) + " KiB"
	default:
		return itoa(int(n)) + " B"
	}
}

// ErrAttachmentRejected 是「这些附件挂不到这条帖子上」。
// 三种原因（不存在 / 已被别的帖子挂了 / 不是你传的）合并成一条，见 store 层的注释。
var ErrAttachmentRejected = Error{
	Code:      "attachment_rejected",
	Message:   "附件不存在、已经挂在别的帖子上，或者不是你传的。重新上传一次",
	Retryable: false,
}

// ErrEmptyAttachment 是「传了个空文件」。400 而不是 500：
// 这是调用方的问题，重试同样一个空文件永远不会成功。
var ErrEmptyAttachment = Error{
	Code:      "attachment_empty",
	Message:   "附件是空的，没有内容可存",
	Retryable: false,
}

// blobs 是这台 hub 的附件存储。零值 = 没开。
func (s *Server) blobs() blobstore.Store {
	return blobstore.Store{Dir: s.cfg.AttachmentDir, MaxBytes: s.cfg.AttachmentMaxBytes}
}

// handleAgentUpload / handleAdminUpload 是两步上传的第一步。
//
// 两侧对称：agent 交产物，人也要能把一份规格书或一张截图递给 agent。
// 只有身份不同，落盘、限额、类型归一化走的是同一段代码。
func (s *Server) handleAgentUpload(w http.ResponseWriter, r *http.Request) {
	s.upload(w, r, "agent", agentFrom(r))
}

func (s *Server) handleAdminUpload(w http.ResponseWriter, r *http.Request) {
	s.upload(w, r, "admin", "")
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request, kind string, who domain.AgentID) {
	bs := s.blobs()
	if !bs.Enabled() {
		writeErr(w, ErrAttachmentsOff)
		return
	}

	// 用 MultipartReader 流式读，不用 ParseMultipartForm：后者会先把整个请求
	// 落到 os.TempDir 的临时文件里，等于同一份内容在磁盘上写两遍，而且写的是
	// 一个我们没挂卷、可能很小的目录。流式读还让 blobstore 能边写边算 sha。
	//
	// MaxBytesReader 是**外层**的硬闸：它拦的是整个请求体（含 multipart 头部
	// 与其它字段），blobstore 的 MaxBytes 拦的是文件内容本身。少了外层这道，
	// 一个不带 boundary 结束符的请求可以让我们一直读下去。
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.AttachmentMaxBytes+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, ErrBadRequest)
		return
	}

	for {
		part, err := mr.NextPart()
		if err != nil {
			// 读完了所有 part 都没见到 file 字段
			writeErr(w, ErrBadRequest)
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}

		filename := safeFilename(part.FileName())
		ct := normalizeContentType(part.Header.Get("Content-Type"), filename)

		sha, size, err := bs.Put(part)
		part.Close()
		if err != nil {
			switch {
			case errors.Is(err, blobstore.ErrTooLarge):
				writeErr(w, tooLargeErr(s.cfg.AttachmentMaxBytes))
			case errors.Is(err, blobstore.ErrEmpty):
				// 调用方的错，不是部署的错。不记 ERROR —— 记了运维会照着
				// 「多半是目录不可写」去查权限，而权限一点问题都没有。
				writeErr(w, ErrEmptyAttachment)
			case errors.Is(err, blobstore.ErrNotConfigured):
				writeErr(w, ErrAttachmentsOff)
			default:
				// 目录不可写会落到这里。这是**部署配错**，不是请求的问题，
				// 所以日志要指得足够明白 —— 运维只会看到这一条。
				s.log.Error("附件落盘失败（多半是 ATTACHMENT_DIR 不可写）",
					"dir", s.cfg.AttachmentDir, "err", err)
				writeErr(w, ErrInternal)
			}
			return
		}

		a, err := s.store.CreateAttachment(r.Context(), store.NewAttachment{
			SHA256: sha, SizeBytes: size, ContentType: ct, Filename: filename,
			UploaderKind: kind, UploaderID: who,
		})
		if err != nil {
			// 文件已经落盘了但库里没记上 —— 那份内容成了失联 blob，
			// 由 worker 的 GC 收走。不在这里删：万一是去重命中的已有内容，
			// 删掉会把别人的附件一起干掉。
			s.log.Error("记录附件失败", "sha", sha, "err", err)
			writeErr(w, ErrInternal)
			return
		}
		writeJSON(w, http.StatusCreated, a)
		return
	}
}

// handleAttachmentDownload 把附件交出去。agent 侧与 admin 侧共用。
//
// 鉴权跟着 thread 走，也就是「登录了就能读」（ADR-0011 第六条）——
// GET /api/agent/threads/{id} 本来就对任何持有效凭证的 agent 开放，
// 附件不另发明一套更严的：两套不一致的可见性规则里，宽的那套决定实际暴露面，
// 严的那套只提供虚假的安全感。
func (s *Server) handleAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	bs := s.blobs()
	if !bs.Enabled() {
		writeErr(w, ErrAttachmentsOff)
		return
	}
	a, err := s.store.AttachmentByID(r.Context(), r.PathValue("attachmentID"))
	if err != nil {
		writeErr(w, ErrNotFound)
		return
	}
	f, info, err := bs.Open(a.SHA256)
	if err != nil {
		// 库里有行、磁盘上没内容。这是**真正的不一致**，不是用户错误 ——
		// 要么 GC 删错了，要么这台机器的卷没挂上。必须留一条能查的日志。
		if !errors.Is(err, fs.ErrNotExist) {
			s.log.Error("打开附件失败", "id", a.ID, "sha", a.SHA256, "err", err)
		} else {
			s.log.Error("附件在库里有记录、磁盘上却没有内容（GC 删错了？卷没挂上？）",
				"id", a.ID, "sha", a.SHA256, "dir", s.cfg.AttachmentDir)
		}
		writeErr(w, ErrNotFound)
		return
	}
	defer f.Close()

	writeAttachmentHeaders(w, a)
	// ServeContent 而不是 io.Copy：免费拿到 Range 断点续传、If-Modified-Since
	// 和 Content-Length。name 传空串，免得它拿文件名去猜类型盖掉我们定死的那个。
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// writeAttachmentHeaders 是这个功能的安全边界，三道一起上（ADR-0011 第四条）。
//
// 附件和控制台**同源**。一个 agent 上传一份 HTML，如果浏览器肯把它当页面渲染，
// 那就是一个挂在管理员会话上的存储型 XSS —— 上传的是 agent，中招的是这个平台上
// 唯一的那个人。
func writeAttachmentHeaders(w http.ResponseWriter, a store.Attachment) {
	w.Header().Set("Content-Type", a.ContentType)

	// ① 直接访问就是下载，不是渲染。
	//    界面上的图片预览不受影响：<img src> 不看 Content-Disposition。
	//    所以「点开能看图」和「直接访问只能下载」可以同时成立。
	w.Header().Set("Content-Disposition", contentDisposition(a.Filename))

	// ② 不许浏览器忽略我们给的类型自己猜。少了它，一份 Content-Type 是
	//    text/plain 但内容以 <html> 开头的文件，在老浏览器里会被当页面渲染。
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// ③ 万一前两道被绕过，这一层里没有脚本、没有外链、没有同源身份。
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")

	// 内容寻址意味着同一个 id 的字节永远不变，可以放心让浏览器长期缓存。
	// private：这是登录后才能拿的东西，不该进共享缓存。
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+a.SHA256+`"`)
}

// contentDisposition 拼那个头，并且**假定文件名是敌意的**。
//
// filename 来自上传方。它会被放进一个 HTTP 头里，而 HTTP 头是用换行分隔的 ——
// 一个带 \r\n 的文件名可以在这里劈开响应头，塞进任意一个新头（响应拆分）。
// 引号同理，能提前闭合 filename="..."。
//
// 所以：ASCII 那一份只留最保守的一小撮字符；真实文件名（中文、空格、emoji）
// 走 RFC 5987 的 filename*，那一份是百分号编码的，结构上带不出分隔符。
func contentDisposition(filename string) string {
	return "attachment; filename=\"" + asciiFallbackName(filename) + "\"; filename*=UTF-8''" +
		encodeRFC5987(filename)
}

// asciiFallbackName 是给不认识 filename* 的老客户端看的那一份。
//
// **扩展名要单独保下来。** 「构建报告.txt」里没有一个 ASCII 字母，
// 逐字符过滤之后是 "____.txt"，再把开头的下划线和点修掉就只剩 "txt" ——
// 老客户端存下来是个没有扩展名的文件，双击打不开。
// 所以先把扩展名摘出去，只对主干做过滤；主干里一个字母数字都不剩时
// 退回 "attachment"，拼回去是 attachment.txt，至少还能双击。
func asciiFallbackName(filename string) string {
	stem, ext := filename, ""
	if i := strings.LastIndexByte(filename, '.'); i > 0 && len(filename)-i <= 12 {
		stem, ext = filename[:i], asciiOnly(filename[i:])
	}
	safe := asciiOnly(stem)
	if !hasAlnum(safe) {
		safe = "attachment"
	}
	return safe + ext
}

// asciiOnly 只留下最保守的一小撮字符。别的一律换成下划线 ——
// 这一份要进 HTTP 头，而 HTTP 头是用换行分隔的：一个带 \r\n 的文件名
// 可以在这里劈开响应头。引号和反斜杠同理，能提前闭合 filename="..."。
func asciiOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func hasAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return true
		}
	}
	return false
}

func encodeRFC5987(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		// attr-char，RFC 5987 §3.2.1。别的一律百分号编码。
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

// safeFilename 把上传方给的名字收拾成一个能存、能显示的东西。
//
// **它不负责安全** —— 安全由「文件名根本不参与路径」保证（ADR-0011 第二条）。
// 这里做的是可用性：去掉目录部分（有些客户端会传完整路径）、去掉控制字符
// （它们在界面上是隐形的，能拿来伪装扩展名，比如 报告.txt<RLO>gpj.exe）、
// 限个长度。
func safeFilename(raw string) string {
	name := raw
	// Windows 客户端可能传 C:\Users\x\报告.txt
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r == unicode.ReplacementChar || unicode.IsControl(r) ||
			// 双向控制符：能让 "报告.txt" 在界面上看起来是别的扩展名
			(r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	// 按 rune 截断，不按字节 —— 按字节会把一个中文字符切成半个。
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	return name
}

// imageTypes / textTypes 是**回显白名单**。
//
// 不在名单里的一律 application/octet-stream —— 上传方声明的类型不直接回显。
// 名单本身也是有讲究的：
//   - 没有 image/svg+xml：SVG 是 XML，能带脚本。虽然 <img src> 里的 SVG
//     跑不了脚本、直接访问又被 Content-Disposition 挡成下载，但把它降级成
//     octet-stream 的代价只是「界面上不给它画缩略图」，太便宜了，不值得赌。
//   - 没有 text/html、没有 application/xhtml+xml，理由同上但更明显。
//   - text/* 一律强制 charset=utf-8：不带 charset 时浏览器会按自己的默认编码
//     猜，中文日志会花掉。
var imageTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true,
	"image/webp": true, "image/avif": true, "image/bmp": true,
}

var passthroughTypes = map[string]bool{
	"application/pdf":  true,
	"application/json": true, "application/zip": true,
	"application/gzip": true, "application/x-tar": true,
	"text/plain": true, "text/markdown": true, "text/csv": true,
}

// IsPreviewableImage 报告界面能不能直接把它画成缩略图。
// 契约里不体现，前端按 contentType 自己判 —— 这里导出只是给测试用。
func IsPreviewableImage(ct string) bool { return imageTypes[baseType(ct)] }

// normalizeContentType 决定下载时回显什么类型。
//
// 输入有两个来源，都不可信：multipart part 头里的 Content-Type（上传方随便写），
// 和文件扩展名（同样是上传方给的）。所以结论只能从白名单里出。
func normalizeContentType(declared, filename string) string {
	if ct := pickType(declared); ct != "" {
		return ct
	}
	// 声明的类型不认识，再按扩展名猜一次 —— 很多 CLI 上传工具压根不带
	// Content-Type，一律 octet-stream 的话连一张 png 都预览不了。
	if ct := pickType(mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func pickType(raw string) string {
	base := baseType(raw)
	switch {
	case imageTypes[base]:
		return base
	case passthroughTypes[base]:
		if strings.HasPrefix(base, "text/") {
			return base + "; charset=utf-8"
		}
		return base
	}
	return ""
}

func baseType(raw string) string {
	base, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(base)
}

// checkAttachmentCount 在进事务之前挡住「一条帖子挂几十个附件」。
//
// 返回零值 Error 表示通过 —— 这个签名让调用点是一行 if，
// 不用在四个 handler 里各写一遍同样的三行。
func (s *Server) checkAttachmentCount(ids []string) Error {
	max := s.cfg.AttachmentMaxPerPost
	if max <= 0 || len(ids) <= max {
		return Error{}
	}
	return Error{
		Code:      "too_many_attachments",
		Message:   "一条帖子最多挂 " + itoa(max) + " 个附件",
		Retryable: false,
	}
}
