package zfw

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

func (s *Session) operatorForm(ctx context.Context) (*html.Node, url.Values, error) {
	doc, err := s.page(ctx, "service/operatorId")
	if err != nil {
		return nil, nil, err
	}
	return parseOperatorForm(doc)
}

func parseOperatorForm(doc *html.Node) (*html.Node, url.Values, error) {
	form := findForm(doc, "/Self/service/bind-operator")
	if form == nil {
		return nil, nil, fmt.Errorf("Self operator binding form is missing")
	}
	values := formValues(form)
	for _, field := range []string{"csrftoken", "FLDEXTRA1", "FLDEXTRA2", "FLDEXTRA3", "FLDEXTRA4"} {
		if _, exists := values[field]; !exists {
			return nil, nil, fmt.Errorf("Self operator binding form is missing %s", field)
		}
	}
	if values.Get("csrftoken") == "" {
		return nil, nil, fmt.Errorf("Self operator binding form has an empty CSRF token")
	}
	return form, values, nil
}

type OperatorAccount struct {
	Account     string `json:"account"`
	PasswordSet bool   `json:"password_set"`
}

type OperatorBindings struct {
	NJXY OperatorAccount `json:"njxy"`
	CMCC OperatorAccount `json:"cmcc"`
}

type OperatorBindingResult struct {
	Bindings *OperatorBindings `json:"bindings,omitempty"`
	Operator string            `json:"operator"`
	Account  string            `json:"account"`
	Outcome  Outcome           `json:"outcome"`
	Message  string            `json:"message"`
	Verified bool              `json:"verified"`
}

func operatorAccounts(values url.Values) *OperatorBindings {
	return &OperatorBindings{
		NJXY: OperatorAccount{Account: values.Get("FLDEXTRA1"), PasswordSet: values.Get("FLDEXTRA2") != ""},
		CMCC: OperatorAccount{Account: values.Get("FLDEXTRA3"), PasswordSet: values.Get("FLDEXTRA4") != ""},
	}
}
func (s *Session) OperatorBinding(ctx context.Context) (*OperatorBindings, error) {
	_, values, err := s.operatorForm(ctx)
	if err != nil {
		return nil, err
	}
	return operatorAccounts(values), nil
}

func (s *Session) BindOperator(ctx context.Context, operator, account, password string) (*OperatorBindingResult, error) {
	if operator != "njxy" && operator != "cmcc" {
		return nil, fmt.Errorf("operator binding requires njxy or cmcc")
	}
	if account == "" || password == "" {
		return nil, fmt.Errorf("operator binding requires a broadband account and password")
	}
	return s.setOperator(ctx, operator, account, password)
}

func (s *Session) UnbindOperator(ctx context.Context, operator string) (*OperatorBindingResult, error) {
	if operator != "njxy" && operator != "cmcc" {
		return nil, fmt.Errorf("operator unbinding requires njxy or cmcc")
	}
	return s.setOperator(ctx, operator, "", "")
}

func (s *Session) setOperator(ctx context.Context, operator, account, password string) (*OperatorBindingResult, error) {
	result := &OperatorBindingResult{Operator: operator, Account: account, Outcome: NotSubmitted}
	form, values, err := s.operatorForm(ctx)
	if err != nil {
		return result, err
	}
	accountField, passwordField := "FLDEXTRA1", "FLDEXTRA2"
	if operator == "cmcc" {
		accountField, passwordField = "FLDEXTRA3", "FLDEXTRA4"
	}
	result.Bindings = operatorAccounts(values)
	if account == "" && password == "" && values.Get(accountField) == "" && values.Get(passwordField) == "" {
		result.Verified = true
		return result, nil
	}
	passwords := []string{values.Get("FLDEXTRA2"), values.Get("FLDEXTRA4"), password}
	values.Set(accountField, account)
	values.Set(passwordField, password)
	result.Outcome = Unknown
	data, final, _, err := s.navigate(ctx, http.MethodPost, attr(form, "action"), values, "/Self/service/operatorId")
	if err != nil {
		return result, fmt.Errorf("operator binding submission failed; result is unknown: %w", err)
	}
	doc, err := s.authenticatedPage(data, final)
	if err != nil {
		return result, err
	}
	message, state, err := formFeedback(doc)
	if err != nil {
		return result, err
	}
	for _, secret := range passwords {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[password]")
		}
	}
	result.Message = message
	switch state {
	case "true":
		result.Outcome = Accepted
	case "false":
		result.Outcome = Rejected
		return result, fmt.Errorf("Self rejected the operator binding submission")
	default:
		return result, fmt.Errorf("Self operator result has no recognized acceptance state")
	}
	var actual url.Values
	if pathWithoutSession(final.Path) == "/Self/service/operatorId" {
		_, actual, err = parseOperatorForm(doc)
	} else {
		_, actual, err = s.operatorForm(ctx)
	}
	if err != nil {
		return result, fmt.Errorf("operator binding was accepted; final state is unverified: %w", err)
	}
	result.Bindings = operatorAccounts(actual)
	if actual.Get(accountField) != account {
		return result, fmt.Errorf("Self operator state does not contain the requested broadband account")
	}
	if actual.Get(passwordField) != password {
		return result, fmt.Errorf("Self operator state does not contain the submitted broadband password")
	}
	otherAccount, otherPassword := "FLDEXTRA3", "FLDEXTRA4"
	if operator == "cmcc" {
		otherAccount, otherPassword = "FLDEXTRA1", "FLDEXTRA2"
	}
	if actual.Get(otherAccount) != values.Get(otherAccount) || actual.Get(otherPassword) != values.Get(otherPassword) {
		return result, fmt.Errorf("Self operator result changed the unselected operator's fields")
	}
	result.Verified = true
	return result, nil
}
