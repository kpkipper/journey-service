package app

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPKey(t *testing.T) {
	assert.Equal(t, "203.0.113.7", ipKey(net.ParseIP("203.0.113.7")))
	assert.Equal(t, "203.0.113.7", ipKey(net.ParseIP("::ffff:203.0.113.7")))
	// One client rotating inside its /64 stays in one bucket.
	a := ipKey(net.ParseIP("2001:db8:1:2:aaaa::1"))
	assert.Equal(t, "2001:db8:1:2::/64", a)
	assert.Equal(t, a, ipKey(net.ParseIP("2001:db8:1:2:ffff:ffff:ffff:ffff")))
	assert.NotEqual(t, a, ipKey(net.ParseIP("2001:db8:1:3::1")))
}
