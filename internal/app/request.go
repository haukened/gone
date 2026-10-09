package app

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/haukened/gone/v3/internal/domain"
)

// ErrRequestsDisabled indicates the Service has no RequestStore configured.
var ErrRequestsDisabled = errors.New("secret requests are not configured")

// CreatedRequest is returned by Service.CreateRequest.
type CreatedRequest struct {
	ID          domain.SecretID    // new request identifier
	ExpiresAt   time.Time          // deadline for a reply
	ManageToken domain.ManageToken // requester's token for status, the reply, and cancel
	FillToken   domain.FillToken   // goes in the reply link; authorizes the reply
}

// CreateRequest opens a new secret request. The server learns only the TTL:
// the requester's public key and label never reach it. Only the SHA-256 of
// each token is stored.
//
// Parameters:
//   - ctx: request context.
//   - ttl: how long to wait for a reply, and how long a reply is then kept.
//
// Returns the new request's ID, expiry and tokens, or domain.ErrTTLInvalid,
// ErrRequestsDisabled, or a storage error.
func (s *Service) CreateRequest(ctx context.Context, ttl time.Duration) (CreatedRequest, error) {
	if s.Requests == nil {
		return CreatedRequest{}, ErrRequestsDisabled
	}
	if err := validateTTL(ttl, s.MinTTL, s.MaxTTL); err != nil {
		return CreatedRequest{}, domain.ErrTTLInvalid
	}
	id, err := domain.NewID()
	if err != nil {
		return CreatedRequest{}, err
	}
	manage, err := domain.NewManageToken()
	if err != nil {
		return CreatedRequest{}, err
	}
	fill, err := domain.NewFillToken()
	if err != nil {
		return CreatedRequest{}, err
	}
	expiresAt := s.Clock.Now().Add(ttl)
	if err = s.Requests.CreateRequest(ctx, id.String(), fill.Hash(), manage.Hash(), ttl, expiresAt); err != nil {
		return CreatedRequest{}, err
	}
	s.inc("requests_created_total")
	return CreatedRequest{ID: id, ExpiresAt: expiresAt, ManageToken: manage, FillToken: fill}, nil
}

// RequestOpen reports whether a request can still be answered, so the reply
// page can say so before anyone types a secret.
//
// Parameters:
//   - ctx: request context.
//   - idStr: request ID.
//   - fillStr: fill token from the reply link.
//
// Returns the reply deadline, or domain.ErrInvalidID, domain.ErrInvalidFill,
// ErrNotFound, ErrRequestsDisabled, or a storage error.
func (s *Service) RequestOpen(ctx context.Context, idStr, fillStr string) (time.Time, error) {
	fill, err := s.parseFill(idStr, fillStr)
	if err != nil {
		return time.Time{}, err
	}
	return s.Requests.RequestOpen(ctx, idStr, fill.Hash())
}

// Fill stores the single reply to an open request.
//
// Parameters:
//   - ctx: request context.
//   - idStr: request ID.
//   - fillStr: fill token from the reply link.
//   - ct: ciphertext reader.
//   - size: ciphertext size in bytes.
//   - version: protocol version; must be domain.ProtocolV3.
//   - nonce: base64url nonce used for encryption.
//
// Returns the reply's expiry, or domain.ErrInvalidID, domain.ErrInvalidFill,
// ErrSizeExceeded, domain.ErrInvalidVersion, domain.ErrInvalidNonce,
// ErrNotFound, ErrRequestsDisabled, or a storage error.
func (s *Service) Fill(ctx context.Context, idStr, fillStr string, ct io.Reader, size int64, version uint8, nonce string) (time.Time, error) {
	fill, err := s.parseFill(idStr, fillStr)
	if err != nil {
		return time.Time{}, err
	}
	if size <= 0 || size > s.MaxBytes {
		return time.Time{}, ErrSizeExceeded
	}
	if err = domain.ValidateReplyProtocol(version, nonce); err != nil {
		return time.Time{}, err
	}
	expires, err := s.Requests.Fill(ctx, idStr, fill.Hash(), Meta{Version: version, NonceB64u: nonce}, ct, size)
	if err != nil {
		return time.Time{}, err
	}
	s.inc("requests_filled_total")
	return expires, nil
}

// RequestStatus reports a request to its requester.
//
// Parameters:
//   - ctx: request context.
//   - idStr: request ID.
//   - tokenStr: manage token issued by CreateRequest.
//
// Returns the status, or domain.ErrInvalidID, domain.ErrInvalidManage,
// ErrNotFound, ErrRequestsDisabled, or a storage error.
func (s *Service) RequestStatus(ctx context.Context, idStr, tokenStr string) (RequestStatus, error) {
	tok, err := s.parseRequestManage(idStr, tokenStr)
	if err != nil {
		return RequestStatus{}, err
	}
	return s.Requests.RequestStatus(ctx, idStr, tok.Hash())
}

