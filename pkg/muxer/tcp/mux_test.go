package tcp

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v2/pkg/tcp"
)

type fakeConn struct {
	call       map[string]int
	remoteAddr net.Addr
}

func (f *fakeConn) Read(b []byte) (n int, err error) {
	panic("implement me")
}

func (f *fakeConn) Write(b []byte) (n int, err error) {
	f.call[string(b)]++
	return len(b), nil
}

func (f *fakeConn) Close() error {
	panic("implement me")
}

func (f *fakeConn) LocalAddr() net.Addr {
	panic("implement me")
}

func (f *fakeConn) RemoteAddr() net.Addr {
	return f.remoteAddr
}

func (f *fakeConn) SetDeadline(t time.Time) error {
	panic("implement me")
}

func (f *fakeConn) SetReadDeadline(t time.Time) error {
	panic("implement me")
}

func (f *fakeConn) SetWriteDeadline(t time.Time) error {
	panic("implement me")
}

func (f *fakeConn) CloseWrite() error {
	panic("implement me")
}

func Test_addTCPRoute(t *testing.T) {
	testCases := []struct {
		desc       string
		rule       string
		serverName string
		remoteAddr string
		routeErr   bool
		matchErr   bool
	}{
		{
			desc:     "no tree",
			routeErr: true,
		},
		{
			desc:     "Rule with no matcher",
			rule:     "rulewithnotmatcher",
			routeErr: true,
		},
		{
			desc:       "Empty HostSNI rule",
			rule:       "HostSNI()",
			serverName: "foobar",
			routeErr:   true,
		},
		{
			desc:       "Empty HostSNI rule",
			rule:       "HostSNI(``)",
			serverName: "foobar",
			routeErr:   true,
		},
		{
			desc:       "Valid HostSNI rule matching",
			rule:       "HostSNI(`foobar`)",
			serverName: "foobar",
		},
		{
			desc:       "Valid negative HostSNI rule matching",
			rule:       "!HostSNI(`bar`)",
			serverName: "foobar",
		},
		{
			desc:       "Valid HostSNI rule matching with alternative case",
			rule:       "hostsni(`foobar`)",
			serverName: "foobar",
		},
		{
			desc:       "Valid HostSNI rule matching with alternative case",
			rule:       "HOSTSNI(`foobar`)",
			serverName: "foobar",
		},
		{
			desc:       "Valid HostSNI rule not matching",
			rule:       "HostSNI(`foobar`)",
			serverName: "bar",
			matchErr:   true,
		},
		{
			desc:       "Valid negative HostSNI rule not matching",
			rule:       "!HostSNI(`bar`)",
			serverName: "bar",
			matchErr:   true,
		},
		{
			desc:     "Empty ClientIP rule",
			rule:     "ClientIP()",
			routeErr: true,
		},
		{
			desc:     "Empty ClientIP rule",
			rule:     "ClientIP(``)",
			routeErr: true,
		},
		{
			desc:     "Invalid ClientIP",
			rule:     "ClientIP(`invalid`)",
			routeErr: true,
		},
		{
			desc:       "Valid ClientIP rule matching",
			rule:       "ClientIP(`10.0.0.1`)",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid negative ClientIP rule matching",
			rule:       "!ClientIP(`20.0.0.1`)",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid ClientIP rule matching with alternative case",
			rule:       "clientip(`10.0.0.1`)",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid ClientIP rule matching with alternative case",
			rule:       "CLIENTIP(`10.0.0.1`)",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid ClientIP rule not matching",
			rule:       "ClientIP(`10.0.0.1`)",
			remoteAddr: "10.0.0.2:80",
			matchErr:   true,
		},
		{
			desc:       "Valid negative ClientIP rule not matching",
			rule:       "!ClientIP(`10.0.0.2`)",
			remoteAddr: "10.0.0.2:80",
			matchErr:   true,
		},
		{
			desc:       "Valid ClientIP rule matching IPv6",
			rule:       "ClientIP(`10::10`)",
			remoteAddr: "[10::10]:80",
		},
		{
			desc:       "Valid ClientIP rule not matching IPv6",
			rule:       "ClientIP(`10::10`)",
			remoteAddr: "[::1]:80",
			matchErr:   true,
		},
		{
			desc:       "Valid ClientIP rule matching multiple IPs",
			rule:       "ClientIP(`10.0.0.1`, `10.0.0.0`)",
			remoteAddr: "10.0.0.0:80",
		},
		{
			desc:       "Valid ClientIP rule matching CIDR",
			rule:       "ClientIP(`11.0.0.0/24`)",
			remoteAddr: "11.0.0.0:80",
		},
		{
			desc:       "Valid ClientIP rule not matching CIDR",
			rule:       "ClientIP(`11.0.0.0/24`)",
			remoteAddr: "10.0.0.0:80",
			matchErr:   true,
		},
		{
			desc:       "Valid ClientIP rule matching CIDR IPv6",
			rule:       "ClientIP(`11::/16`)",
			remoteAddr: "[11::]:80",
		},
		{
			desc:       "Valid ClientIP rule not matching CIDR IPv6",
			rule:       "ClientIP(`11::/16`)",
			remoteAddr: "[10::]:80",
			matchErr:   true,
		},
		{
			desc:       "Valid ClientIP rule matching multiple CIDR",
			rule:       "ClientIP(`11.0.0.0/16`, `10.0.0.0/16`)",
			remoteAddr: "10.0.0.0:80",
		},
		{
			desc:       "Valid ClientIP rule not matching CIDR and matching IP",
			rule:       "ClientIP(`11.0.0.0/16`, `10.0.0.0`)",
			remoteAddr: "10.0.0.0:80",
		},
		{
			desc:       "Valid ClientIP rule matching CIDR and not matching IP",
			rule:       "ClientIP(`11.0.0.0`, `10.0.0.0/16`)",
			remoteAddr: "10.0.0.0:80",
		},
		{
			desc:       "Valid HostSNI and ClientIP rule matching",
			rule:       "HostSNI(`foobar`) && ClientIP(`10.0.0.1`)",
			serverName: "foobar",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid negative HostSNI and ClientIP rule matching",
			rule:       "!HostSNI(`bar`) && ClientIP(`10.0.0.1`)",
			serverName: "foobar",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid HostSNI and negative ClientIP rule matching",
			rule:       "HostSNI(`foobar`) && !ClientIP(`10.0.0.2`)",
			serverName: "foobar",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid negative HostSNI and negative ClientIP rule matching",
			rule:       "!HostSNI(`bar`) && !ClientIP(`10.0.0.2`)",
			serverName: "foobar",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid HostSNI and ClientIP rule not matching",
			rule:       "HostSNI(`foobar`) && ClientIP(`10.0.0.1`)",
			serverName: "bar",
			remoteAddr: "10.0.0.1:80",
			matchErr:   true,
		},
		{
			desc:       "Valid HostSNI and ClientIP rule not matching",
			rule:       "HostSNI(`foobar`) && ClientIP(`10.0.0.1`)",
			serverName: "foobar",
			remoteAddr: "10.0.0.2:80",
			matchErr:   true,
		},
		{
			desc:       "Valid HostSNI or ClientIP rule matching",
			rule:       "HostSNI(`foobar`) || ClientIP(`10.0.0.1`)",
			serverName: "foobar",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid HostSNI or ClientIP rule matching",
			rule:       "HostSNI(`foobar`) || ClientIP(`10.0.0.1`)",
			serverName: "bar",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid HostSNI or ClientIP rule matching",
			rule:       "HostSNI(`foobar`) || ClientIP(`10.0.0.1`)",
			serverName: "foobar",
			remoteAddr: "10.0.0.2:80",
		},
		{
			desc:       "Valid HostSNI or ClientIP rule not matching",
			rule:       "HostSNI(`foobar`) || ClientIP(`10.0.0.1`)",
			serverName: "bar",
			remoteAddr: "10.0.0.2:80",
			matchErr:   true,
		},
		{
			desc:       "Valid HostSNI x 3 OR rule matching",
			rule:       "HostSNI(`foobar`) || HostSNI(`foo`) || HostSNI(`bar`)",
			serverName: "foobar",
		},
		{
			desc:       "Valid HostSNI x 3 OR rule not matching",
			rule:       "HostSNI(`foobar`) || HostSNI(`foo`) || HostSNI(`bar`)",
			serverName: "baz",
			matchErr:   true,
		},
		{
			desc:       "Valid HostSNI and ClientIP Combined rule matching",
			rule:       "HostSNI(`foobar`) || HostSNI(`bar`) && ClientIP(`10.0.0.1`)",
			serverName: "foobar",
			remoteAddr: "10.0.0.2:80",
		},
		{
			desc:       "Valid HostSNI and ClientIP Combined rule matching",
			rule:       "HostSNI(`foobar`) || HostSNI(`bar`) && ClientIP(`10.0.0.1`)",
			serverName: "bar",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid HostSNI and ClientIP Combined rule not matching",
			rule:       "HostSNI(`foobar`) || HostSNI(`bar`) && ClientIP(`10.0.0.1`)",
			serverName: "bar",
			remoteAddr: "10.0.0.2:80",
			matchErr:   true,
		},
		{
			desc:       "Valid HostSNI and ClientIP Combined rule not matching",
			rule:       "HostSNI(`foobar`) || HostSNI(`bar`) && ClientIP(`10.0.0.1`)",
			serverName: "baz",
			remoteAddr: "10.0.0.1:80",
			matchErr:   true,
		},
		{
			desc:       "Valid HostSNI and ClientIP complex combined rule matching",
			rule:       "(HostSNI(`foobar`) || HostSNI(`bar`)) && (ClientIP(`10.0.0.1`) || ClientIP(`10.0.0.2`))",
			serverName: "bar",
			remoteAddr: "10.0.0.1:80",
		},
		{
			desc:       "Valid HostSNI and ClientIP complex combined rule not matching",
			rule:       "(HostSNI(`foobar`) || HostSNI(`bar`)) && (ClientIP(`10.0.0.1`) || ClientIP(`10.0.0.2`))",
			serverName: "baz",
			remoteAddr: "10.0.0.1:80",
			matchErr:   true,
		},
		{
			desc:       "Valid HostSNI and ClientIP complex combined rule not matching",
			rule:       "(HostSNI(`foobar`) || HostSNI(`bar`)) && (ClientIP(`10.0.0.1`) || ClientIP(`10.0.0.2`))",
			serverName: "bar",
			remoteAddr: "10.0.0.3:80",
			matchErr:   true,
		},
		{
			desc:       "Valid HostSNI and ClientIP more complex (but absurd) combined rule matching",
			rule:       "(HostSNI(`foobar`) || (HostSNI(`bar`) && !HostSNI(`foobar`))) && ((ClientIP(`10.0.0.1`) && !ClientIP(`10.0.0.2`)) || ClientIP(`10.0.0.2`)) ",
			serverName: "bar",
			remoteAddr: "10.0.0.1:80",
		},
	}

	for _, test := range testCases {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			msg := "BYTES"
			handler := tcp.HandlerFunc(func(conn tcp.WriteCloser) {
				_, err := conn.Write([]byte(msg))
				require.NoError(t, err)
			})
			router, err := NewMuxer()
			require.NoError(t, err)

			err = router.AddRoute(test.rule, 0, handler)
			if test.routeErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				addr := "0.0.0.0:0"
				if test.remoteAddr != "" {
					addr = test.remoteAddr
				}

				conn := &fakeConn{
					call:       map[string]int{},
					remoteAddr: fakeAddr{addr: addr},
				}

				connData, err := NewConnData(test.serverName, conn)
				require.NoError(t, err)

				handler := router.Match(connData)
				if test.matchErr {
					require.Nil(t, handler)
					return
				}

				require.NotNil(t, handler)

				handler.ServeTCP(conn)

				n, ok := conn.call[msg]
				assert.Equal(t, n, 1)
				assert.True(t, ok)
			}
		})
	}
}

