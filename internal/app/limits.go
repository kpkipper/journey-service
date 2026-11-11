package app

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/logger"
	"github.com/kpkipper/journey-service/pkg/utils"
)

// LoginEmailsPerIPPerHour caps login-link requests from one client IP. The per-email and
// total limits in AuthService still apply, so a spoofed or shared IP can't get past them.
// 20 leaves room for many users behind one mobile-carrier NAT address.
const LoginEmailsPerIPPerHour = 20

// DefaultClientIPHeader is the header Railway's edge sets to the caller's address.
const DefaultClientIPHeader = "X-Real-IP"

// clientIP returns the caller's address from header, else the first hop of
// X-Forwarded-For, else the socket peer (behind Railway that is the edge proxy, so every
// caller would share one bucket: logged once so a missing header is noticed).
// IPv6 addresses are cut to their /64, which one client can rotate within.
// Kept in memory: one Railway instance; a restart resets the counts, which the
// database-backed limits cover.
func clientIP(header string) func(*fiber.Ctx) string {
	var warnOnce sync.Once
	return func(c *fiber.Ctx) string {
		for _, h := range []string{header, fiber.HeaderXForwardedFor} {
			if h == "" {
				continue
			}
			v := strings.TrimSpace(strings.Split(c.Get(h), ",")[0])
			if ip := net.ParseIP(v); ip != nil {
				return ipKey(ip)
			}
		}
		warnOnce.Do(func() {
			logger.Get().Warn().Str("header", header).Msg("no client IP header on login email request; per-IP limit uses the connection address")
		})
		return c.IP()
	}
}

func ipKey(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func loginEmailLimiter(header string) fiber.Handler {
	key := clientIP(header)
	return limiter.New(limiter.Config{
		Max:          LoginEmailsPerIPPerHour,
		Expiration:   time.Hour,
		KeyGenerator: key,
		LimitReached: func(c *fiber.Ctx) error {
			logger.Get().Warn().Str("ip", key(c)).Msg("login email per-IP limit reached")
			return utils.Error(c, fiber.StatusTooManyRequests, services.ErrTooManyRequests.Error())
		},
	})
}
