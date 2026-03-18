package selfservice

import (
	"context"

	"github.com/PuerkitoBio/goquery"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

// GetPerson returns guarded person-list diagnostics and form fields.
func (c *Client) GetPerson(ctx context.Context) (*kernel.OperationResult[kernel.PersonState], error) {
	if err := c.ensureSession("setting.person.get"); err != nil {
		return nil, err
	}
	doc, resp, err := c.readDocument(ctx, personListPath, kernel.RequestOptions{}, "setting.person.get")
	if err != nil {
		return nil, err
	}
	if looksLikeLoginPage(resp.Body) {
		return nil, &kernel.OpError{Op: "setting.person.get", Message: "personList returned login page", Err: kernel.ErrAuth}
	}
	state := &kernel.PersonState{
		CSRFTOKEN: extractInputValue(doc, "csrftoken"),
		Fields:    extractInputFields(doc),
		RawHTML:   string(resp.Body),
	}
	return &kernel.OperationResult[kernel.PersonState]{
		Level:   kernel.EvidenceGuarded,
		Success: true,
		Message: "personList loaded as guarded capability",
		Data:    state,
		Raw:     rawCapture(resp),
	}, nil
}

// UpdateUserSecurity is intentionally guarded/blocked for success semantics.
func (c *Client) UpdateUserSecurity(ctx context.Context, form map[string]string, dryRun bool) (*kernel.OperationResult[kernel.PersonState], error) {
	state, err := c.GetPerson(ctx)
	if err != nil {
		return nil, err
	}
	if state.Data == nil {
		return nil, &kernel.OpError{Op: "setting.person.update", Message: "person state unavailable", Err: kernel.ErrBlockedCapability}
	}
	if dryRun {
		return &kernel.OperationResult[kernel.PersonState]{
			Level:   kernel.EvidenceBlocked,
			Success: false,
			Message: "dry-run only; real success semantics remain blocked",
			Data:    state.Data,
		}, nil
	}

	payload := map[string]string{"csrftoken": state.Data.CSRFTOKEN}
	for k, v := range form {
		payload[k] = v
	}
	resp, err := c.session.PostForm(ctx, updateUserSecurityPath, kernel.RequestOptions{Form: payload})
	if err != nil {
		return nil, &kernel.OpError{Op: "setting.person.update", Message: "submit failed", Err: err}
	}

	nextState := &kernel.PersonState{
		CSRFTOKEN: state.Data.CSRFTOKEN,
		Fields:    state.Data.Fields,
		RawHTML:   string(resp.Body),
	}
	return &kernel.OperationResult[kernel.PersonState]{
		Level:   kernel.EvidenceBlocked,
		Success: false,
		Message: "request submitted, but success semantics remain blocked by SSOT",
		Data:    nextState,
		Raw:     rawCapture(resp),
	}, &kernel.OpError{Op: "setting.person.update", Message: "success path is blocked by SSOT", Err: kernel.ErrBlockedCapability}
}

func extractInputFields(doc *goquery.Document) map[string]string {
	fields := map[string]string{}
	doc.Find("input[name]").Each(func(_ int, selection *goquery.Selection) {
		name, ok := selection.Attr("name")
		if !ok || name == "" {
			return
		}
		value, _ := selection.Attr("value")
		fields[name] = value
	})
	return fields
}
