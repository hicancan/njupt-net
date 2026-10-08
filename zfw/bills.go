package zfw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// BillQuery describes the filters of one of Self's three bill tables.
// Empty pagination, sort and date fields use that table's displayed defaults.
// Year applies only to monthly bills; Start and End apply to the other tables.
type BillQuery struct {
	Kind        string
	Start, End  string
	Year        int
	Page, Size  int
	Sort, Order string
}

// BillPage contains the typed table selected by Kind.
type BillPage struct {
	Kind       string          `json:"kind"`
	Online     *OnlineBills    `json:"online,omitempty"`
	Monthly    *MonthlyBills   `json:"monthly,omitempty"`
	Operations *OperationBills `json:"operations,omitempty"`
}

type OnlineBills struct {
	Total   int           `json:"total"`
	Rows    []OnlineBill  `json:"rows"`
	Summary OnlineSummary `json:"summary"`
}

type OnlineSummary struct {
	Count            *json.Number `json:"count,omitempty"`
	InternetUpFlow   *json.Number `json:"internet_up_flow,omitempty"`
	InternetDownFlow *json.Number `json:"internet_down_flow,omitempty"`
	ChinanetUpFlow   *json.Number `json:"chinanet_up_flow,omitempty"`
	ChinanetDownFlow *json.Number `json:"chinanet_down_flow,omitempty"`
	CostMoney        *json.Number `json:"cost_money,omitempty"`
	Flow             *json.Number `json:"flow,omitempty"`
	Time             *json.Number `json:"time,omitempty"`
}

type onlineSummaryWire struct {
	Count            *json.Number `json:"COU,omitempty"`
	InternetUpFlow   *json.Number `json:"INTERNETUPFLOW,omitempty"`
	InternetDownFlow *json.Number `json:"INTERNETDOWNFLOW,omitempty"`
	ChinanetUpFlow   *json.Number `json:"CHINANETUPFLOW,omitempty"`
	ChinanetDownFlow *json.Number `json:"CHINANETDOWNFLOW,omitempty"`
	CostMoney        *json.Number `json:"COSTMONEY,omitempty"`
	Flow             *json.Number `json:"FLOW,omitempty"`
	Time             *json.Number `json:"TIME,omitempty"`
}

func (value *onlineSummaryWire) UnmarshalJSON(data []byte) error {
	type plain onlineSummaryWire
	var decoded plain
	if err := decodeObject(data, &decoded, "COU"); err != nil {
		return err
	}
	*value = onlineSummaryWire(decoded)
	return nil
}

type OnlineBill struct {
	Area             int64       `json:"area"`
	ChinanetDownFlow json.Number `json:"chinanet_down_flow"`
	ChinanetUpFlow   json.Number `json:"chinanet_up_flow"`
	CostID           int64       `json:"cost_id"`
	CostMoney        json.Number `json:"cost_money"`
	CostStyleID      int64       `json:"cost_style_id"`
	Extend           string      `json:"extend"`
	Flow             json.Number `json:"flow"`
	InternetDownFlow json.Number `json:"internet_down_flow"`
	InternetUpFlow   json.Number `json:"internet_up_flow"`
	LoginTime        int64       `json:"login_time"`
	LogoutTime       int64       `json:"logout_time"`
	MACAddress       string      `json:"mac_address"`
	MainControlID    int64       `json:"main_control_id"`
	MultiGroupID     int64       `json:"multi_group_id"`
	NASIP            string      `json:"nas_ip"`
	NASPort          int64       `json:"nas_port"`
	OtherFlow        json.Number `json:"other_flow"`
	Time             json.Number `json:"time"`
	UserGroupID      int64       `json:"user_group_id"`
	UserID           int64       `json:"user_id"`
	UserIP           string      `json:"user_ip"`
	UserName         string      `json:"user_name"`
	UserPhone        string      `json:"user_phone"`
	UserRealName     string      `json:"user_real_name"`
}