type fakeAddr struct {
	addr string
}

func (f fakeAddr) String() string {
	return f.addr
}

func (f fakeAddr) Network() string {
	panic("Implement me")
}

func TestParseHostSNI(t *testing.T) {
	testCases := []struct {
		description   string
		expression    string
		domain        []string
		errorExpected bool
	}{
		{
			description:   "Many hostSNI rules",
			expression:    "HostSNI(`foo.bar`,`test.bar`)",
			domain:        []string{"foo.bar", "test.bar"},
			errorExpected: false,
		},
		{
			description:   "Many hostSNI rules upper",
			expression:    "HOSTSNI(`foo.bar`,`test.bar`)",
			domain:        []string{"foo.bar", "test.bar"},
			errorExpected: false,
		},
		{
			description:   "Many hostSNI rules lower",
			expression:    "hostsni(`foo.bar`,`test.bar`)",
			domain:        []string{"foo.bar", "test.bar"},
			errorExpected: false,
		},
		{
			description:   "No hostSNI rule",
			expression:    "ClientIP(`10.1`)",
			errorExpected: false,
		},
		{
			description:   "HostSNI rule and another rule",
			expression:    "HostSNI(`foo.bar`) && ClientIP(`10.1`)",
			domain:        []string{"foo.bar"},
			errorExpected: false,
		},
		{
			description:   "HostSNI rule to lower and another rule",
			expression:    "HostSNI(`Foo.Bar`) && ClientIP(`10.1`)",
			domain:        []string{"foo.bar"},
			errorExpected: false,
		},
		{
			description:   "HostSNI rule with no domain",
			expression:    "HostSNI() && ClientIP(`10.1`)",
			errorExpected: false,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.expression, func(t *testing.T) {
			t.Parallel()

			domains, err := ParseHostSNI(test.expression)

			if test.errorExpected {
				require.Errorf(t, err, "unable to parse correctly the domains in the HostSNI rule from %q", test.expression)
			} else {
				require.NoError(t, err, "%s: Error while parsing domain.", test.expression)
			}

			assert.EqualValues(t, test.domain, domains, "%s: Error parsing domains from expression.", test.expression)
		})
	}
}

