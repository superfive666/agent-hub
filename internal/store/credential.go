package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/superfive666/agent-hub/internal/domain"
)

var (
	// ErrTokenInvalid 覆盖「不存在 / 已用过 / 已作废 / 已过期」四种情况。
	// 对外一律返回同一个错误，不告诉调用方是哪一种 —— 那是在帮人枚举。
	ErrTokenInvalid = errors.New("注册 token 无效")
	// ErrCredentialInvalid 凭证不存在或已被吊销。
	ErrCredentialInvalid = errors.New("凭证无效")
)

// tokenPrefix 让人一眼看出这串东西是什么，方便在日志里识别并打码。
const (
	regTokenPrefix  = "ahr_reg_"
	credentialPre   = "ahr_cred_"
	tokenRandomBits = 32
)

// DefaultRegistrationTokenTTL 是注册 token 的默认有效期。
// 短有效期是它的安全性来源之一：它躺在剪贴板或聊天记录里的时间越短越好。
const DefaultRegistrationTokenTTL = 24 * time.Hour

func newSecret(prefix string) (string, []byte, error) {
	buf := make([]byte, tokenRandomBits)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("生成随机数: %w", err)
	}
	plain := prefix + base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(plain))
	return plain, sum[:], nil
}

func hashSecret(plain string) []byte {
	sum := sha256.Sum256([]byte(plain))
	return sum[:]
}

// IssueRegistrationToken 给某个 agent 签发一次性注册 token。
//
// **明文只在这里返回一次**，库里只留哈希 —— 关掉页面就再也看不到，只能作废重发。
// 注册 token 不是 API 凭证：它唯一的用途是换取长期凭证，换完立即作废。
func (s *Store) IssueRegistrationToken(ctx context.Context, agent domain.AgentID, ttl time.Duration) (plaintext string, expiresAt time.Time, err error) {
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		plaintext, expiresAt, err = issueRegistrationToken(ctx, tx, agent, ttl)
		return err
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return plaintext, expiresAt, nil
}

// issueRegistrationToken 是签发的事务内实现。
// 建 agent 时「顺手签一张」要和插 agent 行在同一个事务里，所以这一段必须能收 *sql.Tx。
func issueRegistrationToken(ctx context.Context, tx *sql.Tx, agent domain.AgentID, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = DefaultRegistrationTokenTTL
	}
	plain, hash, err := newSecret(regTokenPrefix)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(ttl)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO registration_token (id, agent_id, token_hash, expires_at)
		VALUES (gen_random_uuid(), $1, $2, $3)`, string(agent), hash, expiresAt); err != nil {
		return "", time.Time{}, fmt.Errorf("写注册 token: %w", err)
	}
	return plain, expiresAt, nil
}

// ExchangeRegistrationToken 用注册 token 换长期凭证。
//
// 一个事务里做三件事：校验 token 还能用、标记它用过、签发凭证。
// 同一个 token 被并发使用时，`used_at IS NULL` 的条件更新只会有一个成功。
func (s *Store) ExchangeRegistrationToken(ctx context.Context, plaintext string) (agent domain.AgentID, credential string, err error) {
	hash := hashSecret(plaintext)

	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var agentID string
		// 条件更新即认领：过期、用过、作废的都不会被选中。
		err := tx.QueryRowContext(ctx, `
			UPDATE registration_token
			SET used_at = now()
			WHERE token_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()
			RETURNING agent_id`, hash).Scan(&agentID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTokenInvalid
		}
		if err != nil {
			return fmt.Errorf("认领注册 token: %w", err)
		}

		plain, credHash, err := newSecret(credentialPre)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO agent_credential (id, agent_id, token_hash)
			VALUES (gen_random_uuid(), $1, $2)`, agentID, credHash); err != nil {
			return fmt.Errorf("签发长期凭证: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE agent SET status = 'active' WHERE id = $1`, agentID); err != nil {
			return fmt.Errorf("激活 agent: %w", err)
		}

		agent, credential = domain.AgentID(agentID), plain
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return agent, credential, nil
}

// AuthenticateCredential 用长期凭证换出 agent 身份。
//
// 查表而不是验签：吊销必须立即生效，无状态 JWT 做不到这一点。见技术选型 T9。
func (s *Store) AuthenticateCredential(ctx context.Context, plaintext string) (domain.AgentID, error) {
	if plaintext == "" {
		return "", ErrCredentialInvalid
	}
	hash := hashSecret(plaintext)

	var agentID string
	var stored []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT c.agent_id, c.token_hash
		FROM agent_credential c JOIN agent a ON a.id = c.agent_id
		WHERE c.token_hash = $1 AND c.revoked_at IS NULL AND a.status = 'active'`,
		hash).Scan(&agentID, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrCredentialInvalid
	}
	if err != nil {
		return "", fmt.Errorf("校验凭证: %w", err)
	}
	// 索引命中之后再做一次定时比较，避免任何形式的短路比较被观察到。
	if subtle.ConstantTimeCompare(stored, hash) != 1 {
		return "", ErrCredentialInvalid
	}
	return domain.AgentID(agentID), nil
}

// RevokeCredentials 吊销某个 agent 的全部长期凭证。吊销即刻生效。
func (s *Store) RevokeCredentials(ctx context.Context, agent domain.AgentID) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_credential SET revoked_at = now()
		WHERE agent_id = $1 AND revoked_at IS NULL`, string(agent))
	if err != nil {
		return fmt.Errorf("吊销凭证: %w", err)
	}
	return nil
}
