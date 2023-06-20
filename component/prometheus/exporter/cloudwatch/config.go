package cloudwatch

import (
	"github.com/grafana/agent/pkg/integrations/cloudwatch_exporter"
	yaceConf "github.com/nerdswords/yet-another-cloudwatch-exporter/pkg/config"
	yaceModel "github.com/nerdswords/yet-another-cloudwatch-exporter/pkg/model"
	"time"
)

// Since we are gathering metrics from CloudWatch and writing them in prometheus during each scrape, the timestamp
// used should be the scrape one
var addCloudwatchTimestamp = false

// Avoid producing absence of values in metrics
var nilToZero = true

var defaults = Arguments{
	Debug:                 false,
	DiscoveryExportedTags: nil,
	FIPSDisabled:          true,
}

// Arguments are the river based options to configure the embedded CloudWatch exporter.
type Arguments struct {
	STSRegion             string           `river:"sts_region,attr"`
	FIPSDisabled          bool             `river:"fips_disabled,attr,optional"`
	Debug                 bool             `river:"debug,attr,optional"`
	DiscoveryExportedTags TagsPerNamespace `river:"discovery_exported_tags,attr,optional"`
	Discovery             []DiscoveryJob   `river:"discovery,block,optional"`
	Static                []StaticJob      `river:"static,block,optional"`
}

type TagsPerNamespace = cloudwatch_exporter.TagsPerNamespace

// DiscoveryJob configures a discovery job for a given service.
type DiscoveryJob struct {
	Auth       RegionAndRoles `river:",squash"`
	CustomTags Tags           `river:"custom_tags,attr,optional"`
	SearchTags Tags           `river:"search_tags,attr,optional"`
	Type       string         `river:"type,attr"`
	Metrics    []Metric       `river:"metric,block"`
}

// Tags represents a series of tags configured on an AWS resource. Each tag is a
// key value pair in the dictionary.
type Tags map[string]string

// StaticJob will scrape metrics that match all defined dimensions.
type StaticJob struct {
	Name       string         `river:",label"`
	Auth       RegionAndRoles `river:",squash"`
	CustomTags Tags           `river:"custom_tags,attr,optional"`
	Namespace  string         `river:"namespace,attr"`
	Dimensions Dimensions     `river:"dimensions,attr"`
	Metrics    []Metric       `river:"metric,block"`
}

// RegionAndRoles exposes for each supported job, the AWS regions and IAM roles in which the agent should perform the
// scrape.
type RegionAndRoles struct {
	Regions []string `river:"regions,attr"`
	Roles   []Role   `river:"roles,block,optional"`
}

type Role struct {
	RoleArn    string `river:"role_arn,attr"`
	ExternalID string `river:"external_id,attr,optional"`
}

// Dimensions are the label values used to identify a unique metric stream in CloudWatch.
// Each key value pair in the dictionary corresponds to a label value pair.
type Dimensions map[string]string

type Metric struct {
	Name       string        `river:"name,attr"`
	Statistics []string      `river:"statistics,attr"`
	Period     time.Duration `river:"period,attr"`
}

// SetToDefault implements river.Defaulter.
func (a *Arguments) SetToDefault() {
	*a = defaults
}

// ConvertToYACE converts the river config into YACE config model. Note that the conversion is
// not direct, some values have been opinionated to simplify the config model the agent exposes
// for this integration.
func ConvertToYACE(a Arguments) (yaceConf.ScrapeConf, error) {
	var discoveryJobs []*yaceConf.Job
	for _, job := range a.Discovery {
		discoveryJobs = append(discoveryJobs, toYACEDiscoveryJob(job))
	}
	var staticJobs []*yaceConf.Static
	for _, stat := range a.Static {
		staticJobs = append(staticJobs, toYACEStaticJob(stat))
	}
	conf := yaceConf.ScrapeConf{
		APIVersion: "v1alpha1",
		StsRegion:  a.STSRegion,
		Discovery: yaceConf.Discovery{
			ExportedTagsOnMetrics: yaceModel.ExportedTagsOnMetrics(a.DiscoveryExportedTags),
			Jobs:                  discoveryJobs,
		},
		Static: staticJobs,
	}

	// Run the exporter's config validation. Between other things, it will check that the service for which a discovery
	// job is instantiated, it's supported.
	if err := conf.Validate(); err != nil {
		return yaceConf.ScrapeConf{}, err
	}
	patchYACEDefaults(&conf)

	return conf, nil
}