func Test_HostSNI(t *testing.T) {
	testCases := []struct {
		desc       string
		ruleHosts  []string
		serverName string
		buildErr   bool
		matchErr   bool
	}{
		{
			desc:     "Empty",
			buildErr: true,
		},
		{
			desc:      "Non ASCII host",
			ruleHosts: []string{"héhé"},
			buildErr:  true,
		},
		{
			desc:       "Not Matching hosts",
			ruleHosts:  []string{"foobar"},
			serverName: "bar",
			matchErr:   true,
		},
		{
			desc:       "Matching globing host `*`",
			ruleHosts:  []string{"*"},
			serverName: "foobar",
		},
		{
			desc:       "Matching globing host `*` and empty serverName",
			ruleHosts:  []string{"*"},
			serverName: "",
		},
		{
			desc:       "Matching globing host `*` and another non matching host",
			ruleHosts:  []string{"foo", "*"},
			serverName: "bar",
		},
		{
			desc:       "Matching globing host `*` and another non matching host, and empty servername",
			ruleHosts:  []string{"foo", "*"},
			serverName: "",
			matchErr:   true,
		},
		{
			desc:      "Not Matching globing host with subdomain",
			ruleHosts: []string{"*.bar"},
			buildErr:  true,
		},
		{
			desc:       "Not Matching host with trailing dot with ",
			ruleHosts:  []string{"foobar."},
			serverName: "foobar.",
		},
		{
			desc:       "Matching host with trailing dot",
			ruleHosts:  []string{"foobar."},
			serverName: "foobar",
		},
		{
			desc:       "Matching hosts",
			ruleHosts:  []string{"foobar"},
			serverName: "foobar",
		},
		{
			desc:       "Matching hosts with subdomains",
			ruleHosts:  []string{"foo.bar"},
			serverName: "foo.bar",
		},
	}

	for _, test := range testCases {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			matcherTree := &matchersTree{}
			err := hostSNI(matcherTree, test.ruleHosts...)
			if test.buildErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			meta := ConnData{
				serverName: test.serverName,
			}

			if test.matchErr {
				assert.False(t, matcherTree.match(meta))
			} else {
				assert.True(t, matcherTree.match(meta))
			}
		})
	}
}

