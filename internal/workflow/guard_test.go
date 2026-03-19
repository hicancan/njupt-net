package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/hicancan/njupt-net-cli/internal/config"
	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

type fakeProber struct {
	checks []probeResult
	ips    []string
	ipErr  error
}

type probeResult struct {
	ok      bool
	message string
}

func (p *fakeProber) CheckConnectivity(ctx context.Context) (bool, string) {
	_ = ctx
	if len(p.checks) == 0 {
		return false, "no connectivity result queued"
	}
	result := p.checks[0]
	p.checks = p.checks[1:]
	return result.ok, result.message
}

func (p *fakeProber) DetectLocalIPv4(ctx context.Context) (string, error) {
	_ = ctx
	if p.ipErr != nil {
		return "", p.ipErr
	}
	if len(p.ips) == 0 {
		return "", errors.New("no ip queued")
	}
	ip := p.ips[0]
	p.ips = p.ips[1:]
	return ip, nil
}

type fakeSelfClient struct {
	loginErr    error
	binding     *kernel.OperatorBinding
	bindingErr  error
	bindErrs    []error
	bindTargets []map[string]string
}

func (c *fakeSelfClient) Login(ctx context.Context, username, password string) (*kernel.OperationResult[kernel.SelfLoginResult], error) {
	_ = ctx
	_ = username
	_ = password
	return &kernel.OperationResult[kernel.SelfLoginResult]{Level: kernel.EvidenceConfirmed, Success: c.loginErr == nil}, c.loginErr
}

func (c *fakeSelfClient) GetOperatorBinding(ctx context.Context) (*kernel.OperationResult[kernel.OperatorBinding], error) {
	_ = ctx
	return &kernel.OperationResult[kernel.OperatorBinding]{
		Level:   kernel.EvidenceConfirmed,
		Success: c.bindingErr == nil,
		Data:    c.binding,
	}, c.bindingErr
}

func (c *fakeSelfClient) BindOperator(ctx context.Context, target map[string]string, readback, restore bool) (*kernel.OperationResult[kernel.WriteBackResult], error) {
	_ = ctx
	_ = readback
	_ = restore
	c.bindTargets = append(c.bindTargets, target)
	var err error
	if len(c.bindErrs) > 0 {
		err = c.bindErrs[0]
		c.bindErrs = c.bindErrs[1:]
	}
	return &kernel.OperationResult[kernel.WriteBackResult]{
		Level:   kernel.EvidenceConfirmed,
		Success: err == nil,
	}, err
}

type fakePortalClient struct {
	results []*kernel.OperationResult[kernel.Portal802Response]
	errs    []error
}

func (c *fakePortalClient) Login802(ctx context.Context, account, password, ip, isp string) (*kernel.OperationResult[kernel.Portal802Response], error) {
	_ = ctx
	_ = account
	_ = password
	_ = ip
	_ = isp
	if len(c.results) == 0 {
		return nil, errors.New("no portal result queued")
	}
	result := c.results[0]
	c.results = c.results[1:]
	var err error
	if len(c.errs) > 0 {
		err = c.errs[0]
		c.errs = c.errs[1:]
	}
	return result, err
}

type fakeFactory struct {
	selfClients   []GuardSelfClient
	portalClients []GuardPortalClient
}

func (f *fakeFactory) NewSelf() (GuardSelfClient, error) {
	if len(f.selfClients) == 0 {
		return nil, errors.New("no self client queued")
	}
	client := f.selfClients[0]
	f.selfClients = f.selfClients[1:]
	return client, nil
}

func (f *fakeFactory) NewPortal() (GuardPortalClient, error) {
	if len(f.portalClients) == 0 {
		return nil, errors.New("no portal client queued")
	}
	client := f.portalClients[0]
	f.portalClients = f.portalClients[1:]
	return client, nil
}

