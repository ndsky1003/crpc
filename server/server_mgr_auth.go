package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ndsky1003/crpc/v3/coder"
	"github.com/ndsky1003/crpc/v3/protocol"
	"github.com/ndsky1003/crpc/v3/protocol/errors"
	"github.com/ndsky1003/crpc/v3/protocol/header"
	"github.com/ndsky1003/crpc/v3/protocol/header/headercode"
	"github.com/ndsky1003/crpc/v3/protocol/header/headertype"
	"github.com/ndsky1003/net/v2/server"
)

// handleVerify 处理服务注册鉴权
func (s *server_mgr) handleVerify(sess server.Session, body []byte) error {
	secret := *s.opt.Secret

	var claim protocol.JwtClaims
	token, err := jwt.ParseWithClaims(string(body), &claim, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})

	if err != nil || !token.Valid {
		return errors.Newf(errors.ServerInternal, "jwt verify failed: %v", err)
	}

	var req protocol.VerifyReq
	if err := coder.Unmarshal(coder.Msgp, claim.Data, &req); err != nil {
		return errors.Newf(errors.ServerInternal, "unmarshal verify req failed: %v", err)
	}
	if req.UUID != uuid.Nil {
		sess.SetID(req.UUID)
	}
	s.connCache.Store(sess.ID(), sess)

	if oldGroupVal, loaded := s.sidGroupIndex.Load(sess.ID()); loaded {
		oldGroupName := oldGroupVal.(string)
		if oldGroupName != req.Name { // 如果名字变了，说明切换了身份
			if gVal, ok := s.services.Load(oldGroupName); ok {
				gVal.(*ServiceGroup).Remove(sess.ID().String())
			}
		}
	}
	// 获取或创建 ServiceGroup
	g, err := NewServiceGroup(req.Name, *s.opt.GroupReplicas)
	if err != nil {
		return errors.Newf(errors.ServerInternal, "create service group failed: %v", err)
	}

	val, _ := s.services.LoadOrStore(req.Name, g)
	group := val.(*ServiceGroup)

	group.Add(&Session{
		Name:    req.Name,
		Weight:  req.Weight,
		Session: sess,
	})

	s.sidGroupIndex.Store(sess.ID(), req.Name)

	slog.Info("Service Registered", "Name", req.Name, "sid", sess.ID(), "weight", req.Weight)

	return nil
}

// replyVerify 回复鉴权结果
func (s *server_mgr) replyVerify(sess server.Session, reqH *header.Header, verifyErr error) error {
	resp := &protocol.VerifyRes{
		Message: "OK",
	}
	reqH.Code = headercode.OK

	if verifyErr != nil {
		slog.Error("Authentication failed", "err", verifyErr, "sid", sess.ID())
		// 返回通用错误信息，避免泄露JWT验证的敏感信息
		resp.Message = "Authentication failed"
		reqH.Code = headercode.Failed
	} else {
		resp.UUID = sess.ID()
	}
	body, err := coder.Marshal(coder.Msgp, resp)
	if err != nil {
		return err
	}
	payload := protocol.JwtClaims{
		Data: body,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)), // 短期有效
		},
	}
	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString([]byte(*s.opt.Secret))
	if err != nil {
		return err
	}
	reqH.Type = headertype.Res
	packet, err := protocol.Pack(reqH, nil, []byte(tokenString))
	if err != nil {
		return err
	}
	return sess.Sends(context.Background(), packet)
}