func Test_ClientIP(t *testing.T) {
	testCases := []struct {
		desc      string
		ruleCIDRs []string
		remoteIP  string
		buildErr  bool
		matchErr  bool
	}{
		{
			desc:     "Empty",
			buildErr: true,
		},
		{
			desc:      "Malformed CIDR",
			ruleCIDRs: []string{"héhé"},
			buildErr:  true,
		},
		{
			desc:      "Not matching empty remote IP",
			ruleCIDRs: []string{"20.20.20.20"},
			matchErr:  true,
		},
		{
			desc:      "Not matching IP",
			ruleCIDRs: []string{"20.20.20.20"},
			remoteIP:  "10.10.10.10",
			matchErr:  true,
		},
		{
			desc:      "Matching IP",
			ruleCIDRs: []string{"10.10.10.10"},
			remoteIP:  "10.10.10.10",
		},
		{
			desc:      "Not matching multiple IPs",
			ruleCIDRs: []string{"20.20.20.20", "30.30.30.30"},
			remoteIP:  "10.10.10.10",
			matchErr:  true,
		},
		{
			desc:      "Matching multiple IPs",
			ruleCIDRs: []string{"10.10.10.10", "20.20.20.20", "30.30.30.30"},
			remoteIP:  "20.20.20.20",
		},
		{
			desc:      "Not matching CIDR",
			ruleCIDRs: []string{"20.0.0.0/24"},
			remoteIP:  "10.10.10.10",
			matchErr:  true,
		},
		{
			desc:      "Matching CIDR",
			ruleCIDRs: []string{"20.0.0.0/8"},
			remoteIP:  "20.10.10.10",
		},
		{
			desc:      "Not matching multiple CIDRs",
			ruleCIDRs: []string{"10.0.0.0/24", "20.0.0.0/24"},
			remoteIP:  "10.10.10.10",
			matchErr:  true,
		},
		{
			desc:      "Matching multiple CIDRs",
			ruleCIDRs: []string{"10.0.0.0/8", "20.0.0.0/8"},
			remoteIP:  "20.10.10.10",
		},
	}

	for _, test := range testCases {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			matchersTree := &matchersTree{}
			err := clientIP(matchersTree, test.ruleCIDRs...)
			if test.buildErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			meta := ConnData{
				remoteIP: test.remoteIP,
			}

			if test.matchErr {
				assert.False(t, matchersTree.match(meta))
			} else {
				assert.True(t, matchersTree.match(meta))
			}
		})
	}
}

