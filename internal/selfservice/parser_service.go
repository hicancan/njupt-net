package selfservice

import (
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/hicancan/njupt-net-cli/internal/kernel"
)

func parseOperatorState(doc *goquery.Document) (string, map[string]string) {
	token := extractInputValue(doc, "csrftoken")
	state := map[string]string{}
	for _, field := range bindFields {
		state[field] = extractInputValue(doc, field)
	}
	return token, state
}

func operatorBindingFromState(state map[string]string) kernel.OperatorBinding {
	return kernel.OperatorBinding{
		TelecomAccount:  state["FLDEXTRA1"],
		TelecomPassword: state["FLDEXTRA2"],
		MobileAccount:   state["FLDEXTRA3"],
		MobilePassword:  state["FLDEXTRA4"],
	}
}

func parseConsumeProtectState(doc *goquery.Document, body string) *kernel.ConsumeProtectState {
	match := installmentFlagPattern.FindStringSubmatch(body)
	installment := ""
	if len(match) > 1 {
		installment = strings.TrimSpace(match[1])
	}
	return &kernel.ConsumeProtectState{
		CSRFTOKEN:       extractInputValue(doc, "csrftoken"),
		InstallmentFlag: installment,
		CurrentLimit:    firstAttrOrText(doc, "input[name='consumeLimit']", "#consumeLimit", ".consume-limit", ".currentLimit"),
		CurrentUsage:    firstAttrOrText(doc, "#currentUsage", ".currentUsage", ".useFlow"),
		Balance:         firstAttrOrText(doc, "#balance", ".balance", ".remainBalance"),
	}
}