type onlineBillWire struct {
	Area             int64       `json:"area"`
	ChinanetDownFlow json.Number `json:"chinanetDownFlow"`
	ChinanetUpFlow   json.Number `json:"chinanetUpFlow"`
	CostID           int64       `json:"costId"`
	CostMoney        json.Number `json:"costMoney"`
	CostStyleID      int64       `json:"costStyleId"`
	Extend           string      `json:"extend"`
	Flow             json.Number `json:"flow"`
	InternetDownFlow json.Number `json:"internetDownFlow"`
	InternetUpFlow   json.Number `json:"internetUpFlow"`
	LoginTime        int64       `json:"loginTime"`
	LogoutTime       int64       `json:"logoutTime"`
	MACAddress       string      `json:"macAddress"`
	MainControlID    int64       `json:"mainControlId"`
	MultiGroupID     int64       `json:"mutliGroupId"`
	NASIP            string      `json:"nasIp"`
	NASPort          int64       `json:"nasPort"`
	OtherFlow        json.Number `json:"otherFlow"`
	Time             json.Number `json:"time"`
	UserGroupID      int64       `json:"userGroupId"`
	UserID           int64       `json:"userId"`
	UserIP           string      `json:"userIp"`
	UserName         string      `json:"userName"`
	UserPhone        string      `json:"userPhone"`
	UserRealName     string      `json:"userRealName"`
}

func (value *onlineBillWire) UnmarshalJSON(data []byte) error {
	type plain onlineBillWire
	var decoded plain
	if err := decodeObject(data, &decoded, "area", "chinanetDownFlow", "chinanetUpFlow", "costId", "costMoney", "costStyleId", "extend", "flow", "internetDownFlow", "internetUpFlow", "loginTime", "logoutTime", "macAddress", "mainControlId", "mutliGroupId", "nasIp", "nasPort", "otherFlow", "time", "userGroupId", "userId", "userIp", "userName", "userPhone", "userRealName"); err != nil {
		return err
	}
	*value = onlineBillWire(decoded)
	return nil
}

type MonthlyBills struct {
	Total   int            `json:"total"`
	Rows    []MonthlyBill  `json:"rows"`
	Summary MonthlySummary `json:"summary"`
}

type MonthlySummary struct {
	UseTime   json.Number `json:"use_time"`
	BaseMoney json.Number `json:"base_money"`
	Flow      json.Number `json:"flow"`
	UsedMoney json.Number `json:"used_money"`
}

type monthlySummaryWire struct {
	UseTime   json.Number `json:"USETIME"`
	BaseMoney json.Number `json:"USEBASEMONEY"`
	Flow      json.Number `json:"USEFLOW"`
	UsedMoney json.Number `json:"USEDMONEY"`
}

func (value *monthlySummaryWire) UnmarshalJSON(data []byte) error {
	type plain monthlySummaryWire
	var decoded plain
	if err := decodeObject(data, &decoded, "USETIME", "USEBASEMONEY", "USEFLOW", "USEDMONEY"); err != nil {
		return err
	}
	*value = monthlySummaryWire(decoded)
	return nil
}

type MonthlyBill struct {
	StartTime json.Number `json:"start_time"`
	EndTime   json.Number `json:"end_time"`
	Plan      string      `json:"plan"`
	BaseMoney json.Number `json:"base_money"`
	UsedMoney json.Number `json:"used_money"`
	Duration  json.Number `json:"duration"`
	Flow      json.Number `json:"flow"`
	BillTime  json.Number `json:"bill_time"`
}

type monthlyBillWire MonthlyBill

func (b *monthlyBillWire) UnmarshalJSON(data []byte) error {
	return decodeColumns(data, &b.StartTime, &b.EndTime, &b.Plan, &b.BaseMoney, &b.UsedMoney, &b.Duration, &b.Flow, &b.BillTime)
}

type OperationBills struct {
	Total int             `json:"total"`
	Rows  []OperationBill `json:"rows"`
}

