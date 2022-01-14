package dynamic

import (
	"reflect"
	"time"

	ptypes "github.com/traefik/paerser/types"
	traefiktls "github.com/traefik/traefik/v2/pkg/tls"
	"github.com/traefik/traefik/v2/pkg/types"
)

// +k8s:deepcopy-gen=true

// HTTPConfiguration contains all the HTTP configuration parameters.
type HTTPConfiguration struct {
	Routers           map[string]*Router           `json:"routers,omitempty" toml:"routers,omitempty" yaml:"routers,omitempty" export:"true" loggable:"true"`
	Services          map[string]*Service          `json:"services,omitempty" toml:"services,omitempty" yaml:"services,omitempty" export:"true" loggable:"true"`
	Middlewares       map[string]*Middleware       `json:"middlewares,omitempty" toml:"middlewares,omitempty" yaml:"middlewares,omitempty" export:"true" loggable:"true"`
	Models            map[string]*Model            `json:"models,omitempty" toml:"models,omitempty" yaml:"models,omitempty" export:"true" loggable:"true"`
	ServersTransports map[string]*ServersTransport `json:"serversTransports,omitempty" toml:"serversTransports,omitempty" yaml:"serversTransports,omitempty" label:"-" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// Model is a set of default router's values.
type Model struct {
	Middlewares []string         `json:"middlewares,omitempty" toml:"middlewares,omitempty" yaml:"middlewares,omitempty" export:"true" loggable:"true"`
	TLS         *RouterTLSConfig `json:"tls,omitempty" toml:"tls,omitempty" yaml:"tls,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// Service holds a service configuration (can only be of one type at the same time).
type Service struct {
	LoadBalancer *ServersLoadBalancer `json:"loadBalancer,omitempty" toml:"loadBalancer,omitempty" yaml:"loadBalancer,omitempty" export:"true" loggable:"true"`
	Weighted     *WeightedRoundRobin  `json:"weighted,omitempty" toml:"weighted,omitempty" yaml:"weighted,omitempty" label:"-" export:"true" loggable:"true"`
	Mirroring    *Mirroring           `json:"mirroring,omitempty" toml:"mirroring,omitempty" yaml:"mirroring,omitempty" label:"-" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// Router holds the router configuration.
type Router struct {
	EntryPoints []string         `json:"entryPoints,omitempty" toml:"entryPoints,omitempty" yaml:"entryPoints,omitempty" export:"true" loggable:"true"`
	Middlewares []string         `json:"middlewares,omitempty" toml:"middlewares,omitempty" yaml:"middlewares,omitempty" export:"true" loggable:"true"`
	Service     string           `json:"service,omitempty" toml:"service,omitempty" yaml:"service,omitempty" export:"true" loggable:"true"`
	Rule        string           `json:"rule,omitempty" toml:"rule,omitempty" yaml:"rule,omitempty"`
	Priority    int              `json:"priority,omitempty" toml:"priority,omitempty,omitzero" yaml:"priority,omitempty" export:"true" loggable:"true"`
	TLS         *RouterTLSConfig `json:"tls,omitempty" toml:"tls,omitempty" yaml:"tls,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// RouterTLSConfig holds the TLS configuration for a router.
type RouterTLSConfig struct {
	Options      string         `json:"options,omitempty" toml:"options,omitempty" yaml:"options,omitempty" export:"true" loggable:"true"`
	CertResolver string         `json:"certResolver,omitempty" toml:"certResolver,omitempty" yaml:"certResolver,omitempty" export:"true" loggable:"true"`
	Domains      []types.Domain `json:"domains,omitempty" toml:"domains,omitempty" yaml:"domains,omitempty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// Mirroring holds the Mirroring configuration.
type Mirroring struct {
	Service     string          `json:"service,omitempty" toml:"service,omitempty" yaml:"service,omitempty" export:"true" loggable:"true"`
	MaxBodySize *int64          `json:"maxBodySize,omitempty" toml:"maxBodySize,omitempty" yaml:"maxBodySize,omitempty" export:"true" loggable:"true"`
	Mirrors     []MirrorService `json:"mirrors,omitempty" toml:"mirrors,omitempty" yaml:"mirrors,omitempty" export:"true" loggable:"true"`
	HealthCheck *HealthCheck    `json:"healthCheck,omitempty" toml:"healthCheck,omitempty" yaml:"healthCheck,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true" loggable:"true"`
}

// SetDefaults Default values for a WRRService.
func (m *Mirroring) SetDefaults() {
	var defaultMaxBodySize int64 = -1
	m.MaxBodySize = &defaultMaxBodySize
}

// +k8s:deepcopy-gen=true

// MirrorService holds the MirrorService configuration.
type MirrorService struct {
	Name    string `json:"name,omitempty" toml:"name,omitempty" yaml:"name,omitempty" export:"true" loggable:"true"`
	Percent int    `json:"percent,omitempty" toml:"percent,omitempty" yaml:"percent,omitempty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// WeightedRoundRobin is a weighted round robin load-balancer of services.
type WeightedRoundRobin struct {
	Services []WRRService `json:"services,omitempty" toml:"services,omitempty" yaml:"services,omitempty" export:"true" loggable:"true"`
	Sticky   *Sticky      `json:"sticky,omitempty" toml:"sticky,omitempty" yaml:"sticky,omitempty" export:"true" loggable:"true"`
	// HealthCheck enables automatic self-healthcheck for this service, i.e.
	// whenever one of its children is reported as down, this service becomes aware of it,
	// and takes it into account (i.e. it ignores the down child) when running the
	// load-balancing algorithm. In addition, if the parent of this service also has
	// HealthCheck enabled, this service reports to its parent any status change.
	HealthCheck *HealthCheck `json:"healthCheck,omitempty" toml:"healthCheck,omitempty" yaml:"healthCheck,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// WRRService is a reference to a service load-balanced with weighted round robin.
type WRRService struct {
	Name   string `json:"name,omitempty" toml:"name,omitempty" yaml:"name,omitempty" export:"true" loggable:"true"`
	Weight *int   `json:"weight,omitempty" toml:"weight,omitempty" yaml:"weight,omitempty" export:"true" loggable:"true"`
}

// SetDefaults Default values for a WRRService.
func (w *WRRService) SetDefaults() {
	defaultWeight := 1
	w.Weight = &defaultWeight
}

// +k8s:deepcopy-gen=true

// Sticky holds the sticky configuration.
type Sticky struct {
	Cookie *Cookie `json:"cookie,omitempty" toml:"cookie,omitempty" yaml:"cookie,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// Cookie holds the sticky configuration based on cookie.
type Cookie struct {
	Name     string `json:"name,omitempty" toml:"name,omitempty" yaml:"name,omitempty" export:"true" loggable:"true"`
	Secure   bool   `json:"secure,omitempty" toml:"secure,omitempty" yaml:"secure,omitempty" export:"true" loggable:"true"`
	HTTPOnly bool   `json:"httpOnly,omitempty" toml:"httpOnly,omitempty" yaml:"httpOnly,omitempty" export:"true" loggable:"true"`
	SameSite string `json:"sameSite,omitempty" toml:"sameSite,omitempty" yaml:"sameSite,omitempty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// ServersLoadBalancer holds the ServersLoadBalancer configuration.
type ServersLoadBalancer struct {
	Sticky  *Sticky  `json:"sticky,omitempty" toml:"sticky,omitempty" yaml:"sticky,omitempty" label:"allowEmpty" file:"allowEmpty" export:"true" loggable:"true"`
	Servers []Server `json:"servers,omitempty" toml:"servers,omitempty" yaml:"servers,omitempty" label-slice-as-struct:"server" export:"true" loggable:"true"`
	// HealthCheck enables regular active checks of the responsiveness of the
	// children servers of this load-balancer. To propagate status changes (e.g. all
	// servers of this service are down) upwards, HealthCheck must also be enabled on
	// the parent(s) of this service.
	HealthCheck        *ServerHealthCheck  `json:"healthCheck,omitempty" toml:"healthCheck,omitempty" yaml:"healthCheck,omitempty" export:"true" loggable:"true"`
	PassHostHeader     *bool               `json:"passHostHeader" toml:"passHostHeader" yaml:"passHostHeader" export:"true" loggable:"true"`
	ResponseForwarding *ResponseForwarding `json:"responseForwarding,omitempty" toml:"responseForwarding,omitempty" yaml:"responseForwarding,omitempty" export:"true" loggable:"true"`
	ServersTransport   string              `json:"serversTransport,omitempty" toml:"serversTransport,omitempty" yaml:"serversTransport,omitempty" export:"true" loggable:"true"`
}

// Mergeable tells if the given service is mergeable.
func (l *ServersLoadBalancer) Mergeable(loadBalancer *ServersLoadBalancer) bool {
	savedServers := l.Servers
	defer func() {
		l.Servers = savedServers
	}()
	l.Servers = nil

	savedServersLB := loadBalancer.Servers
	defer func() {
		loadBalancer.Servers = savedServersLB
	}()
	loadBalancer.Servers = nil

	return reflect.DeepEqual(l, loadBalancer)
}

// SetDefaults Default values for a ServersLoadBalancer.
func (l *ServersLoadBalancer) SetDefaults() {
	defaultPassHostHeader := true
	l.PassHostHeader = &defaultPassHostHeader
}

// +k8s:deepcopy-gen=true

// ResponseForwarding holds configuration for the forward of the response.
type ResponseForwarding struct {
	FlushInterval string `json:"flushInterval,omitempty" toml:"flushInterval,omitempty" yaml:"flushInterval,omitempty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// Server holds the server configuration.
type Server struct {
	URL    string `json:"url,omitempty" toml:"url,omitempty" yaml:"url,omitempty" label:"-"`
	Scheme string `toml:"-" json:"-" yaml:"-" file:"-"`
	Port   string `toml:"-" json:"-" yaml:"-" file:"-"`
}

// SetDefaults Default values for a Server.
func (s *Server) SetDefaults() {
	s.Scheme = "http"
}

// +k8s:deepcopy-gen=true

// ServerHealthCheck holds the HealthCheck configuration.
type ServerHealthCheck struct {
	Scheme string `json:"scheme,omitempty" toml:"scheme,omitempty" yaml:"scheme,omitempty" export:"true" loggable:"true"`
	Path   string `json:"path,omitempty" toml:"path,omitempty" yaml:"path,omitempty" export:"true" loggable:"true"`
	Port   int    `json:"port,omitempty" toml:"port,omitempty,omitzero" yaml:"port,omitempty" export:"true" loggable:"true"`
	// FIXME change string to ptypes.Duration
	Interval string `json:"interval,omitempty" toml:"interval,omitempty" yaml:"interval,omitempty" export:"true" loggable:"true"`
	// FIXME change string to ptypes.Duration
	Timeout         string            `json:"timeout,omitempty" toml:"timeout,omitempty" yaml:"timeout,omitempty" export:"true" loggable:"true"`
	Hostname        string            `json:"hostname,omitempty" toml:"hostname,omitempty" yaml:"hostname,omitempty"`
	FollowRedirects *bool             `json:"followRedirects" toml:"followRedirects" yaml:"followRedirects" export:"true" loggable:"true"`
	Headers         map[string]string `json:"headers,omitempty" toml:"headers,omitempty" yaml:"headers,omitempty" export:"true" loggable:"true"`
}

// SetDefaults Default values for a HealthCheck.
func (h *ServerHealthCheck) SetDefaults() {
	fr := true
	h.FollowRedirects = &fr
}

// +k8s:deepcopy-gen=true

// HealthCheck controls healthcheck awareness and propagation at the services level.
type HealthCheck struct{}

// +k8s:deepcopy-gen=true

// ServersTransport options to configure communication between Traefik and the servers.
type ServersTransport struct {
	ServerName          string                     `description:"ServerName used to contact the server" json:"serverName,omitempty" toml:"serverName,omitempty" yaml:"serverName,omitempty"`
	InsecureSkipVerify  bool                       `description:"Disable SSL certificate verification." json:"insecureSkipVerify,omitempty" toml:"insecureSkipVerify,omitempty" yaml:"insecureSkipVerify,omitempty" export:"true" loggable:"true"`
	RootCAs             []traefiktls.FileOrContent `description:"Add cert file for self-signed certificate." json:"rootCAs,omitempty" toml:"rootCAs,omitempty" yaml:"rootCAs,omitempty"`
	Certificates        traefiktls.Certificates    `description:"Certificates for mTLS." json:"certificates,omitempty" toml:"certificates,omitempty" yaml:"certificates,omitempty" export:"true" loggable:"true"`
	MaxIdleConnsPerHost int                        `description:"If non-zero, controls the maximum idle (keep-alive) to keep per-host. If zero, DefaultMaxIdleConnsPerHost is used" json:"maxIdleConnsPerHost,omitempty" toml:"maxIdleConnsPerHost,omitempty" yaml:"maxIdleConnsPerHost,omitempty" export:"true" loggable:"true"`
	ForwardingTimeouts  *ForwardingTimeouts        `description:"Timeouts for requests forwarded to the backend servers." json:"forwardingTimeouts,omitempty" toml:"forwardingTimeouts,omitempty" yaml:"forwardingTimeouts,omitempty" export:"true" loggable:"true"`
	DisableHTTP2        bool                       `description:"Disable HTTP/2 for connections with backend servers." json:"disableHTTP2,omitempty" toml:"disableHTTP2,omitempty" yaml:"disableHTTP2,omitempty" export:"true" loggable:"true"`
	PeerCertURI         string                     `description:"URI used to match against SAN URI during the peer certificate verification." json:"peerCertURI,omitempty" toml:"peerCertURI,omitempty" yaml:"peerCertURI,omitempty" export:"true" loggable:"true"`
}

// +k8s:deepcopy-gen=true

// ForwardingTimeouts contains timeout configurations for forwarding requests to the backend servers.
type ForwardingTimeouts struct {
	DialTimeout           ptypes.Duration `description:"The amount of time to wait until a connection to a backend server can be established. If zero, no timeout exists." json:"dialTimeout,omitempty" toml:"dialTimeout,omitempty" yaml:"dialTimeout,omitempty" export:"true" loggable:"true"`
	ResponseHeaderTimeout ptypes.Duration `description:"The amount of time to wait for a server's response headers after fully writing the request (including its body, if any). If zero, no timeout exists." json:"responseHeaderTimeout,omitempty" toml:"responseHeaderTimeout,omitempty" yaml:"responseHeaderTimeout,omitempty" export:"true" loggable:"true"`
	IdleConnTimeout       ptypes.Duration `description:"The maximum period for which an idle HTTP keep-alive connection will remain open before closing itself" json:"idleConnTimeout,omitempty" toml:"idleConnTimeout,omitempty" yaml:"idleConnTimeout,omitempty" export:"true" loggable:"true"`
}

// SetDefaults sets the default values.
func (f *ForwardingTimeouts) SetDefaults() {
	f.DialTimeout = ptypes.Duration(30 * time.Second)
	f.IdleConnTimeout = ptypes.Duration(90 * time.Second)
}
