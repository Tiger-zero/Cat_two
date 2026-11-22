package store

import (
	"context"
	"math"
	"net/netip"
	"time"

	"github.com/pkg/errors"
)

// BlockedLoginIP is a permanent sign-in block for a canonical IP address.
type BlockedLoginIP struct {
	IP           string
	BlockedAt    time.Time
	FailureCount int
}

// IsLoginIPBlocked reports whether ip has reached the sign-in failure limit.
func (s *Store) IsLoginIPBlocked(ctx context.Context, ip string) (bool, error) {
	address, err := canonicalLoginIP(ip)
	if err != nil {
		return false, err
	}
	return s.driver.IsLoginIPBlocked(ctx, address)
}

// RecordLoginIPFailure increments the consecutive failures for ip and blocks
// sign-in when the limit is reached. An existing block keeps its count and time.
func (s *Store) RecordLoginIPFailure(ctx context.Context, ip string, limit int) (int, bool, error) {
	address, err := canonicalLoginIP(ip)
	if err != nil {
		return 0, false, err
	}
	if limit < 1 || limit > math.MaxInt32 {
		return 0, false, errors.New("invalid login failure limit")
	}
	return s.driver.RecordLoginIPFailure(ctx, address, limit, time.Now().Unix())
}

// ClearLoginIPFailures resets an IP's unblocked failure series after sign-in.
// It reports whether the address is already blocked, including a concurrent block.
func (s *Store) ClearLoginIPFailures(ctx context.Context, ip string) (bool, error) {
	address, err := canonicalLoginIP(ip)
	if err != nil {
		return false, err
	}
	return s.driver.ClearLoginIPFailures(ctx, address)
}

// ListBlockedLoginIPs returns blocked addresses in ascending canonical IP order.
// Pass the last address of the preceding page, or an empty address for the first page.
func (s *Store) ListBlockedLoginIPs(ctx context.Context, afterIP string, limit int) ([]*BlockedLoginIP, error) {
	if limit < 1 {
		return nil, errors.New("invalid blocked login IP page limit")
	}
	if afterIP != "" {
		address, err := canonicalLoginIP(afterIP)
		if err != nil {
			return nil, err
		}
		afterIP = address
	}
	return s.driver.ListBlockedLoginIPs(ctx, afterIP, limit)
}

// UnblockLoginIP removes the permanent block and its failure count.
// It reports whether the address was blocked before removal.
func (s *Store) UnblockLoginIP(ctx context.Context, ip string) (bool, error) {
	address, err := canonicalLoginIP(ip)
	if err != nil {
		return false, err
	}
	return s.driver.UnblockLoginIP(ctx, address)
}

func canonicalLoginIP(ip string) (string, error) {
	address, err := netip.ParseAddr(ip)
	if err != nil {
		return "", errors.New("invalid login IP address")
	}
	return address.WithZone("").Unmap().String(), nil
}
