package guard

// StartupEventDetails captures startup-specific machine details.
type StartupEventDetails struct {
	StateDir string `json:"stateDir,omitempty"`
}

// ScheduleSwitchEventDetails captures one completed schedule switch.
type ScheduleSwitchEventDetails struct {
	BindingOK     bool   `json:"bindingOk"`
	InternetOK    bool   `json:"internetOk"`
	PortalLoginOK bool   `json:"portalLoginOk"`
	RecoveryStep  string `json:"recoveryStep,omitempty"`
}

// BindingAuditEventDetails captures one binding audit outcome.
type BindingAuditEventDetails struct {
	BindingOK    bool   `json:"bindingOk"`
	RecoveryStep string `json:"recoveryStep,omitempty"`
}

// PortalLoginEventDetails captures one Portal attempt outcome.
type PortalLoginEventDetails struct {
	InternetOK    bool   `json:"internetOk"`
	PortalLoginOK bool   `json:"portalLoginOk"`
	RecoveryStep  string `json:"recoveryStep,omitempty"`
}

// BindingRepairEventDetails captures one binding repair action.
type BindingRepairEventDetails struct {
	Action        string `json:"action,omitempty"`
	BindingOK     bool   `json:"bindingOk"`
	HolderProfile string `json:"holderProfile,omitempty"`
	RecoveryStep  string `json:"recoveryStep,omitempty"`
	TargetProfile string `json:"targetProfile,omitempty"`
}

// DegradedEventDetails captures why one cycle ended degraded.
type DegradedEventDetails struct {
	BindingOK     bool   `json:"bindingOk"`
	Error         string `json:"error,omitempty"`
	InternetOK    bool   `json:"internetOk"`
	PortalLoginOK bool   `json:"portalLoginOk"`
	RecoveryStep  string `json:"recoveryStep,omitempty"`
}

// ShutdownEventDetails captures graceful shutdown context.
type ShutdownEventDetails struct {
	Reason string `json:"reason,omitempty"`
}

// FatalEventDetails captures runtime-level fatal context.
type FatalEventDetails struct {
	Error string `json:"error,omitempty"`
}

// NormalizeEvent upgrades legacy map details into the supported typed event payloads.
func NormalizeEvent(event Event) Event {
	switch event.Kind {
	case EventStartup:
		event.Details = normalizeStartupEventDetails(event.Details)
	case EventScheduleSwitch:
		event.Details = normalizeScheduleSwitchEventDetails(event.Details)
	case EventBindingAudit:
		event.Details = normalizeBindingAuditEventDetails(event.Details)
	case EventPortalLogin:
		event.Details = normalizePortalLoginEventDetails(event.Details)
	case EventBindingRepair:
		event.Details = normalizeBindingRepairEventDetails(event.Details)
	case EventDegraded:
		event.Details = normalizeDegradedEventDetails(event.Details)
	case EventShutdown:
		event.Details = normalizeShutdownEventDetails(event.Details)
	case EventFatal:
		event.Details = normalizeFatalEventDetails(event.Details)
	}
	return event
}

func normalizeStartupEventDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case StartupEventDetails:
		return value
	case *StartupEventDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return StartupEventDetails{StateDir: value["stateDir"]}
	default:
		return nil
	}
}

func normalizeScheduleSwitchEventDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case ScheduleSwitchEventDetails:
		return value
	case *ScheduleSwitchEventDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return ScheduleSwitchEventDetails{
			BindingOK:     value["bindingOk"] == "true",
			InternetOK:    value["internetOk"] == "true",
			PortalLoginOK: value["portalLoginOk"] == "true",
			RecoveryStep:  value["recoveryStep"],
		}
	default:
		return nil
	}
}

func normalizeBindingAuditEventDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case BindingAuditEventDetails:
		return value
	case *BindingAuditEventDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return BindingAuditEventDetails{
			BindingOK:    value["bindingOk"] == "true",
			RecoveryStep: value["recoveryStep"],
		}
	default:
		return nil
	}
}

func normalizePortalLoginEventDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case PortalLoginEventDetails:
		return value
	case *PortalLoginEventDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return PortalLoginEventDetails{
			InternetOK:    value["internetOk"] == "true",
			PortalLoginOK: value["portalLoginOk"] == "true",
			RecoveryStep:  value["recoveryStep"],
		}
	default:
		return nil
	}
}

func normalizeBindingRepairEventDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case BindingRepairEventDetails:
		return value
	case *BindingRepairEventDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return BindingRepairEventDetails{
			Action:        value["action"],
			BindingOK:     value["bindingOk"] == "true",
			HolderProfile: value["holderProfile"],
			RecoveryStep:  value["recoveryStep"],
			TargetProfile: value["targetProfile"],
		}
	default:
		return nil
	}
}

func normalizeDegradedEventDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case DegradedEventDetails:
		return value
	case *DegradedEventDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return DegradedEventDetails{
			BindingOK:     value["bindingOk"] == "true",
			Error:         value["error"],
			InternetOK:    value["internetOk"] == "true",
			PortalLoginOK: value["portalLoginOk"] == "true",
			RecoveryStep:  value["recoveryStep"],
		}
	default:
		return nil
	}
}

func normalizeShutdownEventDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case ShutdownEventDetails:
		return value
	case *ShutdownEventDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return ShutdownEventDetails{Reason: value["reason"]}
	default:
		return nil
	}
}

func normalizeFatalEventDetails(details any) any {
	switch value := details.(type) {
	case nil:
		return nil
	case FatalEventDetails:
		return value
	case *FatalEventDetails:
		if value == nil {
			return nil
		}
		return *value
	case map[string]string:
		return FatalEventDetails{Error: value["error"]}
	default:
		return nil
	}
}