// patchYACEDefaults overrides some default values YACE applies after validation.
func patchYACEDefaults(yc *yaceConf.ScrapeConf) {
	// YACE doesn't allow during validation a zero-delay in each metrics scrape. Override this behaviour since it's taken
	// into account by the rounding period.
	// https://github.com/nerdswords/yet-another-cloudwatch-exporter/blob/7e5949124bb5f26353eeff298724a5897de2a2a4/pkg/config/config.go#L320
	for _, job := range yc.Discovery.Jobs {
		for _, metric := range job.Metrics {
			metric.Delay = 0
		}
	}
}

func (tags Tags) toYACE() []yaceModel.Tag {
	yaceTags := []yaceModel.Tag{}
	for key, value := range tags {
		yaceTags = append(yaceTags, yaceModel.Tag{Key: key, Value: value})
	}
	return yaceTags
}

func toYACERoles(rs []Role) []yaceConf.Role {
	yaceRoles := []yaceConf.Role{}
	for _, r := range rs {
		yaceRoles = append(yaceRoles, yaceConf.Role{RoleArn: r.RoleArn, ExternalID: r.ExternalID})
	}
	return yaceRoles
}

func toYACEMetrics(ms []Metric) []*yaceConf.Metric {
	yaceMetrics := []*yaceConf.Metric{}
	for _, m := range ms {
		periodSeconds := int64(m.Period.Seconds())
		lengthSeconds := periodSeconds
		yaceMetrics = append(yaceMetrics, &yaceConf.Metric{
			Name:       m.Name,
			Statistics: m.Statistics,

			// Length dictates the size of the window for whom we request metrics, that is, endTime - startTime. Period
			// dictates the size of the buckets in which we aggregate data, inside that window. Since data will be scraped
			// by the agent every so often, dictated by the scrapedInterval, CloudWatch should return a single datapoint
			// for each requested metric. That is if Period >= Length, but is Period > Length, we will be getting not enough
			// data to fill the whole aggregation bucket. Therefore, Period == Length.
			Period: periodSeconds,
			Length: lengthSeconds,

			// Delay moves back the time window for whom CloudWatch is requested data. Since we are already adjusting
			// this with RoundingPeriod (see toYACEDiscoveryJob), we should omit this setting.
			Delay: 0,

			NilToZero:              &nilToZero,
			AddCloudwatchTimestamp: &addCloudwatchTimestamp,
		})
	}
	return yaceMetrics
}

func toYACEStaticJob(sj StaticJob) *yaceConf.Static {
	dims := []yaceConf.Dimension{}
	for name, value := range sj.Dimensions {
		dims = append(dims, yaceConf.Dimension{
			Name:  name,
			Value: value,
		})
	}
	return &yaceConf.Static{
		Name:       sj.Name,
		Regions:    sj.Auth.Regions,
		Roles:      toYACERoles(sj.Auth.Roles),
		Namespace:  sj.Namespace,
		CustomTags: sj.CustomTags.toYACE(),
		Dimensions: dims,
		Metrics:    toYACEMetrics(sj.Metrics),
	}
}

func toYACEDiscoveryJob(rj DiscoveryJob) *yaceConf.Job {
	job := &yaceConf.Job{
		Regions:    rj.Auth.Regions,
		Roles:      toYACERoles(rj.Auth.Roles),
		Type:       rj.Type,
		CustomTags: rj.CustomTags.toYACE(),
		SearchTags: rj.SearchTags.toYACE(),
		// By setting RoundingPeriod to nil, the exporter will align the start and end times for retrieving CloudWatch
		// metrics, with the smallest period in the retrieved batch.
		RoundingPeriod: nil,
		JobLevelMetricFields: yaceConf.JobLevelMetricFields{
			// Set to zero job-wide scraping time settings. This should be configured at the metric level to make the data
			// being fetched more explicit.
			Period:                 0,
			Length:                 0,
			Delay:                  0,
			NilToZero:              &nilToZero,
			AddCloudwatchTimestamp: &addCloudwatchTimestamp,
		},
		Metrics: toYACEMetrics(rj.Metrics),
	}
	return job
}