type OperationBill struct {
	Time        json.Number `json:"time"`
	Description string      `json:"description"`
	Terminal    string      `json:"terminal"`
	// Column 3 is returned by the server but absent from the public table.
	ServerField3 json.Number `json:"server_field_3"`
	Remark       string      `json:"remark"`
}

type operationBillWire OperationBill

func (b *operationBillWire) UnmarshalJSON(data []byte) error {
	return decodeColumns(data, &b.Time, &b.Description, &b.Terminal, &b.ServerField3, &b.Remark)
}

// Validate checks filters without contacting the school.
func (q BillQuery) Validate() error {
	_, err := normalizeBill(q, time.Now())
	return err
}

type billTable struct {
	kind, query, export string
}

func billTableFor(kind string) (billTable, error) {
	switch kind {
	case "online":
		return billTable{kind, "/Self/bill/getUserOnlineLog", "/Self/bill/exportUserOnlineLog"}, nil
	case "monthly":
		return billTable{kind, "/Self/bill/getMonthPay", "/Self/bill/exportMonthPay"}, nil
	case "operations":
		return billTable{kind, "/Self/bill/getOperatorLog", "/Self/bill/exportOperatorLog"}, nil
	default:
		return billTable{}, fmt.Errorf("bill kind must be online, monthly or operations")
	}
}

// Bills converts the deployment's object and positional tables into typed data.
func (s *Session) Bills(ctx context.Context, q BillQuery) (*BillPage, error) {
	table, params, err := s.prepareBill(ctx, q)
	if err != nil {
		return nil, err
	}
	return s.queryBill(ctx, table, params)
}

func (s *Session) queryBill(ctx context.Context, table billTable, params url.Values) (*BillPage, error) {
	var wire struct {
		Total   *int            `json:"total"`
		Rows    json.RawMessage `json:"rows"`
		Summary json.RawMessage `json:"summary"`
	}
	if err := s.json(ctx, table.query, params, &wire); err != nil {
		return nil, err
	}
	if wire.Total == nil || *wire.Total < 0 {
		return nil, fmt.Errorf("%s: expected bill table total", table.query)
	}
	if rows := bytes.TrimSpace(wire.Rows); len(rows) == 0 || rows[0] != '[' {
		return nil, fmt.Errorf("%s: expected bill table rows array", table.query)
	}
	result := &BillPage{}
	switch table.kind {
	case "online":
		result.Kind = "online"
		result.Online = &OnlineBills{Total: *wire.Total}
		var rows []onlineBillWire
		if err := json.Unmarshal(wire.Rows, &rows); err != nil {
			return nil, fmt.Errorf("online bill rows: %w", err)
		}
		result.Online.Rows = make([]OnlineBill, len(rows))
		for index, row := range rows {
			result.Online.Rows[index] = OnlineBill(row)
		}
		if len(wire.Summary) == 0 || bytes.Equal(wire.Summary, []byte("null")) {
			return nil, fmt.Errorf("online bill summary is missing")
		}
		var summary onlineSummaryWire
		if err := json.Unmarshal(wire.Summary, &summary); err != nil {
			return nil, fmt.Errorf("online bill summary: %w", err)
		}
		result.Online.Summary = OnlineSummary(summary)
		if result.Online.Summary.Count == nil {
			return nil, fmt.Errorf("online bill summary is missing its count")
		}
	case "monthly":
		result.Kind = "monthly"
		result.Monthly = &MonthlyBills{Total: *wire.Total}
		var rows []monthlyBillWire
		if err := json.Unmarshal(wire.Rows, &rows); err != nil {
			return nil, fmt.Errorf("monthly bill rows: %w", err)
		}
		result.Monthly.Rows = make([]MonthlyBill, len(rows))
		for index, row := range rows {
			result.Monthly.Rows[index] = MonthlyBill(row)
		}
		if len(wire.Summary) == 0 || bytes.Equal(wire.Summary, []byte("null")) {
			return nil, fmt.Errorf("monthly bill summary is missing")
		}
		var monthlySummary monthlySummaryWire
		if err := json.Unmarshal(wire.Summary, &monthlySummary); err != nil {
			return nil, fmt.Errorf("monthly bill summary: %w", err)
		}
		result.Monthly.Summary = MonthlySummary(monthlySummary)
		summary := result.Monthly.Summary
		if summary.UseTime == "" || summary.BaseMoney == "" || summary.Flow == "" || summary.UsedMoney == "" {
			return nil, fmt.Errorf("monthly bill summary is missing its amounts")
		}
	case "operations":
		result.Kind = "operations"
		result.Operations = &OperationBills{Total: *wire.Total}
		var rows []operationBillWire
		if err := json.Unmarshal(wire.Rows, &rows); err != nil {
			return nil, fmt.Errorf("operation bill rows: %w", err)
		}
		result.Operations.Rows = make([]OperationBill, len(rows))
		for index, row := range rows {
			result.Operations.Rows[index] = OperationBill(row)
		}
	}
	return result, nil
}