func baseGuardEnv(factory GuardClientFactory, prober GuardProber) GuardEnvironment {
	return GuardEnvironment{
		Accounts: map[string]config.AccountConfig{
			"W": {Username: "w-user", Password: "w-pass"},
			"B": {Username: "b-user", Password: "b-pass"},
		},
		Broadband: config.BroadbandConfig{
			Account:  "cmcc-user",
			Password: "cmcc-pass",
		},
		PortalISP: "mobile",
		Factory:   factory,
		Prober:    prober,
	}
}

func TestRepairBindingAlreadyCorrect(t *testing.T) {
	factory := &fakeFactory{
		selfClients: []GuardSelfClient{
			&fakeSelfClient{binding: &kernel.OperatorBinding{MobileAccount: "cmcc-user", MobilePassword: "cmcc-pass"}},
		},
	}

	result, err := RepairBinding(context.Background(), baseGuardEnv(factory, &fakeProber{}), "W")
	if err != nil {
		t.Fatalf("repair binding: %v", err)
	}
	if !result.Success || result.Data == nil || result.Data.Action != "already-correct" {
		t.Fatalf("unexpected repair result: %#v", result)
	}
}

func TestRepairBindingMovesFromHolder(t *testing.T) {
	target := &fakeSelfClient{binding: &kernel.OperatorBinding{}}
	holder := &fakeSelfClient{binding: &kernel.OperatorBinding{MobileAccount: "cmcc-user"}}
	factory := &fakeFactory{
		selfClients: []GuardSelfClient{target, holder},
	}

	result, err := RepairBinding(context.Background(), baseGuardEnv(factory, &fakeProber{}), "W")
	if err != nil {
		t.Fatalf("repair binding: %v", err)
	}
	if !result.Success || result.Data == nil || result.Data.Action != "moved" || result.Data.HolderProfile != "B" {
		t.Fatalf("unexpected repair result: %#v", result)
	}
	if len(holder.bindTargets) != 1 || holder.bindTargets[0]["FLDEXTRA3"] != "" {
		t.Fatalf("expected holder clear, got %#v", holder.bindTargets)
	}
	if len(target.bindTargets) != 1 || target.bindTargets[0]["FLDEXTRA3"] != "cmcc-user" {
		t.Fatalf("expected target bind, got %#v", target.bindTargets)
	}
}

func TestGuardCycleHealthyNoLogin(t *testing.T) {
	prober := &fakeProber{
		checks: []probeResult{{ok: true, message: "internet ok"}},
	}
	factory := &fakeFactory{}

	result, err := GuardCycle(context.Background(), baseGuardEnv(factory, prober), GuardCycleInput{
		DesiredProfile: "B",
		ScheduleWindow: "weekday-day",
	})
	if err != nil {
		t.Fatalf("guard cycle: %v", err)
	}
	if !result.InternetOK || !result.PortalLoginOK || result.RecoveryStep != "healthy" {
		t.Fatalf("unexpected healthy cycle: %#v", result)
	}
}

func TestGuardCycleForceSwitchImmediatelyRestoresTarget(t *testing.T) {
	target := &fakeSelfClient{binding: &kernel.OperatorBinding{MobileAccount: "cmcc-user", MobilePassword: "cmcc-pass"}}
	portalClient := &fakePortalClient{
		results: []*kernel.OperationResult[kernel.Portal802Response]{
			{Level: kernel.EvidenceConfirmed, Success: true, Message: "portal login ok", Data: &kernel.Portal802Response{RetCode: "0"}},
		},
		errs: []error{nil},
	}
	prober := &fakeProber{
		ips:    []string{"10.1.2.3"},
		checks: []probeResult{{ok: true, message: "internet ok after portal"}},
	}
	factory := &fakeFactory{
		selfClients:   []GuardSelfClient{target},
		portalClients: []GuardPortalClient{portalClient},
	}

	result, err := GuardCycle(context.Background(), baseGuardEnv(factory, prober), GuardCycleInput{
		DesiredProfile: "B",
		ScheduleWindow: "weekday-day",
		ForceSwitch:    true,
	})
	if err != nil {
		t.Fatalf("force switch cycle: %v", err)
	}
	if !result.PortalLoginOK || !result.InternetOK || result.RecoveryStep != "portal-login" {
		t.Fatalf("unexpected force-switch cycle: %#v", result)
	}
}

