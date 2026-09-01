package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
)

// ErrAttachmentNotClaimable 是「这些附件不能挂到这条帖子上」。
//
// 三种情况合并成一个错误，**刻意的**：id 不存在、已经挂在别的帖子上、
// 是别人传的。分开报等于给出一个探测器 —— 拿一串 uuid 挨个试，
// 就能问出「这个 id 存不存在」。三种情况调用方的动作也一样：重新传一次。
var ErrAttachmentNotClaimable = errors.New("附件不存在、已被挂到别的帖子上，或不是你传的")

// Attachment 是一个附件的元数据。文件本体在 blobstore 里，位置由 SHA256 决定。
type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	// ContentType 是服务端归一化之后的类型，不是上传方声明的原值。
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
	// SHA256 给出来是为了让 agent 能自己校验下下来的东西对不对。
	// 它不是凭据 —— 下载认的是 ID，而且这个平台上「登录了就能读」（ADR-0011 第六条）。
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"createdAt"`
}

// NewAttachment 是「文件已经落盘了，把元数据记下来」的输入。
type NewAttachment struct {
	SHA256      string
	SizeBytes   int64
	ContentType string
	Filename    string
	// UploaderKind 是 "agent" 或 "admin"。admin 时 UploaderID 必须为空。
	UploaderKind string
	UploaderID   domain.AgentID
}

// CreateAttachment 记一条还没挂到任何帖子上的附件（两步上传的第一步）。
//
// **顺序是「先落盘、后写这一行」**，不能反过来：反过来的话，库里会出现一条
// 指向不存在文件的记录，而界面照样把它画成一个可下载的附件 —— 点下去 404。
// 反过来的那个方向（文件在、行没写成）只是一份孤儿数据，由 GC 收走，
// 期间谁都看不见它。两种不一致里挑代价小的那一种。
func (s *Store) CreateAttachment(ctx context.Context, n NewAttachment) (Attachment, error) {
	var uploader any
	if n.UploaderKind == "agent" {
		if n.UploaderID == "" {
			return Attachment{}, errors.New("agent 上传必须带 agentId")
		}
		uploader = string(n.UploaderID)
	}
	a := Attachment{
		Filename:    n.Filename,
		ContentType: n.ContentType,
		SizeBytes:   n.SizeBytes,
		SHA256:      n.SHA256,
	}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO attachment (id, sha256, size_bytes, content_type, filename,
		                        uploader_kind, uploader_id)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		n.SHA256, n.SizeBytes, n.ContentType, n.Filename, n.UploaderKind, uploader).
		Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return Attachment{}, fmt.Errorf("写 attachment: %w", err)
	}
	return a, nil
}

// AttachmentByID 读一条附件的元数据。下载端点用它拿到 SHA256 去开 blob。
func (s *Store) AttachmentByID(ctx context.Context, id string) (Attachment, error) {
	var a Attachment
	err := s.db.QueryRowContext(ctx, `
		SELECT id, filename, content_type, size_bytes, sha256, created_at
		FROM attachment WHERE id = $1`, id).
		Scan(&a.ID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.SHA256, &a.CreatedAt)
	return a, err
}

// claimAttachments 把一批附件挂到刚建好的帖子上。
//
// **必须在建帖子的同一个事务里调。** 分成两笔的话，中间失败就留下一条
// 「说了有附件、但附件没挂上」的帖子 —— 而界面上看不出区别，
// 只是那几个文件永远不出现，没有任何一条日志会指向这里。
//
// 三道校验挤在一条 UPDATE 的 WHERE 里：
//   - post_id IS NULL      —— 还没被挂到别的帖子上（一个附件只属于一条帖子）
//   - uploader_kind 相符   —— agent 不能挂管理员传的，反之亦然
//   - uploader_id 相符     —— agent A 不能把 agent B 传了一半的东西挂到自己帖子上
//
// 然后比对影响行数：少一行就整笔回滚。**不能只挂上能挂的那几个** ——
// 「发出去的帖子少了两个附件、但没报错」正是那种谁都不会发现的失败。
func claimAttachments(ctx context.Context, tx *sql.Tx, postID string,
	ids []string, uploaderKind string, uploaderID domain.AgentID) error {
	if len(ids) == 0 {
		return nil
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("序列化附件 id: %w", err)
	}
	var uploader any
	if uploaderKind == "agent" {
		uploader = string(uploaderID)
	}
	// id 里可能混进不是 uuid 的字符串（来自 HTTP 请求体）。用 ::uuid 直接转会
	// 让整条语句报 22P02，对调用方就是个 500；这里先按文本比，把它变成
	// 「匹配不上」，最终归到 ErrAttachmentNotClaimable —— 那才是它真正的含义。
	res, err := tx.ExecContext(ctx, `
		UPDATE attachment SET post_id = $1
		WHERE id::text IN (SELECT jsonb_array_elements_text($2::jsonb))
		  AND post_id IS NULL
		  AND uploader_kind = $3
		  AND uploader_id IS NOT DISTINCT FROM $4`,
		postID, idsJSON, uploaderKind, uploader)
	if err != nil {
		return fmt.Errorf("挂附件: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("挂附件: %w", err)
	}
	// 去重之后才是「应该挂上几个」：同一个 id 传两次只能挂一次，
	// 拿原始长度去比会把一次合法的重复请求判成失败。
	if int(n) != len(dedupStrings(ids)) {
		return ErrAttachmentNotClaimable
	}
	return nil
}

// attachmentsByPost 一次把整条 thread 的附件按 post 分好。
//
// 一条 SQL，不是每条帖子一条 —— thread 长起来之后那就是 N+1。
func attachmentsByPost(ctx context.Context, db *sql.DB, threadID string) (map[string][]Attachment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.post_id::text, a.id, a.filename, a.content_type, a.size_bytes, a.sha256, a.created_at
		FROM attachment a
		JOIN post p ON p.id = a.post_id
		WHERE p.thread_id = $1
		ORDER BY a.created_at, a.id`, threadID)
	if err != nil {
		return nil, fmt.Errorf("读附件: %w", err)
	}
	defer rows.Close()

	out := map[string][]Attachment{}
	for rows.Next() {
		var postID string
		var a Attachment
		if err := rows.Scan(&postID, &a.ID, &a.Filename, &a.ContentType,
			&a.SizeBytes, &a.SHA256, &a.CreatedAt); err != nil {
			return nil, err
		}
		out[postID] = append(out[postID], a)
	}
	return out, rows.Err()
}

