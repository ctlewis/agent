// Package loadbalancing provides an otelcol.exporter.loadbalancing component.
package loadbalancing

import (
	"time"

	"github.com/grafana/agent/component"
	"github.com/grafana/agent/component/otelcol"
	"github.com/grafana/agent/component/otelcol/exporter"
	"github.com/grafana/agent/pkg/river"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter"
	otelcomponent "go.opentelemetry.io/collector/component"
	otelconfig "go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
)

func init() {
	component.Register(component.Registration{
		Name:    "otelcol.exporter.loadbalancing",
		Args:    Arguments{},
		Exports: otelcol.ConsumerExports{},

		Build: func(opts component.Options, args component.Arguments) (component.Component, error) {
			fact := loadbalancingexporter.NewFactory()
			return exporter.New(opts, fact, args.(Arguments))
		},
	})
}

// Arguments configures the otelcol.exporter.loadbalancing component.
type Arguments struct {
	Protocol   Protocol         `river:"protocol,block"`
	Resolver   ResolverSettings `river:"resolver,block"`
	RoutingKey string           `river:"routing_key,attr,optional"`
}

var (
	_ exporter.Arguments = Arguments{}
	_ river.Defaulter    = &Arguments{}
)

// DefaultArguments holds default values for Arguments.
var DefaultArguments = Arguments{
	RoutingKey: "traceID",
}

// SetToDefault implements river.Defaulter.
func (args *Arguments) SetToDefault() {
	*args = DefaultArguments
}

// Convert implements exporter.Arguments.
func (args Arguments) Convert() (otelconfig.Exporter, error) {
	return &loadbalancingexporter.Config{
		ExporterSettings: otelconfig.NewExporterSettings(otelconfig.NewComponentID("loadbalancing")),
		Protocol:         args.Protocol.Convert(),
		Resolver:         args.Resolver.Convert(),
		RoutingKey:       args.RoutingKey,
	}, nil
}

// Protocol holds the individual protocol-specific settings. Only OTLP is supported at the moment.
type Protocol struct {
	OTLP OtlpConfig `river:"otlp,block"`
}

func (protocol Protocol) Convert() loadbalancingexporter.Protocol {
	return loadbalancingexporter.Protocol{
		OTLP: protocol.OTLP.Convert(),
	}
}

// OtlpConfig defines the config for an OTLP exporter
type OtlpConfig struct {
	Timeout time.Duration          `river:"timeout,attr,optional"`
	Queue   otelcol.QueueArguments `river:"queue,block,optional"`
	Retry   otelcol.RetryArguments `river:"retry,block,optional"`
	// Most of the time, the user will not have to set anything in the client block.
	// However, the block should not be "optional" so that the defaults are populated.
	Client GRPCClientArguments `river:"client,block"`
}

func (otlpConfig OtlpConfig) Convert() otlpexporter.Config {
	return otlpexporter.Config{
		TimeoutSettings: exporterhelper.TimeoutSettings{
			Timeout: otlpConfig.Timeout,
		},
		QueueSettings:      *otlpConfig.Queue.Convert(),
		RetrySettings:      *otlpConfig.Retry.Convert(),
		GRPCClientSettings: *(*otelcol.GRPCClientArguments)(&otlpConfig.Client).Convert(),
	}
}

// ResolverSettings defines the configurations for the backend resolver
type ResolverSettings struct {
	Static *StaticResolver `river:"static,block,optional"`
	DNS    *DNSResolver    `river:"dns,block,optional"`
}

func (resolverSettings ResolverSettings) Convert() loadbalancingexporter.ResolverSettings {
	res := loadbalancingexporter.ResolverSettings{}

	if resolverSettings.Static != nil {
		staticResolver := resolverSettings.Static.Convert()
		res.Static = &staticResolver
	}

	if resolverSettings.DNS != nil {
		dnsResover := resolverSettings.DNS.Convert()
		res.DNS = &dnsResover
	}

	return res
}

// StaticResolver defines the configuration for the resolver providing a fixed list of backends
type StaticResolver struct {
	Hostnames []string `river:"hostnames,attr"`
}

func (staticResolver StaticResolver) Convert() loadbalancingexporter.StaticResolver {
	return loadbalancingexporter.StaticResolver{
		Hostnames: staticResolver.Hostnames,
	}
}

// DNSResolver defines the configuration for the DNS resolver
type DNSResolver struct {
	Hostname string        `river:"hostname,attr"`
	Port     string        `river:"port,attr,optional"`
	Interval time.Duration `river:"interval,attr,optional"`
	Timeout  time.Duration `river:"timeout,attr,optional"`
}

var (
	_ river.Defaulter = &DNSResolver{}
)

// DefaultDNSResolver holds default values for DNSResolver.
var DefaultDNSResolver = DNSResolver{
	Port:     "4317",
	Interval: 5 * time.Second,
	Timeout:  1 * time.Second,
}

// SetToDefault implements river.Defaulter.
func (args *DNSResolver) SetToDefault() {
	*args = DefaultDNSResolver
}

func (dnsResolver *DNSResolver) Convert() loadbalancingexporter.DNSResolver {
	return loadbalancingexporter.DNSResolver{
		Hostname: dnsResolver.Hostname,
		Port:     dnsResolver.Port,
		Interval: dnsResolver.Interval,
		Timeout:  dnsResolver.Timeout,
	}
}

// Extensions implements exporter.Arguments.
func (args Arguments) Extensions() map[otelconfig.ComponentID]otelcomponent.Extension {
	//TODO: Do we need to set these extensions?
	return (*otelcol.GRPCClientArguments)(&args.Protocol.OTLP.Client).Extensions()
}

// Exporters implements exporter.Arguments.
func (args Arguments) Exporters() map[otelconfig.DataType]map[otelconfig.ComponentID]otelcomponent.Exporter {
	return nil
}

// GRPCClientArguments is used to configure otelcol.exporter.loadbalancing with
// component-specific defaults.
type GRPCClientArguments otelcol.GRPCClientArguments

var (
	_ river.Defaulter = &GRPCClientArguments{}
)

// DefaultGRPCClientArguments holds component-specific default settings for
// GRPCClientArguments.
var DefaultGRPCClientArguments = GRPCClientArguments{
	Headers:         map[string]string{},
	Compression:     otelcol.CompressionTypeGzip,
	WriteBufferSize: 512 * 1024,
}

// SetToDefault implements river.Defaulter.
func (args *GRPCClientArguments) SetToDefault() {
	*args = DefaultGRPCClientArguments
}
