package guard

import (
	"time"

	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/httpx"
	"github.com/hicancan/njupt-net-cli/internal/portal"
	"github.com/hicancan/njupt-net-cli/internal/selfservice"
	"github.com/hicancan/njupt-net-cli/internal/workflow"
)

type clientFactory struct {
	settings Settings
}

func newClientFactory(settings Settings) workflow.GuardClientFactory {
	return clientFactory{settings: settings}
}

func (f clientFactory) NewSelf() (workflow.GuardSelfClient, error) {
	session, err := httpx.NewSessionClient(httpx.Options{
		BaseURL:     f.settings.SelfBaseURL,
		Timeout:     maxDuration(f.settings.SelfTimeout, 5*time.Second),
		InsecureTLS: f.settings.InsecureTLS,
	})
	if err != nil {
		return nil, err
	}
	return selfservice.NewClient(session), nil
}

func (f clientFactory) NewPortal() (workflow.GuardPortalClient, error) {
	session, err := httpx.NewSessionClient(httpx.Options{
		BaseURL:     f.settings.PortalBaseURL,
		Timeout:     maxDuration(f.settings.PortalTimeout, 5*time.Second),
		InsecureTLS: f.settings.InsecureTLS,
	})
	if err != nil {
		return nil, err
	}
	return portal.NewClient(session, f.settings.PortalBaseURL, f.settings.PortalFallbackBaseURL), nil
}

func toWorkflowAccounts(accounts map[string]config.AccountConfig) map[string]workflow.Credentials {
	converted := make(map[string]workflow.Credentials, len(accounts))
	for name, account := range accounts {
		converted[name] = workflow.Credentials{
			Username: account.Username,
			Password: account.Password,
		}
	}
	return converted
}

func toWorkflowBroadband(broadband config.BroadbandConfig) workflow.BroadbandCredentials {
	return workflow.BroadbandCredentials{
		Account:  broadband.Account,
		Password: broadband.Password,
	}
}

func maxDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
