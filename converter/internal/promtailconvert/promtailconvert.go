package promtailconvert

import (
	"bytes"
	"fmt"
	"github.com/grafana/agent/component/common/loki"
	lokiwrite "github.com/grafana/agent/component/loki/write"
	"github.com/grafana/agent/converter/internal/common"
	"github.com/grafana/loki/clients/pkg/promtail/client"
	lokiflag "github.com/grafana/loki/pkg/util/flagext"
	"gopkg.in/yaml.v2"

	"github.com/grafana/agent/converter/diag"
	"github.com/grafana/agent/pkg/river/token/builder"
	promtailcfg "github.com/grafana/loki/clients/pkg/promtail/config"
)

// Convert implements a Promtail config converter.
func Convert(in []byte) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	var cfg promtailcfg.Config
	// TODO: this doesn't handle the defaults correctly. We'd need to import other Loki's packages to do that.
	if err := yaml.UnmarshalStrict(in, &cfg); err != nil {
		diags.Add(diag.SeverityLevelError, fmt.Sprintf("failed to parse Promtail config: %s", err))
		return nil, diags
	}

	f := builder.NewFile()
	diags = AppendAll(f, &cfg)

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		diags.Add(diag.SeverityLevelError, fmt.Sprintf("failed to render Flow config: %s", err.Error()))
		return nil, diags
	}
	return buf.Bytes(), diags
}

// AppendAll analyzes the entire promtail config in memory and transforms it
// into Flow components. It then appends each argument to the file builder.
func AppendAll(f *builder.File, cfg *promtailcfg.Config) diag.Diagnostics {
	var diags diag.Diagnostics

	for i, c := range cfg.ClientConfigs {
		appendLokiWrite(f, &c, i)
	}

	return diags
}

//func appendRemoteWrite(f *builder.File, promConfig *promconfig.Config) *remotewrite.Exports {
//	remoteWriteArgs := toRemotewriteArguments(promConfig)
//	common.AppendBlockWithOverride(f, []string{"prometheus", "remote_write"}, "default", remoteWriteArgs)
//
//	return &remotewrite.Exports{
//		Receiver: common.ConvertAppendable{Expr: "prometheus.remote_write.default.receiver"},
//	}
//}

func appendLokiWrite(f *builder.File, client *client.Config, index int) *lokiwrite.Exports {
	name := fmt.Sprintf("default_%d", index)
	lokiWriteArgs := toLokiWriteArguments(client)
	common.AppendBlockWithOverride(f, []string{"loki", "write"}, name, lokiWriteArgs)
	return &lokiwrite.Exports{
		Receiver: make(loki.LogsReceiver),
	}
}

func toLokiWriteArguments(config *client.Config) *lokiwrite.Arguments {
	return &lokiwrite.Arguments{
		Endpoints: []lokiwrite.EndpointOptions{
			{
				URL: config.URL.String(),
			},
		},
		ExternalLabels: convertLabels(config.ExternalLabels),
	}
}

func convertLabels(labels lokiflag.LabelSet) map[string]string {
	result := map[string]string{}
	for k, v := range labels.LabelSet {
		result[string(k)] = string(v)
	}
	return result
}