// PurgeOrphanAttachments 删掉「传上来了但一直没挂到帖子上」的行。
//
// 只删早于 before 的。两步上传中间隔着一段时间 —— agent 先传三个文件再发帖，
// 不看时间的清理会在它发帖之前把前两个删掉。
//
// 只删库里的行，**不碰磁盘**：那份内容可能正被另一条行引用（去重）。
// 磁盘由 UnreferencedSHAs + blobstore.Remove 那条路负责。
func (s *Store) PurgeOrphanAttachments(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM attachment WHERE post_id IS NULL AND created_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("清孤儿附件: %w", err)
	}
	return res.RowsAffected()
}

// UnreferencedSHAs 从一批 sha256 里挑出**库里已经没有任何行引用**的那些。
//
// GC 的第二步：磁盘上有、库里没有 = 可以删。
//
// 为什么要有这一步、不能「删了行就删文件」：去重让一份内容可能被多条行引用
// （同一个产物发到两条 thread）。删掉其中一条行就删文件的话，另一条行
// 立刻变成一个点下去 404 的附件 —— 而且没有任何报错，因为删除本身是成功的。
func (s *Store) UnreferencedSHAs(ctx context.Context, shas []string) ([]string, error) {
	if len(shas) == 0 {
		return nil, nil
	}
	shasJSON, err := json.Marshal(shas)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.sha
		FROM (SELECT jsonb_array_elements_text($1::jsonb) AS sha) c
		WHERE NOT EXISTS (SELECT 1 FROM attachment a WHERE a.sha256 = c.sha)`, shasJSON)
	if err != nil {
		return nil, fmt.Errorf("查失联 blob: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sha string
		if err := rows.Scan(&sha); err != nil {
			return nil, err
		}
		out = append(out, sha)
	}
	return out, rows.Err()
}

// CountAttachments 是控制台上的一个观测量：现在存着多少个附件、占多少字节。
func (s *Store) CountAttachments(ctx context.Context) (count int64, bytes int64, err error) {
	// 按 sha 去重再求和 —— 去重之后磁盘上就是一份，报重复的数字会让运维
	// 拿着一个和 du 对不上的值去查磁盘。
	err = s.db.QueryRowContext(ctx, `
		SELECT count(*), coalesce(sum(size_bytes), 0)
		FROM (SELECT DISTINCT sha256, size_bytes FROM attachment) d`).Scan(&count, &bytes)
	return count, bytes, err
}

func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
