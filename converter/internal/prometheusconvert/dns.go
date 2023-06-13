package prometheusconvert

import (
	"time"

	"github.com/grafana/agent/component/discovery"
	"github.com/grafana/agent/component/discovery/dns"
	"github.com/grafana/agent/converter/internal/common"
	"github.com/grafana/agent/pkg/river/token/builder"
	promdns "github.com/prometheus/prometheus/discovery/dns"
)

func appendDiscoveryDns(f *builder.File, jobName string, sdConfig *promdns.SDConfig) discovery.Exports {
	discoveryDnsArgs := toDiscoveryDns(sdConfig)
	common.AppendBlockWithOverride(f, []string{"discovery", "dns"}, jobName, discoveryDnsArgs)
	return discovery.Exports{
		// This target map will utilize a RiverTokenize that results in this
		// component export rather than the standard discovery.Target RiverTokenize.
		Targets: []discovery.Target{map[string]string{"__expr__": "discovery.dns." + jobName + ".targets"}},
	}
}

func toDiscoveryDns(sdConfig *promdns.SDConfig) *dns.Arguments {
	if sdConfig == nil {
		return nil
	}

	return &dns.Arguments{
		Names:           sdConfig.Names,
		RefreshInterval: time.Duration(sdConfig.RefreshInterval),
		Type:            sdConfig.Type,
		Port:            sdConfig.Port,
	}
}