package vpn

import (
	"errors"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-check/check"
)

const KeyFile = "tailscale.secret"

var (
	vpn   *tailscaleNotSuite
	mu    sync.Mutex
	count int
)

func NewVPN() *tailscaleNotSuite {
	mu.Lock()
	defer mu.Unlock()
	defer func() { count += 1 }()

	if vpn != nil {
		return vpn
	}

	vpn = setupVPN(nil, Keyfile)
	return vpn
}

func TearDown(c *check.C) {
	mu.Lock()
	defer mu.Unlock()

	if count != 0 {
		count -= 1
		return
	}

	if vpn.composeProject != nil && vpn.dockerComposeService != nil {
		vpn.composeDown(c)
	}

	vpn = nil
}

func CanVPN() bool {
	_, err := os.Stat(KeyFile)
	return !errors.Is(err, os.ErrNotExist)
}

// tailscaleNotSuite includes a BaseSuite out of convenience, so we can benefit
// from composeUp et co., but it is not meant to function as a TestSuite per se.
type tailscaleNotSuite struct{ BaseSuite }

// setupVPN starts tailscale on the corresponding container, and makes it a subnet
// router, for all the other containers (whoamis, etc) subsequently started for the
// integration tests.
// It only does so if the file provided as argument exists, and contains a
// tailscale auth key (an ephemeral, but reusable, one is recommended).
//
// Add this section to your tailscale ACLs to auto-approve the routes for the
// containers in the docker subnet:
//
// "autoApprovers": {
//   // Allow myself to automatically advertize routes for docker networks
//   "routes": {
//     "172.0.0.0/8": ["your_tailscale_identity"],
//   },
// },
//
// TODO(mpl): we could maybe even move this setup to the Makefile, to start it
// and let it run (forever, or until voluntarily stopped).
func setupVPN(c *check.C, keyFile string) *tailscaleNotSuite {
	data, err := ioutil.ReadFile(keyFile)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Fatal(err)
		}
		return nil
	}
	authKey := strings.TrimSpace(string(data))
	// TODO: copy and create versions that don't need a check.C?
	vpn := &tailscaleNotSuite{}
	vpn.createComposeProject(c, "tailscale")
	vpn.composeUp(c)
	time.Sleep(5 * time.Second)
	// TODO(mpl): make sure this docker subnet stays the same as the one we setup in Makefile.
	vpn.composeExec(c, "tailscaled", "tailscale", "up", "--authkey="+authKey, "--advertise-routes=172.31.42.0/24")
	return vpn
}