// ExportBills returns an XLS workbook. A current-page export first populates the
// table in the same management session because type=1 uses server-side state.
func (s *Session) ExportBills(ctx context.Context, q BillQuery, all bool) ([]byte, error) {
	table, params, err := s.prepareBill(ctx, q)
	if err != nil {
		return nil, err
	}
	if !all {
		if _, err := s.queryBill(ctx, table, params); err != nil {
			return nil, err
		}
	}
	export := url.Values{"type": {"1"}}
	if all {
		export.Set("type", "2")
	}
	switch q.Kind {
	case "online":
		if all {
			export.Set("startTime", params.Get("startTime"))
			export.Set("endTime", params.Get("endTime"))
		}
	case "monthly":
		if all {
			export.Set("year", params.Get("year"))
		}
	case "operations":
		for _, name := range []string{"startTime", "endTime", "sortName", "sortOrder"} {
			export.Set(name, params.Get(name))
		}
	}
	data, finalURL, header, err := s.request(ctx, http.MethodGet, table.export, export)
	if err != nil {
		return nil, err
	}
	if finalURL == nil || finalURL.Path != table.export {
		return nil, fmt.Errorf("%s: unexpected export destination", table.export)
	}
	// Self serves an unquoted non-ASCII filename, which is not an RFC MIME
	// parameter. Read the deployment's actual header grammar directly.
	filename, attachment := strings.CutPrefix(header.Get("Content-Disposition"), "attachment; filename=")
	if !attachment || !strings.HasSuffix(filename, ".xls") || len(filename) <= len(".xls") ||
		strings.ContainsAny(filename, "\";\r\n") || strings.TrimSpace(filename) != filename ||
		header.Get("Content-Type") != "file;charset=UTF-8" {
		return nil, fmt.Errorf("%s: expected an XLS attachment", table.export)
	}
	magic := []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}
	if !bytes.HasPrefix(data, magic) {
		return nil, fmt.Errorf("%s: attachment is not an XLS compound document", table.export)
	}
	return data, nil
}

func (s *Session) prepareBill(ctx context.Context, q BillQuery) (billTable, url.Values, error) {
	table, err := billTableFor(q.Kind)
	if err != nil {
		return table, nil, err
	}
	q, err = normalizeBill(q, time.Now())
	if err != nil {
		return table, nil, err
	}
	if s.identity == "" {
		return table, nil, ErrSessionExpired
	}
	params := url.Values{
		"pageNumber": {strconv.Itoa(q.Page)}, "pageSize": {strconv.Itoa(q.Size)},
		"searchText": {""}, "sortName": {q.Sort}, "sortOrder": {q.Order},
	}
	if q.Kind == "monthly" {
		if q.Year == 0 {
			page, err := s.page(ctx, "/Self/bill/monthPay")
			if err != nil {
				return table, nil, err
			}
			q.Year, err = billYear(page)
			if err != nil {
				return table, nil, err
			}
		}
		params.Set("year", strconv.Itoa(q.Year))
	} else {
		params.Set("startTime", q.Start)
		params.Set("endTime", q.End)
	}
	return table, params, nil
}

