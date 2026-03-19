package selfservice

import "github.com/PuerkitoBio/goquery"

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