func Test_Priority(t *testing.T) {
	testCases := []struct {
		desc         string
		rules        map[string]int
		serverName   string
		remoteIP     string
		expectedRule string
	}{
		{
			desc: "One matching rule, calculated priority",
			rules: map[string]int{
				"HostSNI(`bar`)":    0,
				"HostSNI(`foobar`)": 0,
			},
			expectedRule: "HostSNI(`bar`)",
			serverName:   "bar",
		},
		{
			desc: "One matching rule, custom priority",
			rules: map[string]int{
				"HostSNI(`foobar`)": 0,
				"HostSNI(`bar`)":    10000,
			},
			expectedRule: "HostSNI(`foobar`)",
			serverName:   "foobar",
		},
		{
			desc: "Two matching rules, calculated priority",
			rules: map[string]int{
				"HostSNI(`foobar`)":        0,
				"HostSNI(`foobar`, `bar`)": 0,
			},
			expectedRule: "HostSNI(`foobar`, `bar`)",
			serverName:   "foobar",
		},
		{
			desc: "Two matching rules, custom priority",
			rules: map[string]int{
				"HostSNI(`foobar`)":        10000,
				"HostSNI(`foobar`, `bar`)": 0,
			},
			expectedRule: "HostSNI(`foobar`)",
			serverName:   "foobar",
		},
	}

	for _, test := range testCases {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			muxer, err := NewMuxer()
			require.NoError(t, err)

			matchedRule := ""
			for rule, priority := range test.rules {
				rule := rule
				err := muxer.AddRoute(rule, priority, tcp.HandlerFunc(func(conn tcp.WriteCloser) {
					matchedRule = rule
				}))
				require.NoError(t, err)
			}

			handler := muxer.Match(ConnData{
				serverName: test.serverName,
				remoteIP:   test.remoteIP,
			})
			require.NotNil(t, handler)

			handler.ServeTCP(nil)
			assert.Equal(t, test.expectedRule, matchedRule)
		})
	}
}