func normalizeBill(q BillQuery, now time.Time) (BillQuery, error) {
	if _, err := billTableFor(q.Kind); err != nil {
		return q, err
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Size == 0 {
		q.Size = 10
	}
	if q.Page < 1 {
		return q, fmt.Errorf("bill page must be positive")
	}
	allowedSize := q.Size == 10 || q.Size == 50 || q.Size == 100 || q.Kind == "online" && q.Size == 20 || q.Kind != "online" && q.Size == 25
	if !allowedSize {
		return q, fmt.Errorf("bill page size is not offered by the %s table", q.Kind)
	}
	if q.Order == "" {
		q.Order = "DESC"
	}
	q.Order = strings.ToUpper(q.Order)
	if q.Order != "ASC" && q.Order != "DESC" {
		return q, fmt.Errorf("bill sort order must be ASC or DESC")
	}
	if q.Sort == "" {
		q.Sort = "0"
		if q.Kind == "online" {
			q.Sort = "loginTime"
		}
	}
	validSort := q.Kind == "online" && q.Sort == "loginTime"
	if q.Kind == "monthly" {
		validSort = len(q.Sort) == 1 && q.Sort[0] >= '0' && q.Sort[0] <= '7'
	}
	if q.Kind == "operations" {
		validSort = q.Sort == "0" || q.Sort == "1" || q.Sort == "2" || q.Sort == "4"
	}
	if !validSort {
		return q, fmt.Errorf("bill sort field is not offered by the %s table", q.Kind)
	}
	if q.Kind == "monthly" {
		if q.Start != "" || q.End != "" {
			return q, fmt.Errorf("monthly bills use year, not start and end dates")
		}
		if q.Year < 0 || q.Year > 9999 {
			return q, fmt.Errorf("bill year is invalid")
		}
		return q, nil
	}
	if q.Year != 0 {
		return q, fmt.Errorf("%s bills use start and end dates, not year", q.Kind)
	}
	// The school's date pickers cap their selection at today's calendar date.
	today := now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format(time.DateOnly)
	if q.Start == "" && q.End == "" {
		q.Start, q.End = today, today
	} else if q.Start == "" || q.End == "" {
		return q, fmt.Errorf("bill start and end dates must be supplied together")
	}
	start, startErr := time.Parse(time.DateOnly, q.Start)
	end, endErr := time.Parse(time.DateOnly, q.End)
	if startErr != nil || endErr != nil {
		return q, fmt.Errorf("bill dates must be valid YYYY-MM-DD calendar dates")
	}
	if end.Before(start) || end.Sub(start) > 60*24*time.Hour {
		return q, fmt.Errorf("bill end date must follow start date by at most 60 days")
	}
	if q.End > today {
		return q, fmt.Errorf("bill date range cannot end after today in Asia/Shanghai")
	}
	return q, nil
}

// The default year is the selected option (or first option, as in the browser).
func billYear(page *html.Node) (int, error) {
	selectNode := find(page, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "select" && attr(n, "id") == "year"
	})
	if selectNode == nil {
		return 0, fmt.Errorf("monthly bill year selector is absent")
	}
	defaultValue := ""
	first := true
	walk(selectNode, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "option" {
			return
		}
		value := attr(n, "value")
		if !hasAttr(n, "value") {
			value = strings.TrimSpace(nodeText(n))
		}
		if first || hasAttr(n, "selected") {
			defaultValue = value
		}
		first = false
	})
	year, err := strconv.Atoi(defaultValue)
	if err != nil || len(defaultValue) != 4 || year < 1 {
		return 0, fmt.Errorf("monthly bill default year is invalid")
	}
	return year, nil
}