func TestGuardCyclePortalFailureTriggersRepairAndRetry(t *testing.T) {
	target := &fakeSelfClient{binding: &kernel.OperatorBinding{}}
	portalClient := &fakePortalClient{
		results: []*kernel.OperationResult[kernel.Portal802Response]{
			{Level: kernel.EvidenceGuarded, Success: false, Message: "portal login failed"},
			{Level: kernel.EvidenceConfirmed, Success: true, Message: "portal login ok"},
		},
		errs: []error{errors.New("first login failed"), nil},
	}
	prober := &fakeProber{
		checks: []probeResult{
			{ok: false, message: "offline"},
			{ok: true, message: "internet ok after retry"},
		},
		ips: []string{"10.1.2.3", "10.1.2.3"},
	}
	factory := &fakeFactory{
		selfClients:   []GuardSelfClient{target},
		portalClients: []GuardPortalClient{portalClient},
	}

	result, err := GuardCycle(context.Background(), baseGuardEnv(factory, prober), GuardCycleInput{
		DesiredProfile: "W",
		ScheduleWindow: "weekday-night",
	})
	if err != nil {
		t.Fatalf("portal failure cycle: %v", err)
	}
	if !result.PortalLoginOK || !result.InternetOK || result.RecoveryStep != "binding-repair-then-portal-login" {
		t.Fatalf("unexpected retry cycle: %#v", result)
	}
	if result.BindingRepair == nil || result.BindingRepair.Action != "attached" {
		t.Fatalf("expected binding repair evidence: %#v", result.BindingRepair)
	}
}

func TestGuardCyclePortalSuccessButStillOfflineRepairsThenRetries(t *testing.T) {
	target := &fakeSelfClient{binding: &kernel.OperatorBinding{}}
	portalClient := &fakePortalClient{
		results: []*kernel.OperationResult[kernel.Portal802Response]{
			{Level: kernel.EvidenceConfirmed, Success: true, Message: "portal login ok"},
			{Level: kernel.EvidenceConfirmed, Success: true, Message: "portal login ok second"},
		},
		errs: []error{nil, nil},
	}
	prober := &fakeProber{
		checks: []probeResult{
			{ok: false, message: "offline"},
			{ok: false, message: "still offline"},
			{ok: true, message: "internet restored"},
		},
		ips: []string{"10.1.2.3", "10.1.2.3"},
	}
	factory := &fakeFactory{
		selfClients:   []GuardSelfClient{target},
		portalClients: []GuardPortalClient{portalClient},
	}

	result, err := GuardCycle(context.Background(), baseGuardEnv(factory, prober), GuardCycleInput{
		DesiredProfile: "W",
		ScheduleWindow: "weekday-night",
	})
	if err != nil {
		t.Fatalf("portal success but offline cycle: %v", err)
	}
	if !result.PortalLoginOK || !result.InternetOK {
		t.Fatalf("expected recovered internet: %#v", result)
	}
}

func TestGuardCycleBindingRepairFailureDegradesButContinues(t *testing.T) {
	target := &fakeSelfClient{
		binding:  &kernel.OperatorBinding{},
		bindErrs: []error{errors.New("bind failed")},
	}
	prober := &fakeProber{
		checks: []probeResult{{ok: true, message: "internet ok"}},
	}
	factory := &fakeFactory{
		selfClients: []GuardSelfClient{target},
	}

	result, err := GuardCycle(context.Background(), baseGuardEnv(factory, prober), GuardCycleInput{
		DesiredProfile:    "W",
		ScheduleWindow:    "weekday-night",
		ForceBindingCheck: true,
	})
	if err != nil {
		t.Fatalf("expected degraded cycle without fatal error, got %v", err)
	}
	if result.BindingOK || !result.InternetOK || result.RecoveryStep != "degraded-binding-repair" {
		t.Fatalf("unexpected degraded cycle: %#v", result)
	}
}