// ClaimReply reserves a ready reply for the requester, exactly like Claim but
// authorized by the manage token.
//
// Parameters:
//   - ctx: request context.
//   - idStr: request ID.
//   - manageStr: manage token issued by CreateRequest.
//   - claimStr: existing claim token for a retry, or "" for a fresh claim.
//
// Returns the claimed ciphertext with its token, or domain.ErrInvalidID,
// domain.ErrInvalidManage, domain.ErrInvalidClaim, ErrNotFound,
// ErrRequestsDisabled, or a storage error.
func (s *Service) ClaimReply(ctx context.Context, idStr, manageStr, claimStr string) (ClaimResult, error) {
	manage, err := s.parseRequestManage(idStr, manageStr)
	if err != nil {
		return ClaimResult{}, err
	}
	tok, retry, err := claimToken(claimStr)
	if err != nil {
		return ClaimResult{}, err
	}
	until := s.Clock.Now().Add(s.claimLease())
	c, err := s.Requests.ClaimReply(ctx, idStr, manage.Hash(), tok.Hash(), retry, until)
	if err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{Claimed: c, Token: tok}, nil
}

// AckReply confirms the requester opened the reply, permanently deleting it.
//
// Parameters:
//   - ctx: request context.
//   - idStr: request ID.
//   - claimStr: claim token issued by ClaimReply.
//
// Returns nil on deletion, or domain.ErrInvalidID, domain.ErrInvalidClaim,
// ErrNotFound, or a storage error.
func (s *Service) AckReply(ctx context.Context, idStr, claimStr string) error {
	if _, err := domain.ParseID(idStr); err != nil {
		return domain.ErrInvalidID
	}
	tok, err := domain.ParseClaimToken(claimStr)
	if err != nil {
		return err
	}
	if err = s.Store.Ack(ctx, idStr, tok.Hash()); err != nil {
		return err
	}
	s.inc("requests_opened_total")
	return nil
}

// CancelRequest deletes an open request or an unopened reply for the
// requester.
//
// Parameters:
//   - ctx: request context.
//   - idStr: request ID.
//   - tokenStr: manage token issued by CreateRequest.
//
// Returns nil on deletion, or domain.ErrInvalidID, domain.ErrInvalidManage,
// ErrNotFound, ErrRequestsDisabled, or a storage error.
func (s *Service) CancelRequest(ctx context.Context, idStr, tokenStr string) error {
	tok, err := s.parseRequestManage(idStr, tokenStr)
	if err != nil {
		return err
	}
	if err = s.Requests.CancelRequest(ctx, idStr, tok.Hash()); err != nil {
		return err
	}
	s.inc("requests_cancelled_total")
	return nil
}

// parseFill validates a request ID and fill token and that requests are
// configured.
//
// Parameters:
//   - idStr: request ID.
//   - fillStr: fill token.
//
// Returns the parsed token, or ErrRequestsDisabled, domain.ErrInvalidID or
// domain.ErrInvalidFill.
func (s *Service) parseFill(idStr, fillStr string) (domain.FillToken, error) {
	if s.Requests == nil {
		return "", ErrRequestsDisabled
	}
	if _, err := domain.ParseID(idStr); err != nil {
		return "", domain.ErrInvalidID
	}
	return domain.ParseFillToken(fillStr)
}

// parseRequestManage validates a request ID and manage token and that
// requests are configured.
//
// Parameters:
//   - idStr: request ID.
//   - tokenStr: manage token.
//
// Returns the parsed token, or ErrRequestsDisabled, domain.ErrInvalidID or
// domain.ErrInvalidManage.
func (s *Service) parseRequestManage(idStr, tokenStr string) (domain.ManageToken, error) {
	if s.Requests == nil {
		return "", ErrRequestsDisabled
	}
	return parseManage(idStr, tokenStr)
}

// claimToken parses an existing claim token for a retry, or issues a new one
// when claimStr is empty.
//
// Parameters:
//   - claimStr: presented claim token, or "".
//
// Returns the token, whether it is a retry, or domain.ErrInvalidClaim or an
// RNG error.
func claimToken(claimStr string) (domain.ClaimToken, bool, error) {
	if claimStr == "" {
		tok, err := domain.NewClaimToken()
		return tok, false, err
	}
	tok, err := domain.ParseClaimToken(claimStr)
	return tok, true, err
}
