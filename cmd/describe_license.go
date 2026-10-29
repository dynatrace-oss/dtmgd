package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/dynatrace-oss/dtmgd/pkg/client"
)

// The Cluster API's license endpoints, relative to {host}/api.
const (
	clusterLicensePath = "/cluster/v2/clusterLicense"
	licenseUsagePath   = "/cluster/v2/clusterLicense/environment/total"
)

// hintClassicLicensing is the hint of the license commands for the errors a
// cluster without a classic license gives: the API exists only from 1.326,
// and only for Dynatrace classic licensing.
const hintClassicLicensing = "The cluster license API needs Dynatrace Managed 1.326+ and works only with Dynatrace classic licensing. A cluster on the Dynatrace Platform Subscription (DPS) has no classic license to show; see its usage in the Cluster Management Console."

// LicenseDetail is the part of the cluster license describe license's summary
// shows.
type LicenseDetail struct {
	LicenseName           string        `json:"licenseName"`
	LicenseType           string        `json:"licenseType"`
	LicenseStatus         string        `json:"licenseStatus"`
	LicenseKey            string        `json:"licenseKey"`
	AccountName           string        `json:"accountName"`
	ClusterID             string        `json:"clusterId"`
	LicenseExpirationTime string        `json:"licenseExpirationTime"`
	ProductVersion        string        `json:"productVersion"`
	LastBillingTime       string        `json:"lastBillingTime"`
	HostUnits             *licenseUsage `json:"usageOfHostUnits"`
	DEMUnits              *licenseUsage `json:"usageOfDemUnits"`
	DDUs                  *licenseUsage `json:"usageOfDduUnits"`
}

// licenseUsage is one unit's usage of the cluster license.
type licenseUsage struct {
	Quota        *float64 `json:"quota"`
	Usage        *float64 `json:"usage"`
	UsagePercent *float64 `json:"usagePercent"`
	UsageStatus  string   `json:"usageStatus"`
	Overage      *struct {
		OverageUsage *float64 `json:"overageUsage"`
	} `json:"overageUsage"`
}

func newDescribeLicenseCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "license",
		Args:  noArgs,
		Short: "Show a Dynatrace Managed cluster's license and billed usage (cluster context)",
		Long: `Show the cluster license and its billed usage of host units, DEM units and
Davis data units, from the Cluster Management API. Needs a cluster context:
one without env-id, whose token has the ServiceProviderAPI scope.

The license key is masked in every output format: only its last four
characters are shown.

The API needs Dynatrace Managed 1.326+ and works only with Dynatrace classic
licensing; a cluster on the Dynatrace Platform Subscription (DPS) has none to
show. Billed usage covers the current contract and lags about two hours.
"dtmgd get license-usage" shows the usage per environment.

Examples:
  dtmgd describe license --context prod-cluster
  dtmgd describe license -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runClusterFetch(fetchSpec{
				Verb:     "describe",
				Resource: "license",
				Fetch:    licenseFetch(maskedLicense(clusterObjectFetch(clusterLicensePath, nil))),
				Human:    detailHuman(printLicenseDetail),
				NotFound: hintClassicLicensing,
			})
		},
	}
	return clusterCommand(cmd)
}

// licenseFetch gives the license endpoints' client errors the classic
// licensing hint. A 401, 403 and 429 keep theirs, and a 404 gets it through
// fetchSpec.NotFound.
func licenseFetch(fetch func(*client.Client, int) (fetched, error)) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		f, err := fetch(c, limit)
		var apiErr *client.ErrAPI
		if !errors.As(err, &apiErr) || apiErr.StatusCode < 400 || apiErr.StatusCode >= 500 {
			return f, err
		}
		switch apiErr.StatusCode {
		case 401, 403, 404, 429:
			return f, err
		}
		return f, withHint(err, hintClassicLicensing)
	}
}

// maskedLicense masks the license key of the license fetch returns, before
// any format can show it.
func maskedLicense(fetch func(*client.Client, int) (fetched, error)) func(*client.Client, int) (fetched, error) {
	return func(c *client.Client, limit int) (fetched, error) {
		f, err := fetch(c, limit)
		if m, ok := f.Body.(map[string]interface{}); ok {
			if k, ok := m["licenseKey"].(string); ok {
				m["licenseKey"] = maskLicenseKey(k)
			}
		}
		return f, err
	}
}

// maskLicenseKey is k with all but its last four characters replaced by
// "****", or "****" alone for a key that short.
func maskLicenseKey(k string) string {
	switch {
	case k == "":
		return ""
	case len(k) <= 4:
		return "****"
	}
	return "****" + k[len(k)-4:]
}

func printLicenseDetail(w io.Writer, d LicenseDetail) {
	line := func(label, value string) {
		if value != "" {
			_, _ = fmt.Fprintf(w, "%-12s %s\n", label+":", value)
		}
	}
	name := d.LicenseName
	if d.LicenseType != "" || d.LicenseStatus != "" {
		name = fmt.Sprintf("%s (%s, %s)", d.LicenseName, d.LicenseType, d.LicenseStatus)
	}
	line("License", name)
	line("Key", d.LicenseKey)
	line("Account", d.AccountName)
	line("Cluster ID", d.ClusterID)
	line("Expires", d.LicenseExpirationTime)
	line("Version", d.ProductVersion)
	line("Billed at", d.LastBillingTime)
	line("Host units", unitUsage(d.HostUnits))
	line("DEM units", unitUsage(d.DEMUnits))
	line("DDUs", unitUsage(d.DDUs))
}

// unitUsage is "42.5 of 100 (42.5%), USING_QUOTA", with ", overage n" when
// the unit is in overage, or "" when the license has no such unit.
func unitUsage(u *licenseUsage) string {
	if u == nil || u.Usage == nil {
		return ""
	}
	s := formatNumber(*u.Usage)
	if u.Quota != nil {
		s += " of " + formatNumber(*u.Quota)
	}
	if u.UsagePercent != nil {
		s += " (" + formatNumber(*u.UsagePercent) + "%)"
	}
	if u.UsageStatus != "" {
		s += ", " + u.UsageStatus
	}
	if o := u.Overage; o != nil && o.OverageUsage != nil && *o.OverageUsage > 0 {
		s += ", overage " + formatNumber(*o.OverageUsage)
	}
	return s
}
