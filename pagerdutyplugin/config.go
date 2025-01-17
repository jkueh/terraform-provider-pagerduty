package pagerduty

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/PagerDuty/go-pagerduty"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
	"github.com/terraform-providers/terraform-provider-pagerduty/util"
)

// Config defines the configuration options for the PagerDuty client
type Config struct {
	mu sync.Mutex

	// The PagerDuty API URL
	ApiUrl string

	// Override default PagerDuty API URL
	ApiUrlOverride string

	// The PagerDuty APP URL
	AppUrl string

	// The PagerDuty API V2 token
	Token string

	// The PagerDuty User level token for Slack
	UserToken string

	// Skip validation of the token against the PagerDuty API
	SkipCredsValidation bool

	// Target version for terraform
	TerraformVersion string

	// Region where the server of the service is deployed
	ServiceRegion string

	// Parameters for fine-grained access control
	AppOauthScopedToken *AppOauthScopedToken

	// API wrapper
	client *pagerduty.Client
}

type AppOauthScopedToken struct {
	ClientId, ClientSecret, Subdomain string
}

const invalidCreds = `
No valid credentials found for PagerDuty provider.
Please see https://www.terraform.io/docs/providers/pagerduty/index.html
for more information on providing credentials for this provider.
`

// Client returns a PagerDuty client, initializing when necessary.
func (c *Config) Client(ctx context.Context) (*pagerduty.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return the previously-configured client if available.
	if c.client != nil {
		return c.client, nil
	}

	httpClient := http.DefaultClient
	httpClient.Timeout = 1 * time.Minute
	httpClient.Transport = logging.NewTransport("PagerDuty", http.DefaultTransport)

	apiUrl := c.ApiUrl
	if c.ApiUrlOverride != "" {
		apiUrl = c.ApiUrlOverride
	}

	clientOpts := []pagerduty.ClientOptions{
		WithHTTPClient(httpClient),
		pagerduty.WithAPIEndpoint(apiUrl),
		pagerduty.WithTerraformProvider(c.TerraformVersion),
	}

	if c.AppOauthScopedToken != nil {
		tokenFile := getTokenFilepath()
		account := fmt.Sprintf("as_account-%s.%s", c.ServiceRegion, c.AppOauthScopedToken.Subdomain)
		accountAndScopes := []string{account}
		accountAndScopes = append(accountAndScopes, availableOauthScopes()...)
		opt := pagerduty.WithScopedOAuthAppTokenSource(pagerduty.NewFileTokenSource(
			ctx,
			c.AppOauthScopedToken.ClientId,
			c.AppOauthScopedToken.ClientSecret,
			accountAndScopes,
			tokenFile,
		))
		clientOpts = append(clientOpts, opt)
	}

	// Validate that the PagerDuty token is set
	if c.Token == "" && c.AppOauthScopedToken == nil {
		return nil, fmt.Errorf(invalidCreds)
	}
	client := pagerduty.NewClient(c.Token, clientOpts...)

	if !c.SkipCredsValidation {
		// Validate the credentials by calling the abilities endpoint,
		// if we get a 401 response back we return an error to the user
		if _, err := client.ListAbilitiesWithContext(ctx); err != nil {
			return nil, fmt.Errorf(fmt.Sprintf("%s\n%s", err, invalidCreds))
		}
	}
	c.client = client

	log.Printf("[INFO] PagerDuty plugin client configured")
	return c.client, nil
}

func WithHTTPClient(httpClient pagerduty.HTTPClient) pagerduty.ClientOptions {
	return func(c *pagerduty.Client) {
		if util.IsNilFunc(httpClient) {
			return
		}
		c.HTTPClient = httpClient
	}
}

func getTokenFilepath() string {
	dir, err := os.UserHomeDir()
	if err == nil {
		dir = filepath.Join(dir, ".pagerduty")
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.Mkdir(dir, os.ModeDir|0o755)
		}
	} else {
		dir = ""
	}
	return filepath.Join(dir, "token.json")
}

func availableOauthScopes() []string {
	return []string{
		"abilities.read",
		"addons.read",
		"addons.write",
		"analytics.read",
		"audit_records.read",
		"change_events.read",
		"change_events.write",
		"custom_fields.read",
		"custom_fields.write",
		"escalation_policies.read",
		"escalation_policies.write",
		"event_orchestrations.read",
		"event_orchestrations.write",
		"event_rules.read",
		"event_rules.write",
		"extension_schemas.read",
		"extensions.read",
		"extensions.write",
		"incident_workflows.read",
		"incident_workflows.write",
		"incident_workflows:instances.write",
		"incidents.read",
		"incidents.write",
		"licenses.read",
		"notifications.read",
		"oncalls.read",
		"priorities.read",
		"response_plays.read",
		"response_plays.write",
		"schedules.read",
		"schedules.write",
		"services.read",
		"services.write",
		"standards.read",
		"standards.write",
		"status_dashboards.read",
		"status_pages.read",
		"status_pages.write",
		"subscribers.read",
		"subscribers.write",
		"tags.read",
		"tags.write",
		"teams.read",
		"teams.write",
		"templates.read",
		"templates.write",
		"users.read",
		"users.write",
		"users:contact_methods.read",
		"users:contact_methods.write",
		"users:sessions.read",
		"users:sessions.write",
		"vendors.read",
	}
}

// ConfigurePagerdutyClient sets a pagerduty API client in a pointer `dst` to
// the property of any datasource or resource struct from the general
// configuration of the provider.
func ConfigurePagerdutyClient(dst **pagerduty.Client, providerData any) diag.Diagnostics {
	var diags diag.Diagnostics
	if providerData == nil {
		return diags
	}
	client, ok := providerData.(*pagerduty.Client)
	if !ok {
		diags.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected *github.com/PagerDuty/go-pagerduty.Client, got: %T."+
					"Please report this issue to the provider developers.",
				providerData,
			),
		)
		return diags
	}
	if dst == nil {
		diags.AddError(
			"Bad usage of ConfigurePagerdutyClient",
			"Received a null client destination",
		)
		return diags
	}
	*dst = client
	return diags
}
