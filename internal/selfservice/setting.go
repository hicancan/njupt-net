package selfservice

import (
	"context"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// GetPerson returns a sanitized projection of person-list state.
func (c *Client) GetPerson(ctx context.Context) (*kernel.OperationResult[kernel.PersonState], error) {
	if err := c.ensureSession("setting.person.get"); err != nil {
		return nil, err
	}
	doc, resp, err := c.readDocument(ctx, personListPath, kernel.RequestOptions{}, "setting.person.get")
	if err != nil {
		return nil, err
	}
	if responseLooksLikeLogin(resp) {
		return nil, &kernel.OpError{Op: "setting.person.get", Message: "personList returned login page", Err: kernel.ErrAuth}
	}
	fields := sanitizeSensitiveFields(extractInputFields(doc))
	for key, value := range extractWindowUserFields(resp.Body) {
		fields[key] = value
	}
	state := &kernel.PersonState{
		CSRFTOKEN: extractInputValue(doc, "csrftoken"),
		Fields:    fields,
	}
	return &kernel.OperationResult[kernel.PersonState]{
		Level:   kernel.EvidenceConfirmed,
		Success: true,
		Message: "personList loaded with sanitized state",
		Data:    state,
	}, nil
}

// UpdateUserSecurity is intentionally guarded/blocked for success semantics.
func (c *Client) UpdateUserSecurity(ctx context.Context, form map[string]string, dryRun bool) (*kernel.OperationResult[kernel.PersonState], error) {
	state, err := c.GetPerson(ctx)
	if err != nil {
		return nil, err
	}
	if state.Data == nil {
		return nil, &kernel.OpError{Op: "setting.person.update", Message: "person state unavailable", Err: kernel.ErrBlockedCapability, ProblemDetails: kernel.CapabilityProblemDetails{Capability: "setting.person.update", Reason: "person state unavailable"}}
	}
	if dryRun {
		return &kernel.OperationResult[kernel.PersonState]{
			Level:   kernel.EvidenceBlocked,
			Success: false,
			Message: "dry-run only; page exposes no stable writable fields and success semantics remain blocked",
			Data:    state.Data,
		}, nil
	}

	payload := map[string]string{"csrftoken": state.Data.CSRFTOKEN}
	for k, v := range form {
		payload[k] = v
	}
	_, err = c.session.PostForm(ctx, updateUserSecurityPath, kernel.RequestOptions{Form: payload})
	if err != nil {
		return nil, &kernel.OpError{Op: "setting.person.update", Message: "submit failed", Err: err}
	}

	nextState := &kernel.PersonState{
		CSRFTOKEN: state.Data.CSRFTOKEN,
		Fields:    sanitizeSensitiveFields(state.Data.Fields),
	}
	return &kernel.OperationResult[kernel.PersonState]{
		Level:   kernel.EvidenceBlocked,
		Success: false,
		Message: "request submitted, but page exposes no stable writable fields and success semantics remain blocked",
		Data:    nextState,
	}, &kernel.OpError{Op: "setting.person.update", Message: "success path is blocked because personList only exposes csrftoken and submit-without-fields returns Not valid!", Err: kernel.ErrBlockedCapability, ProblemDetails: kernel.CapabilityProblemDetails{Capability: "setting.person.update", Reason: "page exposes no stable writable fields"}}
}
