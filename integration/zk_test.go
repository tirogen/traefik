package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/abronan/valkeyrie"
	"github.com/abronan/valkeyrie/store"
	"github.com/abronan/valkeyrie/store/zookeeper"
	composeapi "github.com/docker/compose/v2/pkg/api"
	"github.com/go-check/check"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/traefik/traefik/v2/integration/try"
	"github.com/traefik/traefik/v2/pkg/api"
	checker "github.com/vdemeester/shakers"
)

// Zk test suites (using libcompose).
type ZookeeperSuite struct {
	BaseSuite
	kvClient store.Store
}

func (s *ZookeeperSuite) setupStore(c *check.C) {
	s.createComposeProject(c, "zookeeper")
	err := s.dockerService.Up(context.Background(), s.composeProject, composeapi.UpOptions{})
	c.Assert(err, checker.IsNil)

	zookeeper.Register()
	kv, err := valkeyrie.NewStore(
		store.ZK,
		[]string{"zookeeper:2181"},
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
		},
	)
	if err != nil {
		c.Fatal("Cannot create store zookeeper")
	}
	s.kvClient = kv

	// wait for zk
	err = try.Do(60*time.Second, try.KVExists(kv, "test"))
	c.Assert(err, checker.IsNil)
}

func (s *ZookeeperSuite) TestSimpleConfiguration(c *check.C) {
	s.setupStore(c)

	address := "zookeeper:2181"
	file := s.adaptFile(c, "fixtures/zookeeper/simple.toml", struct{ ZkAddress string }{address})
	defer os.Remove(file)

	data := map[string]string{
		"traefik/http/routers/Router0/entryPoints/0": "web",
		"traefik/http/routers/Router0/middlewares/0": "compressor",
		"traefik/http/routers/Router0/middlewares/1": "striper",
		"traefik/http/routers/Router0/service":       "simplesvc",
		"traefik/http/routers/Router0/rule":          "Host(`kv1.localhost`)",
		"traefik/http/routers/Router0/priority":      "42",
		"traefik/http/routers/Router0/tls":           "",

		"traefik/http/routers/Router1/rule":                 "Host(`kv2.localhost`)",
		"traefik/http/routers/Router1/priority":             "42",
		"traefik/http/routers/Router1/tls/domains/0/main":   "aaa.localhost",
		"traefik/http/routers/Router1/tls/domains/0/sans/0": "aaa.aaa.localhost",
		"traefik/http/routers/Router1/tls/domains/0/sans/1": "bbb.aaa.localhost",
		"traefik/http/routers/Router1/tls/domains/1/main":   "bbb.localhost",
		"traefik/http/routers/Router1/tls/domains/1/sans/0": "aaa.bbb.localhost",
		"traefik/http/routers/Router1/tls/domains/1/sans/1": "bbb.bbb.localhost",
		"traefik/http/routers/Router1/entryPoints/0":        "web",
		"traefik/http/routers/Router1/service":              "simplesvc",

		"traefik/http/services/simplesvc/loadBalancer/servers/0/url": "http://10.0.1.1:8888",
		"traefik/http/services/simplesvc/loadBalancer/servers/1/url": "http://10.0.1.1:8889",

		"traefik/http/services/srvcA/loadBalancer/servers/0/url": "http://10.0.1.2:8888",
		"traefik/http/services/srvcA/loadBalancer/servers/1/url": "http://10.0.1.2:8889",

		"traefik/http/services/srvcB/loadBalancer/servers/0/url": "http://10.0.1.3:8888",
		"traefik/http/services/srvcB/loadBalancer/servers/1/url": "http://10.0.1.3:8889",

		"traefik/http/services/mirror/mirroring/service":           "simplesvc",
		"traefik/http/services/mirror/mirroring/mirrors/0/name":    "srvcA",
		"traefik/http/services/mirror/mirroring/mirrors/0/percent": "42",
		"traefik/http/services/mirror/mirroring/mirrors/1/name":    "srvcB",
		"traefik/http/services/mirror/mirroring/mirrors/1/percent": "42",

		"traefik/http/services/Service03/weighted/services/0/name":   "srvcA",
		"traefik/http/services/Service03/weighted/services/0/weight": "42",
		"traefik/http/services/Service03/weighted/services/1/name":   "srvcB",
		"traefik/http/services/Service03/weighted/services/1/weight": "42",

		"traefik/http/middlewares/compressor/compress":            "",
		"traefik/http/middlewares/striper/stripPrefix/prefixes/0": "foo",
		"traefik/http/middlewares/striper/stripPrefix/prefixes/1": "bar",
		"traefik/http/middlewares/striper/stripPrefix/forceSlash": "true",
	}

	for k, v := range data {
		err := s.kvClient.Put(k, []byte(v), nil)
		c.Assert(err, checker.IsNil)
	}

	cmd, display := s.traefikCmd(withConfigFile(file))
	defer display(c)
	err := cmd.Start()
	c.Assert(err, checker.IsNil)
	defer s.killCmd(cmd)

	// wait for traefik
	err = try.GetRequest("http://127.0.0.1:8080/api/rawdata", 2*time.Second,
		try.BodyContains(`"striper@zookeeper":`, `"compressor@zookeeper":`, `"srvcA@zookeeper":`, `"srvcB@zookeeper":`),
	)
	c.Assert(err, checker.IsNil)

	resp, err := http.Get("http://127.0.0.1:8080/api/rawdata")
	c.Assert(err, checker.IsNil)

	var obtained api.RunTimeRepresentation
	err = json.NewDecoder(resp.Body).Decode(&obtained)
	c.Assert(err, checker.IsNil)
	got, err := json.MarshalIndent(obtained, "", "")
	c.Assert(err, checker.IsNil)

	expectedJSON := filepath.FromSlash("testdata/rawdata-zk.json")

	if *updateExpected {
		err = os.WriteFile(expectedJSON, got, 0o666)
		c.Assert(err, checker.IsNil)
	}

	expected, err := os.ReadFile(expectedJSON)
	c.Assert(err, checker.IsNil)

	// ensure json is minified before testing diff
	expectedStr := minifyJSON(string(expected))
	gotStr := minifyJSON(string(got))

	if !bytes.Equal([]byte(expectedStr), []byte(gotStr)) {
		diff := difflib.UnifiedDiff{
			FromFile: "Expected",
			A:        difflib.SplitLines(expectedStr),
			ToFile:   "Got",
			B:        difflib.SplitLines(gotStr),
			Context:  3,
		}

		text, err := difflib.GetUnifiedDiffString(diff)
		c.Assert(err, checker.IsNil)
		c.Error(text)
	}
}
